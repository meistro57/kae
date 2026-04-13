package ingestion

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
)

const coreAPI = "https://api.core.ac.uk/v3/search/works"

// CorePaper is a paper returned by the CORE API.
type CorePaper struct {
	ID          string
	Title       string
	Abstract    string
	DownloadURL string
	DOI         string
}

// CoreSearch returns open-access papers from CORE for a topic.
// Requires CORE_API_KEY env var — returns nil,nil if key is absent so
// the caller can silently skip this source without failing the run.
// CORE is the world's largest open-access aggregator and provides
// full-text access, making its abstracts far denser than arXiv's.
func CoreSearch(topic string, limit int) ([]*CorePaper, error) {
	apiKey := os.Getenv("CORE_API_KEY")
	if apiKey == "" {
		return nil, nil // key not configured — skip gracefully
	}

	params := url.Values{
		"q":      {topic},
		"limit":  {fmt.Sprintf("%d", limit)},
		"fields": {"id,title,abstract,downloadUrl,doi"},
	}

	req, err := http.NewRequest("GET", coreAPI+"?"+params.Encode(), nil)
	if err != nil {
		return nil, fmt.Errorf("core request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("User-Agent", "KAE-Bot/1.0 (knowledge-archaeology-engine)")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("core fetch: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == 401 {
		return nil, fmt.Errorf("core: invalid API key")
	}
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("core returned %d", resp.StatusCode)
	}

	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var response struct {
		Results []struct {
			ID          string `json:"id"`
			Title       string `json:"title"`
			Abstract    string `json:"abstract"`
			DownloadURL string `json:"downloadUrl"`
			DOI         string `json:"doi"`
		} `json:"results"`
	}

	if err := json.Unmarshal(b, &response); err != nil {
		return nil, fmt.Errorf("core parse: %w", err)
	}

	papers := make([]*CorePaper, 0, len(response.Results))
	for _, r := range response.Results {
		if r.Title == "" || r.Abstract == "" {
			continue
		}
		papers = append(papers, &CorePaper{
			ID:          r.ID,
			Title:       strings.TrimSpace(r.Title),
			Abstract:    strings.TrimSpace(r.Abstract),
			DownloadURL: r.DownloadURL,
			DOI:         r.DOI,
		})
	}

	return papers, nil
}

// CorePaperToChunks converts a CorePaper to embeddable chunks.
func CorePaperToChunks(p *CorePaper) []string {
	source := p.DOI
	if source == "" {
		source = p.DownloadURL
	}
	full := fmt.Sprintf("Title: %s\n\nAbstract: %s", p.Title, p.Abstract)
	_ = source
	return Chunk(full, 200, 30)
}
