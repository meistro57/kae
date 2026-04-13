package ingestion

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

const semanticScholarAPI = "https://api.semanticscholar.org/graph/v1/paper/search"

// SemanticPaper is a paper returned by Semantic Scholar.
type SemanticPaper struct {
	ID       string
	Title    string
	Abstract string
	TLDR     string // one-sentence AI summary when available
	Year     int
	URL      string
}

// SemanticScholarSearch returns papers matching topic from Semantic Scholar.
// No API key required for basic usage. Uses tl;dr summaries to supplement
// abstracts, which helps bridge jargon gaps in cross-domain reasoning.
func SemanticScholarSearch(topic string, limit int) ([]*SemanticPaper, error) {
	params := url.Values{
		"query":  {topic},
		"limit":  {fmt.Sprintf("%d", limit)},
		"fields": {"title,abstract,tldr,year,url,externalIds"},
	}

	req, err := http.NewRequest("GET", semanticScholarAPI+"?"+params.Encode(), nil)
	if err != nil {
		return nil, fmt.Errorf("semantic scholar request: %w", err)
	}
	req.Header.Set("User-Agent", "KAE-Bot/1.0 (knowledge-archaeology-engine)")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("semantic scholar fetch: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("semantic scholar returned %d", resp.StatusCode)
	}

	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var response struct {
		Data []struct {
			PaperID  string `json:"paperId"`
			Title    string `json:"title"`
			Abstract string `json:"abstract"`
			TLDR     *struct {
				Text string `json:"text"`
			} `json:"tldr"`
			Year int    `json:"year"`
			URL  string `json:"url"`
		} `json:"data"`
	}

	if err := json.Unmarshal(b, &response); err != nil {
		return nil, fmt.Errorf("semantic scholar parse: %w", err)
	}

	papers := make([]*SemanticPaper, 0, len(response.Data))
	for _, d := range response.Data {
		if d.Title == "" {
			continue
		}
		p := &SemanticPaper{
			ID:       d.PaperID,
			Title:    strings.TrimSpace(d.Title),
			Abstract: strings.TrimSpace(d.Abstract),
			Year:     d.Year,
			URL:      d.URL,
		}
		if d.TLDR != nil {
			p.TLDR = strings.TrimSpace(d.TLDR.Text)
		}
		papers = append(papers, p)
	}

	return papers, nil
}

// SemanticPaperToChunks converts a SemanticPaper to embeddable chunks.
// Prefers tl;dr + abstract over abstract alone so cross-domain links are tighter.
func SemanticPaperToChunks(p *SemanticPaper) []string {
	var parts []string
	parts = append(parts, fmt.Sprintf("Title: %s", p.Title))
	if p.Year > 0 {
		parts = append(parts, fmt.Sprintf("Year: %d", p.Year))
	}
	if p.TLDR != "" {
		parts = append(parts, fmt.Sprintf("Summary: %s", p.TLDR))
	}
	if p.Abstract != "" {
		parts = append(parts, fmt.Sprintf("Abstract: %s", p.Abstract))
	}
	full := strings.Join(parts, "\n\n")
	return Chunk(full, 200, 30)
}
