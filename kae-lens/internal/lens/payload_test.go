package lens

import (
	"strings"
	"testing"

	"github.com/qdrant/go-client/qdrant"
)

func TestNeighborSummaryFromPoint_KnowledgeSchema(t *testing.T) {
	p := &qdrant.ScoredPoint{
		Id:    qdrant.NewID("knowledge-1"),
		Score: 0.91,
		Payload: map[string]*qdrant.Value{
			"title":   qdrant.NewValueString("Quantum Decoherence"),
			"content": qdrant.NewValueString("Decoherence explains the classical transition."),
			"domain":  qdrant.NewValueString("physics"),
			"url":     qdrant.NewValueString("https://example.com/decoherence"),
		},
	}

	ns := neighborSummaryFromPoint(p)

	if ns.Content != "Decoherence explains the classical transition." {
		t.Errorf("content: got %q", ns.Content)
	}
	if ns.Domain != "physics" {
		t.Errorf("domain: got %q, want physics", ns.Domain)
	}
	if ns.Title != "Quantum Decoherence" {
		t.Errorf("title: got %q", ns.Title)
	}
	if ns.URL != "https://example.com/decoherence" {
		t.Errorf("url: got %q", ns.URL)
	}
}

func TestNeighborSummaryFromPoint_ChunksSchema(t *testing.T) {
	p := &qdrant.ScoredPoint{
		Id:    qdrant.NewID("chunk-1"),
		Score: 0.88,
		Payload: map[string]*qdrant.Value{
			"source":          qdrant.NewValueString("https://example.com/chunk"),
			"text":            qdrant.NewValueString("Chunk body text from the knowledge base."),
			"semantic_domain": qdrant.NewValueString("philosophy"),
		},
	}

	ns := neighborSummaryFromPoint(p)

	if ns.Content != "Chunk body text from the knowledge base." {
		t.Errorf("content: got %q", ns.Content)
	}
	if ns.Domain != "philosophy" {
		t.Errorf("domain: got %q, want philosophy", ns.Domain)
	}
	if ns.Title != "https://example.com/chunk" {
		t.Errorf("title: got %q", ns.Title)
	}
}

func TestNeighborSummaryFromPoint_MissingKeys(t *testing.T) {
	p := &qdrant.ScoredPoint{
		Id:    qdrant.NewID("bare-1"),
		Score: 0.5,
		Payload: map[string]*qdrant.Value{
			"lens_processed": qdrant.NewValueBool(true),
		},
	}

	ns := neighborSummaryFromPoint(p) // must not panic

	if ns.Content != "" {
		t.Errorf("content: got %q, want empty", ns.Content)
	}
	if ns.Domain != "" {
		t.Errorf("domain: got %q, want empty", ns.Domain)
	}
	if ns.Title != "" {
		t.Errorf("title: got %q, want empty", ns.Title)
	}
}

func TestNeighborSummaryFromPoint_PrefersKnowledgeSchema(t *testing.T) {
	p := &qdrant.ScoredPoint{
		Id:    qdrant.NewID("dual-1"),
		Score: 0.7,
		Payload: map[string]*qdrant.Value{
			"content":         qdrant.NewValueString("knowledge content"),
			"text":            qdrant.NewValueString("chunk text"),
			"domain":          qdrant.NewValueString("domain"),
			"semantic_domain": qdrant.NewValueString("semantic_domain"),
			"title":           qdrant.NewValueString("knowledge title"),
			"source":          qdrant.NewValueString("https://example.com/source"),
		},
	}

	ns := neighborSummaryFromPoint(p)

	if ns.Content != "knowledge content" {
		t.Errorf("content: got %q, want knowledge content", ns.Content)
	}
	if ns.Domain != "domain" {
		t.Errorf("domain: got %q, want domain", ns.Domain)
	}
	if ns.Title != "knowledge title" {
		t.Errorf("title: got %q, want knowledge title", ns.Title)
	}
}

func TestBuildUserPrompt_IncludesNeighborTextForBothSchemas(t *testing.T) {
	s := testSynthesizer()

	knowledge := neighborSummaryFromPoint(&qdrant.ScoredPoint{
		Id:    qdrant.NewID("knowledge-1"),
		Score: 0.9,
		Payload: map[string]*qdrant.Value{
			"title":   qdrant.NewValueString("Decoherence"),
			"content": qdrant.NewValueString("knowledge neighbor text"),
			"domain":  qdrant.NewValueString("physics"),
		},
	})
	chunk := neighborSummaryFromPoint(&qdrant.ScoredPoint{
		Id:    qdrant.NewID("chunk-1"),
		Score: 0.8,
		Payload: map[string]*qdrant.Value{
			"source":          qdrant.NewValueString("https://example.com/chunk"),
			"text":            qdrant.NewValueString("chunk neighbor text"),
			"semantic_domain": qdrant.NewValueString("philosophy"),
		},
	})

	anchor := &anchorPoint{id: "anchor", title: "Anchor", domain: "physics", content: "anchor content"}
	profile := &DensityProfile{Label: "dense", SearchWidth: 10}

	prompt := s.buildUserPrompt(anchor, []neighborSummary{knowledge, chunk}, profile)

	for _, want := range []string{
		"knowledge neighbor text",
		"chunk neighbor text",
		"physics",
		"philosophy",
	} {
		if !strings.Contains(prompt, want) {
			t.Errorf("prompt missing %q", want)
		}
	}
}
