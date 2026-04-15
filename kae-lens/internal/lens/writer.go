package lens

import (
	"context"
	"encoding/json"
	"fmt"
	"log"

	"github.com/google/uuid"
	"github.com/meistro/kae/collections"
	"github.com/meistro/kae/internal/llm"
	qdrantclient "github.com/meistro/kae/internal/qdrantclient"
	"github.com/qdrant/go-client/qdrant"
)

// Writer embeds LensFinding objects and upserts them into kae_lens_findings.
// It also writes data-grounded correction chunks back into the knowledge
// collection so future KAE cycles reason over corrected understanding.
type Writer struct {
	qc                  *qdrantclient.Client
	llm                 *llm.Client
	collection          string // kae_lens_findings
	knowledgeCollection string // kae_chunks — where corrections are written back
}

// NewWriter creates a Writer.
func NewWriter(qc *qdrantclient.Client, llmClient *llm.Client, findingsCollection, knowledgeCollection string) *Writer {
	return &Writer{
		qc:                  qc,
		llm:                 llmClient,
		collection:          findingsCollection,
		knowledgeCollection: knowledgeCollection,
	}
}

// Write embeds and upserts a batch of findings into kae_lens_findings.
func (w *Writer) Write(ctx context.Context, findings []*collections.LensFinding) error {
	if len(findings) == 0 {
		return nil
	}

	// Collect embedding texts for batch embedding
	texts := make([]string, len(findings))
	for i, f := range findings {
		texts[i] = f.EmbeddingText
	}

	// Batch embed all findings in one API call
	vectors, err := w.llm.EmbedBatch(ctx, texts)
	if err != nil {
		return fmt.Errorf("embedding %d findings: %w", len(findings), err)
	}
	if len(vectors) != len(findings) {
		return fmt.Errorf("expected %d embeddings, got %d", len(findings), len(vectors))
	}

	// Build Qdrant points
	points := make([]*qdrant.PointStruct, len(findings))
	for i, f := range findings {
		pointID := uuid.New().String()

		// Serialize source_point_ids and domains as Qdrant list values
		sourceIDValues := make([]*qdrant.Value, len(f.SourcePointIDs))
		for j, id := range f.SourcePointIDs {
			sourceIDValues[j] = qdrant.NewValueString(id)
		}

		domainValues := make([]*qdrant.Value, len(f.Domains))
		for j, d := range f.Domains {
			domainValues[j] = qdrant.NewValueString(d)
		}

		payload := map[string]*qdrant.Value{
			"type":            qdrant.NewValueString(string(f.Type)),
			"confidence":      qdrant.NewValueDouble(f.Confidence),
			"summary":         qdrant.NewValueString(f.Summary),
			"reasoning_trace": qdrant.NewValueString(f.ReasoningTrace),
			"correction":      qdrant.NewValueString(f.Correction),
			"embedding_text":  qdrant.NewValueString(f.EmbeddingText),
			"batch_id":        qdrant.NewValueString(f.BatchID),
			"created_at":      qdrant.NewValueInt(f.CreatedAt),
			"reviewed":        qdrant.NewValueBool(f.Reviewed),
			"source_point_ids": {
				Kind: &qdrant.Value_ListValue{
					ListValue: &qdrant.ListValue{Values: sourceIDValues},
				},
			},
			"domains": {
				Kind: &qdrant.Value_ListValue{
					ListValue: &qdrant.ListValue{Values: domainValues},
				},
			},
		}
		if len(f.SourceURLs) > 0 {
			if urlsJSON, err := json.Marshal(f.SourceURLs); err == nil {
				payload["source_urls"] = qdrant.NewValueString(string(urlsJSON))
			}
		}

		points[i] = &qdrant.PointStruct{
			Id:      qdrant.NewID(pointID),
			Vectors: qdrant.NewVectors(vectors[i]...),
			Payload: payload,
		}
	}

	if err := w.qc.UpsertPoints(ctx, w.collection, points); err != nil {
		return fmt.Errorf("upserting %d finding points: %w", len(points), err)
	}

	log.Printf("[writer] wrote %d findings to %q", len(points), w.collection)
	return nil
}

// WriteCorrectionChunks embeds correction text from anomaly/contradiction
// findings and upserts them back into the knowledge collection as new chunks.
// This makes the corrected understanding available to future KAE reasoning
// cycles and future Lens passes without altering the original source points.
//
// Correction chunks are marked lens_processed:true so the watcher does not
// re-process them, and lens_correction:true so they are identifiable.
func (w *Writer) WriteCorrectionChunks(ctx context.Context, findings []*collections.LensFinding) error {
	// Collect findings that have a correction
	var corrected []*collections.LensFinding
	for _, f := range findings {
		if f.Correction != "" {
			corrected = append(corrected, f)
		}
	}
	if len(corrected) == 0 {
		return nil
	}

	// Embed correction texts
	texts := make([]string, len(corrected))
	for i, f := range corrected {
		texts[i] = f.Correction
	}

	vectors, err := w.llm.EmbedBatch(ctx, texts)
	if err != nil {
		return fmt.Errorf("embedding %d correction chunks: %w", len(corrected), err)
	}
	if len(vectors) != len(corrected) {
		return fmt.Errorf("expected %d correction embeddings, got %d", len(corrected), len(vectors))
	}

	// Build points in the kae_chunks schema
	points := make([]*qdrant.PointStruct, len(corrected))
	for i, f := range corrected {
		pointID := uuid.New().String()

		// Use first domain as topic; fall back to finding type
		topic := string(f.Type)
		if len(f.Domains) > 0 {
			topic = f.Domains[0]
		}

		// Serialize source IDs for the payload
		sourceIDValues := make([]*qdrant.Value, len(f.SourcePointIDs))
		for j, id := range f.SourcePointIDs {
			sourceIDValues[j] = qdrant.NewValueString(id)
		}

		points[i] = &qdrant.PointStruct{
			Id:      qdrant.NewID(pointID),
			Vectors: qdrant.NewVectors(vectors[i]...),
			Payload: map[string]*qdrant.Value{
				// kae_chunks schema fields so KAE's search phase picks this up
				"source": qdrant.NewValueString("lens_correction"),
				"text":   qdrant.NewValueString(f.Correction),
				"topic":  qdrant.NewValueString(topic),
				"run_id": qdrant.NewValueString("lens_" + f.BatchID),
				// Correction metadata
				"lens_processed":        qdrant.NewValueBool(true), // skip re-processing
				"lens_correction":       qdrant.NewValueBool(true), // identifiable
				"correction_for_type":   qdrant.NewValueString(string(f.Type)),
				"correction_confidence": qdrant.NewValueDouble(f.Confidence),
				"correction_source_ids": {
					Kind: &qdrant.Value_ListValue{
						ListValue: &qdrant.ListValue{Values: sourceIDValues},
					},
				},
			},
		}
	}

	if err := w.qc.UpsertPoints(ctx, w.knowledgeCollection, points); err != nil {
		return fmt.Errorf("upserting %d correction chunks: %w", len(points), err)
	}

	log.Printf("[writer] wrote %d correction chunks back to %q", len(points), w.knowledgeCollection)
	return nil
}
