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
	ID            string
	Title         string
	Abstract      string
	TLDR          string // one-sentence AI summary when available
	Year          int
	URL           string
	CitationCount int
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
			Year          int    `json:"year"`
			URL           string `json:"url"`
			CitationCount int    `json:"citationCount"`
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
			ID:            d.PaperID,
			Title:         strings.TrimSpace(d.Title),
			Abstract:      strings.TrimSpace(d.Abstract),
			Year:          d.Year,
			URL:           d.URL,
			CitationCount: d.CitationCount,
		}
		if d.TLDR != nil {
			p.TLDR = strings.TrimSpace(d.TLDR.Text)
		}
		papers = append(papers, p)
	}

	return papers, nil
}

const semanticScholarPaperAPI = "https://api.semanticscholar.org/graph/v1/paper"

// GetPaperReferences returns papers cited by the given paper ID.
// paperID can be a Semantic Scholar ID, "arXiv:2109.01234", "DOI:...", etc.
func GetPaperReferences(paperID string, limit int) ([]*SemanticPaper, error) {
	endpoint := fmt.Sprintf("%s/%s/references?fields=paperId,title,abstract,tldr,year,url,citationCount&limit=%d",
		semanticScholarPaperAPI, url.PathEscape(paperID), limit)

	req, err := http.NewRequest("GET", endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("references request: %w", err)
	}
	req.Header.Set("User-Agent", "KAE-Bot/1.0 (knowledge-archaeology-engine)")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("references fetch: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("references returned %d for %s", resp.StatusCode, paperID)
	}

	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var response struct {
		Data []struct {
			CitedPaper struct {
				PaperID  string `json:"paperId"`
				Title    string `json:"title"`
				Abstract string `json:"abstract"`
				TLDR     *struct {
					Text string `json:"text"`
				} `json:"tldr"`
				Year          int    `json:"year"`
				URL           string `json:"url"`
				CitationCount int    `json:"citationCount"`
			} `json:"citedPaper"`
		} `json:"data"`
	}

	if err := json.Unmarshal(b, &response); err != nil {
		return nil, fmt.Errorf("references parse: %w", err)
	}

	papers := make([]*SemanticPaper, 0, len(response.Data))
	for _, d := range response.Data {
		cp := d.CitedPaper
		if cp.Title == "" {
			continue
		}
		p := &SemanticPaper{
			ID:            cp.PaperID,
			Title:         strings.TrimSpace(cp.Title),
			Abstract:      strings.TrimSpace(cp.Abstract),
			Year:          cp.Year,
			URL:           cp.URL,
			CitationCount: cp.CitationCount,
		}
		if cp.TLDR != nil {
			p.TLDR = strings.TrimSpace(cp.TLDR.Text)
		}
		papers = append(papers, p)
	}

	return papers, nil
}

// GetPaperCitations returns papers that cite the given paper ID.
func GetPaperCitations(paperID string, limit int) ([]*SemanticPaper, error) {
	endpoint := fmt.Sprintf("%s/%s/citations?fields=paperId,title,abstract,tldr,year,url,citationCount&limit=%d",
		semanticScholarPaperAPI, url.PathEscape(paperID), limit)

	req, err := http.NewRequest("GET", endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("citations request: %w", err)
	}
	req.Header.Set("User-Agent", "KAE-Bot/1.0 (knowledge-archaeology-engine)")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("citations fetch: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("citations returned %d for %s", resp.StatusCode, paperID)
	}

	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var response struct {
		Data []struct {
			CitingPaper struct {
				PaperID  string `json:"paperId"`
				Title    string `json:"title"`
				Abstract string `json:"abstract"`
				TLDR     *struct {
					Text string `json:"text"`
				} `json:"tldr"`
				Year          int    `json:"year"`
				URL           string `json:"url"`
				CitationCount int    `json:"citationCount"`
			} `json:"citingPaper"`
		} `json:"data"`
	}

	if err := json.Unmarshal(b, &response); err != nil {
		return nil, fmt.Errorf("citations parse: %w", err)
	}

	papers := make([]*SemanticPaper, 0, len(response.Data))
	for _, d := range response.Data {
		cp := d.CitingPaper
		if cp.Title == "" {
			continue
		}
		p := &SemanticPaper{
			ID:            cp.PaperID,
			Title:         strings.TrimSpace(cp.Title),
			Abstract:      strings.TrimSpace(cp.Abstract),
			Year:          cp.Year,
			URL:           cp.URL,
			CitationCount: cp.CitationCount,
		}
		if cp.TLDR != nil {
			p.TLDR = strings.TrimSpace(cp.TLDR.Text)
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
