package ingestion

import (
	"strings"
	"testing"
)

// These are live integration tests — they hit real APIs.
// Run with: go test ./internal/ingestion/ -v

func TestWikiSummary(t *testing.T) {
	result, err := WikiSummary("interdependence")
	if err != nil {
		t.Fatalf("WikiSummary error: %v", err)
	}
	if result.Title == "" {
		t.Error("expected non-empty Title")
	}
	if result.Extract == "" {
		t.Error("expected non-empty Extract")
	}
	if result.URL == "" {
		t.Error("expected non-empty URL")
	}
	t.Logf("Title:   %s", result.Title)
	t.Logf("URL:     %s", result.URL)
	t.Logf("Extract: %d chars", len(result.Extract))
	t.Logf("Preview: %s", truncate(result.Extract, 200))
}

func TestWikiSummaryMissingTopic(t *testing.T) {
	_, err := WikiSummary("xkzqwmblurp_nonexistent_zzzz")
	if err == nil {
		t.Error("expected error for nonexistent topic, got nil")
	}
	t.Logf("got expected error: %v", err)
}

func TestArxivSearch(t *testing.T) {
	results, err := ArxivSearch("interdependence", 3)
	if err != nil {
		t.Skipf("ArxivSearch unavailable: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("expected at least one result")
	}
	for i, r := range results {
		if r.Title == "" {
			t.Errorf("result %d: empty Title", i)
		}
		if r.Abstract == "" {
			t.Errorf("result %d: empty Abstract", i)
		}
		if r.URL == "" {
			t.Errorf("result %d: empty URL", i)
		}
		t.Logf("[%d] %s", i+1, r.Title)
		t.Logf("    URL:      %s", r.URL)
		t.Logf("    Authors:  %s", strings.Join(r.Authors, ", "))
		t.Logf("    Abstract: %s", truncate(r.Abstract, 150))
	}
}

func TestArxivDigest(t *testing.T) {
	results, err := ArxivSearch("autopoiesis", 2)
	if err != nil {
		t.Skipf("ArxivSearch unavailable: %v", err)
	}
	digest := ArxivDigest(results)
	if digest == "" {
		t.Error("expected non-empty digest")
	}
	t.Logf("Digest (%d chars):\n%s", len(digest), truncate(digest, 400))
}

func TestGutenbergSearch(t *testing.T) {
	results, err := GutenbergSearch("plato", 2)
	if err != nil {
		t.Skipf("GutenbergSearch unavailable: %v", err)
	}
	if len(results) == 0 {
		t.Skip("no Gutenberg results — skipping")
	}
	for i, r := range results {
		if r.Title == "" {
			t.Errorf("result %d: empty Title", i)
		}
		if r.URL == "" {
			t.Errorf("result %d: empty URL", i)
		}
		t.Logf("[%d] %s", i+1, r.Title)
		t.Logf("    Authors:  %s", strings.Join(r.Authors, ", "))
		t.Logf("    URL:      %s", r.URL)
		t.Logf("    TextURL:  %s", r.TextURL)
		t.Logf("    Subjects: %s", strings.Join(r.Subjects, "; "))
	}
}

func TestGutenbergDigestWithExcerpts(t *testing.T) {
	results, err := GutenbergSearch("consciousness", 1)
	if err != nil {
		t.Skipf("GutenbergSearch unavailable: %v", err)
	}
	if len(results) == 0 {
		t.Skip("no Gutenberg results for topic — skipping excerpt test")
	}
	digest := GutenbergDigest(results, true)
	if digest == "" {
		t.Error("expected non-empty digest")
	}
	t.Logf("Digest (%d chars):\n%s", len(digest), truncate(digest, 500))
}

func TestChunk(t *testing.T) {
	text := "one two three four five six seven eight nine ten"
	chunks := Chunk(text, 4, 1)
	if len(chunks) == 0 {
		t.Fatal("expected chunks")
	}
	for i, c := range chunks {
		t.Logf("chunk %d: %q", i, c)
	}
	// verify overlap: last word of chunk N should appear in chunk N+1
	if len(chunks) > 1 {
		words0 := strings.Fields(chunks[0])
		words1 := strings.Fields(chunks[1])
		last0 := words0[len(words0)-1]
		if !strings.Contains(chunks[1], last0) {
			t.Errorf("overlap missing: %q not in %q", last0, words1)
		}
	}
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
