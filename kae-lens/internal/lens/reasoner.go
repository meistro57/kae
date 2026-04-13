package lens

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/meistro/kae/collections"
	"github.com/meistro/kae/internal/graph"
	qdrantclient "github.com/meistro/kae/internal/qdrantclient"
	"github.com/qdrant/go-client/qdrant"
)

// Reasoner is the core agent loop. For each batch of new KAE knowledge points,
// it assesses density, retrieves adaptive neighbors, calls the Synthesizer,
// writes findings, and emits events.
type Reasoner struct {
	qc          *qdrantclient.Client
	density     *DensityCalculator
	synthesizer *Synthesizer
	writer      *Writer
	events      chan<- any
	collection  string
}

// NewReasoner creates a Reasoner.
func NewReasoner(
	qc *qdrantclient.Client,
	density *DensityCalculator,
	synthesizer *Synthesizer,
	writer *Writer,
	events chan<- any,
	knowledgeCollection string,
) *Reasoner {
	return &Reasoner{
		qc:          qc,
		density:     density,
		synthesizer: synthesizer,
		writer:      writer,
		events:      events,
		collection:  knowledgeCollection,
	}
}

// anchorPoint is a parsed knowledge point ready for reasoning.
type anchorPoint struct {
	id      string
	title   string
	domain  string
	content string
	url     string
	vector  []float32
}

// ProcessBatch runs the reasoning loop over a batch of newly-ingested points.
// Returns the number of findings produced.
func (r *Reasoner) ProcessBatch(ctx context.Context, batchID string, points []*qdrant.RetrievedPoint) (int, error) {
	totalFindings := 0

	for _, p := range points {
		if ctx.Err() != nil {
			return totalFindings, ctx.Err()
		}

		anchor, err := parseAnchor(p)
		if err != nil {
			log.Printf("[reasoner] skipping point %s: %v", p.Id, err)
			continue
		}

		findings, err := r.processPoint(ctx, batchID, anchor)
		if err != nil {
			log.Printf("[reasoner] error processing point %s (%s): %v", anchor.id, anchor.title, err)
			continue
		}

		totalFindings += len(findings)
	}

	return totalFindings, nil
}

// processPoint runs the full reasoning pipeline for a single anchor point.
func (r *Reasoner) processPoint(ctx context.Context, batchID string, anchor *anchorPoint) ([]*collections.LensFinding, error) {
	// 1. Assess local density → adaptive search parameters
	profile, err := r.density.Assess(ctx, r.collection, anchor.vector)
	if err != nil {
		return nil, fmt.Errorf("density assessment: %w", err)
	}

	log.Printf("[reasoner] point %q | density=%s | width=%d | threshold=%.2f",
		anchor.title, profile.Label, profile.SearchWidth, profile.ScoreThreshold)

	// 2. Retrieve adaptive neighbors from kae_knowledge
	neighbors, err := r.qc.QueryNeighbors(
		ctx,
		r.collection,
		anchor.vector,
		uint64(profile.SearchWidth),
		profile.ScoreThreshold,
	)
	if err != nil {
		return nil, fmt.Errorf("querying neighbors: %w", err)
	}

	// Filter out the anchor itself from neighbors
	filtered := make([]*qdrant.ScoredPoint, 0, len(neighbors))
	for _, n := range neighbors {
		if n.Id.GetUuid() != anchor.id {
			filtered = append(filtered, n)
		}
	}

	if len(filtered) == 0 {
		log.Printf("[reasoner] no neighbors found for %q, skipping synthesis", anchor.title)
		return nil, nil
	}

	// 3. Call synthesizer
	findings, err := r.synthesizer.Synthesize(ctx, batchID, anchor, filtered, profile)
	if err != nil {
		return nil, fmt.Errorf("synthesis: %w", err)
	}

	if len(findings) == 0 {
		return nil, nil
	}

	// 4. Write findings to kae_lens_findings
	if err := r.writer.Write(ctx, findings); err != nil {
		return nil, fmt.Errorf("writing findings: %w", err)
	}

	// 5. Emit finding events to dashboard
	for _, f := range findings {
		r.emit(graph.FindingEvent{
			ID:             fmt.Sprintf("%s-%d", batchID, time.Now().UnixNano()),
			Type:           string(f.Type),
			Confidence:     f.Confidence,
			SourceIDs:      f.SourcePointIDs,
			Domains:        f.Domains,
			Summary:        f.Summary,
			ReasoningTrace: f.ReasoningTrace,
			CreatedAt:      time.Unix(f.CreatedAt, 0),
			BatchID:        f.BatchID,
		})
	}

	return findings, nil
}

// parseAnchor extracts a typed anchorPoint from a raw Qdrant RetrievedPoint.
// Handles both the kae_knowledge schema (title/content/domain) and the
// kae_chunks schema (source/text/topic) that KAE actually writes.
func parseAnchor(p *qdrant.RetrievedPoint) (*anchorPoint, error) {
	if p.Id == nil {
		return nil, fmt.Errorf("point has no ID")
	}

	// Extract vector — v1.17+ uses VectorsOutput: GetVector().GetDense().GetData()
	vec := extractVector(p.Vectors)
	if len(vec) == 0 {
		return nil, fmt.Errorf("point has no vector")
	}

	// Extract payload — support both kae_knowledge and kae_chunks schemas
	payload := qdrantclient.PayloadToMap(p.Payload)

	title := stringFromMap(payload, "title", "source")
	content := stringFromMap(payload, "content", "text")
	domain := stringFromMap(payload, "domain", "topic")
	url := stringFromMap(payload, "url")

	// Use a sensible fallback title so logs are readable
	if title == "" {
		title = fmt.Sprintf("point-%s", qdrantclient.PointIDStr(p.Id))
	}

	return &anchorPoint{
		id:      qdrantclient.PointIDStr(p.Id),
		title:   title,
		domain:  domain,
		content: content,
		url:     url,
		vector:  vec,
	}, nil
}

// extractVector pulls the float32 dense vector out of a VectorsOutput.
// Qdrant go-client v1.17+ changed the type from *Vectors to *VectorsOutput,
// with data at .GetVector().GetDense().GetData().
func extractVector(vo *qdrant.VectorsOutput) []float32 {
	if vo == nil {
		return nil
	}
	if v := vo.GetVector(); v != nil {
		if d := v.GetDense(); d != nil {
			return d.GetData()
		}
	}
	// Named vectors — grab the default ("") or first available
	if named := vo.GetVectors(); named != nil {
		vecs := named.GetVectors()
		if v, ok := vecs[""]; ok {
			if d := v.GetDense(); d != nil {
				return d.GetData()
			}
		}
		for _, v := range vecs {
			if d := v.GetDense(); d != nil {
				return d.GetData()
			}
		}
	}
	return nil
}

// stringFromMap returns the first non-empty string value for the given keys.
func stringFromMap(m map[string]any, keys ...string) string {
	for _, k := range keys {
		if v, ok := m[k]; ok {
			if s, ok := v.(string); ok && s != "" {
				return s
			}
		}
	}
	return ""
}

func (r *Reasoner) emit(event any) {
	select {
	case r.events <- event:
	default:
		// Non-blocking: if channel is full, drop rather than stall the agent
	}
}
