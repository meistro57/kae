package report

import (
	"strings"
	"testing"
	"time"
)

func TestBuildBaseFilename(t *testing.T) {
	now := time.Date(2026, time.April, 11, 12, 34, 56, 0, time.UTC)
	got := BuildBaseFilename("Quantum Entanglement", now)
	want := "report_quantum_entanglement_20260411_123456"
	if got != want {
		t.Fatalf("BuildBaseFilename() = %q, want %q", got, want)
	}
}

func TestRenderHTMLIncludesConvertedMarkdown(t *testing.T) {
	doc, err := RenderHTML("Run report", "## Cycle 1\n\n- Item")
	if err != nil {
		t.Fatalf("RenderHTML() error = %v", err)
	}
	html := string(doc)
	checks := []string{"<h2>Cycle 1</h2>", "<li>Item</li>", "<title>Run report</title>"}
	for _, check := range checks {
		if !strings.Contains(html, check) {
			t.Fatalf("RenderHTML() missing expected fragment %q", check)
		}
	}
}
