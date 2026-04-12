package lens

import (
	"context"
	"fmt"
	"log"

	"github.com/google/uuid"
	"github.com/meistro/kae/collections"
	"github.com/meistro/kae/internal/llm"
	qdrantclient "github.com/meistro/kae/internal/qdrantclient"
	"github.com/qdrant/go-client/qdrant"
)

// Writer embeds LensFinding objects and upserts them into kae_lens_findings.
type Writer struct {
	qc         *qdrantclient.Client
	llm        *llm.Client
	collection string
}

// NewWriter creates a Writer.
func NewWriter(qc *qdrantclient.Client, llmClient *llm.Client, findingsCollection string) *Writer {
	return &Writer{
		qc:         qc,
		llm:        llmClient,
		collection: findingsCollection,
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

		points[i] = &qdrant.PointStruct{
			Id:      qdrant.NewID(pointID),
			Vectors: qdrant.NewVectors(vectors[i]...),
			Payload: map[string]*qdrant.Value{
				"type":            qdrant.NewValueString(string(f.Type)),
				"confidence":      qdrant.NewValueDouble(f.Confidence),
				"summary":         qdrant.NewValueString(f.Summary),
				"reasoning_trace": qdrant.NewValueString(f.ReasoningTrace),
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
			},
		}
	}

	if err := w.qc.UpsertPoints(ctx, w.collection, points); err != nil {
		return fmt.Errorf("upserting %d finding points: %w", len(points), err)
	}

	log.Printf("[writer] wrote %d findings to %q", len(points), w.collection)
	return nil
}
