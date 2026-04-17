package ingestion

import (
	"strings"
	"testing"

	"github.com/meistro57/kae/internal/llm"
)

// mockProvider returns a fixed JSON response for testing
type mockProvider struct {
	response string
}

func (m *mockProvider) Stream(_ string, _ []llm.Message) <-chan llm.Chunk {
	ch := make(chan llm.Chunk, 2)
	ch <- llm.Chunk{Type: llm.ChunkText, Text: m.response}
	ch <- llm.Chunk{Type: llm.ChunkDone}
	close(ch)
	return ch
}

func (m *mockProvider) ModelName() string { return "mock" }

func TestInferDomainFromSource(t *testing.T) {
	tests := []struct {
		source   string
		expected string
	}{
		{"https://pubmed.ncbi.nlm.nih.gov/38022829/", "Medical Research"},
		{"https://arxiv.org/abs/2301.00001", "Academic Research"},
		{"https://en.wikipedia.org/wiki/Marcus_Aurelius", "Encyclopedia"},
		{"Meditations - Marcus Aurelius", "Stoic Philosophy"},
		{"The Kybalion", "Hermetic Philosophy"},
		{"", "General Knowledge"},
	}
	for _, tt := range tests {
		got := inferDomainFromSource(tt.source)
		if got != tt.expected {
			t.Errorf("inferDomainFromSource(%q) = %q, want %q", tt.source, got, tt.expected)
		}
	}
}

func TestClassifyDomainBatchEmpty(t *testing.T) {
	provider := &mockProvider{}
	results := ClassifyDomainBatch(nil, nil, provider)
	if len(results) != 0 {
		t.Errorf("expected empty results, got %d", len(results))
	}
}

func TestClassifyDomainBatchLLMSuccess(t *testing.T) {
	provider := &mockProvider{
		response: `[
			{"domain": "Roman History", "confidence": 0.95},
			{"domain": "Medical Research", "confidence": 0.88}
		]`,
	}

	texts := []string{
		"Marcus Aurelius was Roman Emperor from 161 to 180 AD and wrote Meditations.",
		"RETRO-POPE: A retrospective multicenter real-world study of treatment outcomes.",
	}
	sources := []string{
		"Meditations - Marcus Aurelius",
		"https://pubmed.ncbi.nlm.nih.gov/38022829/",
	}

	results := ClassifyDomainBatch(texts, sources, provider)
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	if !strings.Contains(results[0].Domain, "Roman") {
		t.Errorf("chunk 0: expected Roman domain, got %q", results[0].Domain)
	}
	if results[0].Confidence < 0.9 {
		t.Errorf("chunk 0: expected confidence >= 0.9, got %f", results[0].Confidence)
	}
	if !strings.Contains(results[1].Domain, "Medical") {
		t.Errorf("chunk 1: expected Medical domain, got %q", results[1].Domain)
	}
}

func TestClassifyDomainBatchLLMFallback(t *testing.T) {
	provider := &mockProvider{response: "not valid json"}

	texts := []string{"some text"}
	sources := []string{"https://pubmed.ncbi.nlm.nih.gov/123/"}

	results := ClassifyDomainBatch(texts, sources, provider)
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Domain != "Medical Research" {
		t.Errorf("expected heuristic fallback 'Medical Research', got %q", results[0].Domain)
	}
	if results[0].Confidence != 0.5 {
		t.Errorf("expected confidence 0.5, got %f", results[0].Confidence)
	}
}

func TestClassifyDomainBatchPartialLLMResponse(t *testing.T) {
	// LLM returns only 1 result for 2 chunks — second should fall back to heuristic
	provider := &mockProvider{
		response: `[{"domain": "Hermetic Philosophy", "confidence": 0.9}]`,
	}

	texts := []string{"Kybalion text...", "arxiv abstract..."}
	sources := []string{"The Kybalion", "https://arxiv.org/abs/2301.00001"}

	results := ClassifyDomainBatch(texts, sources, provider)
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	if results[0].Domain != "Hermetic Philosophy" {
		t.Errorf("chunk 0: got %q", results[0].Domain)
	}
	if results[1].Domain != "Academic Research" {
		t.Errorf("chunk 1 fallback: got %q", results[1].Domain)
	}
}
