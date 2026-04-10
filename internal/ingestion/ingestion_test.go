package ingestion

import (
	"strings"
	"testing"
)

func TestParseArxivFeedParsesEntries(t *testing.T) {
	t.Parallel()

	xmlFeed := `<?xml version="1.0" encoding="UTF-8"?>
<feed xmlns="http://www.w3.org/2005/Atom">
  <entry>
    <id>http://arxiv.org/abs/1234.5678v1</id>
    <title>  A Curious Paper  </title>
    <summary>  A concise abstract. </summary>
    <author><name>Ada Lovelace</name></author>
    <author><name>Alan Turing</name></author>
  </entry>
</feed>`

	papers, err := parseArxivFeed([]byte(xmlFeed))
	if err != nil {
		t.Fatalf("parseArxivFeed returned error: %v", err)
	}

	if len(papers) != 1 {
		t.Fatalf("expected 1 paper, got %d", len(papers))
	}

	paper := papers[0]
	if paper.ID != "http://arxiv.org/abs/1234.5678v1" {
		t.Fatalf("unexpected id: %q", paper.ID)
	}
	if paper.Title != "A Curious Paper" {
		t.Fatalf("unexpected title: %q", paper.Title)
	}
	if paper.Abstract != "A concise abstract." {
		t.Fatalf("unexpected abstract: %q", paper.Abstract)
	}
	if len(paper.Authors) != 2 || paper.Authors[0] != "Ada Lovelace" || paper.Authors[1] != "Alan Turing" {
		t.Fatalf("unexpected authors: %#v", paper.Authors)
	}
}

func TestParseArxivFeedRejectsInvalidXML(t *testing.T) {
	t.Parallel()

	_, err := parseArxivFeed([]byte("<feed><entry>missing end tags"))
	if err == nil {
		t.Fatal("expected parse error for invalid xml")
	}
}

func TestBooksForTopicReturnsRelevantMatches(t *testing.T) {
	t.Parallel()

	books := BooksForTopic("quantum consciousness")
	if len(books) == 0 {
		t.Fatal("expected relevant books for topic")
	}

	seen := map[int]bool{}
	for _, b := range books {
		if seen[b.ID] {
			t.Fatalf("duplicate book id returned: %d", b.ID)
		}
		seen[b.ID] = true
	}

	if !seen[17921] || !seen[8800] || !seen[2411] {
		t.Fatalf("expected quantum and consciousness recommendations, got ids: %#v", seen)
	}
}

func TestBooksForTopicFallsBackToDefault(t *testing.T) {
	t.Parallel()

	books := BooksForTopic("utterly unrelated topic")
	if len(books) != 2 {
		t.Fatalf("expected 2 default books, got %d", len(books))
	}
	if books[0].ID != KAEBookList[0].ID || books[1].ID != KAEBookList[1].ID {
		t.Fatalf("expected default first two books, got %+v", books)
	}
}

func TestStripGutenbergBoilerplateRemovesHeadersAndFooters(t *testing.T) {
	t.Parallel()

	text := `Intro
*** START OF THE PROJECT GUTENBERG EBOOK SAMPLE ***
Body content lives here.
*** END OF THE PROJECT GUTENBERG EBOOK SAMPLE ***
Footer`
	stripped := stripGutenbergBoilerplate(text)

	if strings.Contains(stripped, "START OF THE PROJECT GUTENBERG") || strings.Contains(stripped, "END OF THE PROJECT GUTENBERG") {
		t.Fatalf("expected boilerplate markers removed, got %q", stripped)
	}
	if !strings.Contains(stripped, "Body content lives here.") {
		t.Fatalf("expected body content preserved, got %q", stripped)
	}
}

func TestChunkProducesExpectedOverlap(t *testing.T) {
	t.Parallel()

	text := "one two three four five six seven eight nine ten"
	chunks := Chunk(text, 4, 1)

	expected := []string{
		"one two three four",
		"four five six seven",
		"seven eight nine ten",
	}

	if len(chunks) != len(expected) {
		t.Fatalf("expected %d chunks, got %d (%#v)", len(expected), len(chunks), chunks)
	}
	for i := range expected {
		if chunks[i] != expected[i] {
			t.Fatalf("chunk %d mismatch: want %q got %q", i, expected[i], chunks[i])
		}
	}
}
