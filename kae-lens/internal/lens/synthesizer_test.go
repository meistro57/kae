package lens

import (
	"strings"
	"testing"

	"github.com/meistro/kae/internal/config"
)

func testSynthesizer() *Synthesizer {
	return &Synthesizer{
		cfg: &config.LensConfig{
			LLM: config.LLMConfig{
				MinConfidence:      0.65,
				FastBatchThreshold: 5,
			},
		},
	}
}

// ── parseFindings ─────────────────────────────────────────────────────────────

func TestParseFindings_ValidJSON(t *testing.T) {
	s := testSynthesizer()
	input := `[
  {
    "type": "connection",
    "confidence": 0.87,
    "source_point_ids": ["abc", "def"],
    "domains": ["physics", "philosophy"],
    "summary": "Quantum entanglement mirrors the Vedic concept of Akasha.",
    "reasoning_trace": "Step 1: anchor is QE. Step 2: neighbor discusses Akasha. Conclusion: shared non-locality."
  }
]`
	findings, err := s.parseFindings(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(findings) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(findings))
	}
	f := findings[0]
	if f.Type != "connection" {
		t.Errorf("type: got %q, want %q", f.Type, "connection")
	}
	if f.Confidence != 0.87 {
		t.Errorf("confidence: got %.2f, want 0.87", f.Confidence)
	}
	if len(f.SourcePointIDs) != 2 {
		t.Errorf("source IDs: got %d, want 2", len(f.SourcePointIDs))
	}
}

func TestParseFindings_EmptyArray(t *testing.T) {
	s := testSynthesizer()
	findings, err := s.parseFindings(`[]`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(findings) != 0 {
		t.Errorf("expected 0 findings, got %d", len(findings))
	}
}

func TestParseFindings_StripsMarkdownFences(t *testing.T) {
	s := testSynthesizer()
	input := "```json\n[{\"type\":\"anomaly\",\"confidence\":0.9,\"source_point_ids\":[\"x\"],\"domains\":[\"physics\"],\"summary\":\"outlier\",\"reasoning_trace\":\"trace\"}]\n```"
	findings, err := s.parseFindings(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(findings) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(findings))
	}
	if findings[0].Type != "anomaly" {
		t.Errorf("got type %q, want anomaly", findings[0].Type)
	}
}

func TestParseFindings_PreambleBeforeArray(t *testing.T) {
	s := testSynthesizer()
	input := `Here are the findings I identified:
[{"type":"cluster","confidence":0.75,"source_point_ids":["a"],"domains":["math"],"summary":"group","reasoning_trace":"trace"}]
`
	findings, err := s.parseFindings(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(findings) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(findings))
	}
}

func TestParseFindings_NoArray(t *testing.T) {
	s := testSynthesizer()
	_, err := s.parseFindings("Nothing to report here.")
	if err == nil {
		t.Error("expected error for missing JSON array, got nil")
	}
}

func TestParseFindings_InvalidJSON(t *testing.T) {
	s := testSynthesizer()
	_, err := s.parseFindings("[{bad json}]")
	if err == nil {
		t.Error("expected error for invalid JSON, got nil")
	}
}

// ── isValidFindingType ────────────────────────────────────────────────────────

func TestIsValidFindingType(t *testing.T) {
	valid := []string{"connection", "contradiction", "cluster", "anomaly"}
	for _, v := range valid {
		if !isValidFindingType(v) {
			t.Errorf("%q should be valid", v)
		}
	}
	invalid := []string{"", "unknown", "Connection", "ANOMALY", "random"}
	for _, v := range invalid {
		if isValidFindingType(v) {
			t.Errorf("%q should be invalid", v)
		}
	}
}

// ── contains ─────────────────────────────────────────────────────────────────

func TestContains(t *testing.T) {
	s := []string{"a", "b", "c"}
	if !contains(s, "b") {
		t.Error("expected contains(s, \"b\") == true")
	}
	if contains(s, "d") {
		t.Error("expected contains(s, \"d\") == false")
	}
	if contains(nil, "a") {
		t.Error("expected contains(nil, ...) == false")
	}
}

// ── uniqueDomains ─────────────────────────────────────────────────────────────

func TestUniqueDomains(t *testing.T) {
	in := []string{"physics", "philosophy", "physics", "", "mathematics", "philosophy"}
	out := uniqueDomains(in)

	if len(out) != 3 {
		t.Errorf("expected 3 unique domains, got %d: %v", len(out), out)
	}
	// empty strings must be filtered
	for _, d := range out {
		if d == "" {
			t.Error("empty string slipped through uniqueDomains")
		}
	}
}

func TestUniqueDomains_AllEmpty(t *testing.T) {
	out := uniqueDomains([]string{"", "", ""})
	if len(out) != 0 {
		t.Errorf("expected empty result, got %v", out)
	}
}

// ── buildSystemPrompt ─────────────────────────────────────────────────────────

func TestBuildSystemPrompt_ContainsKeyInstructions(t *testing.T) {
	s := testSynthesizer()
	prompt := s.buildSystemPrompt()

	required := []string{
		"connection",
		"contradiction",
		"cluster",
		"anomaly",
		"JSON array",
		"confidence",
		"reasoning_trace",
	}
	for _, kw := range required {
		if !strings.Contains(prompt, kw) {
			t.Errorf("system prompt missing keyword %q", kw)
		}
	}
}

// ── buildUserPrompt ───────────────────────────────────────────────────────────

func TestBuildUserPrompt_ContainsAnchorData(t *testing.T) {
	s := testSynthesizer()
	anchor := &anchorPoint{
		id:      "test-uuid-123",
		title:   "Quantum Entanglement",
		domain:  "physics",
		content: "Two particles remain connected regardless of distance.",
	}
	profile := &DensityProfile{Label: "sparse", SearchWidth: 35}

	prompt := s.buildUserPrompt(anchor, nil, profile)

	checks := []string{"test-uuid-123", "Quantum Entanglement", "physics", "sparse"}
	for _, c := range checks {
		if !strings.Contains(prompt, c) {
			t.Errorf("user prompt missing %q", c)
		}
	}
}

func TestBuildUserPrompt_TruncatesLongContent(t *testing.T) {
	s := testSynthesizer()
	longContent := strings.Repeat("x", 700)
	anchor := &anchorPoint{id: "id", title: "T", domain: "d", content: longContent}
	profile := &DensityProfile{Label: "medium", SearchWidth: 20}

	prompt := s.buildUserPrompt(anchor, nil, profile)

	// The truncated content + "..." should appear, not the full 700 chars
	if strings.Contains(prompt, longContent) {
		t.Error("long content was not truncated in user prompt")
	}
	if !strings.Contains(prompt, "...") {
		t.Error("truncated content should end with '...'")
	}
}
