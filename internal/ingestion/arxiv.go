package ingestion

import (
	"encoding/xml"
	"fmt"
	"io"
	"net/url"
	"strings"
	"time"
)

const arxivAPI = "https://export.arxiv.org/api/query"

// ArxivResult holds a single paper's relevant fields.
type ArxivResult struct {
	Title    string
	Abstract string
	Authors  []string
	URL      string
	Published time.Time
}

// ArxivSearch queries the arxiv API and returns up to maxResults papers for topic.
func ArxivSearch(topic string, maxResults int) ([]ArxivResult, error) {
	params := url.Values{
		"search_query": {fmt.Sprintf("all:%s", topic)},
		"max_results":  {fmt.Sprintf("%d", maxResults)},
		"sortBy":       {"relevance"},
		"sortOrder":    {"descending"},
	}

	resp, err := httpClient.Get(arxivAPI + "?" + params.Encode())
	if err != nil {
		return nil, fmt.Errorf("arxiv fetch: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("arxiv: HTTP %d", resp.StatusCode)
	}

	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	return parseArxivFeed(b)
}

// ArxivDigest returns a single string combining the abstracts of the top papers,
// suitable for injection into an LLM prompt.
func ArxivDigest(results []ArxivResult) string {
	if len(results) == 0 {
		return ""
	}
	var sb strings.Builder
	for i, r := range results {
		sb.WriteString(fmt.Sprintf("[arxiv paper %d] %s\n", i+1, r.Title))
		if len(r.Authors) > 0 {
			sb.WriteString(fmt.Sprintf("Authors: %s\n", strings.Join(r.Authors, ", ")))
		}
		sb.WriteString(fmt.Sprintf("Abstract: %s\n\n", strings.TrimSpace(r.Abstract)))
	}
	return strings.TrimSpace(sb.String())
}

// ── Atom XML parsing ──────────────────────────────────────────────────────────

type atomFeed struct {
	Entries []atomEntry `xml:"entry"`
}

type atomEntry struct {
	Title     string       `xml:"title"`
	Summary   string       `xml:"summary"`
	Authors   []atomAuthor `xml:"author"`
	Published string       `xml:"published"`
	Links     []atomLink   `xml:"link"`
}

type atomAuthor struct {
	Name string `xml:"name"`
}

type atomLink struct {
	Href string `xml:"href,attr"`
	Rel  string `xml:"rel,attr"`
	Type string `xml:"type,attr"`
}

func parseArxivFeed(data []byte) ([]ArxivResult, error) {
	var feed atomFeed
	if err := xml.Unmarshal(data, &feed); err != nil {
		return nil, fmt.Errorf("arxiv parse: %w", err)
	}

	results := make([]ArxivResult, 0, len(feed.Entries))
	for _, e := range feed.Entries {
		if strings.TrimSpace(e.Summary) == "" {
			continue // skip entries with no abstract (error entries from the API)
		}

		r := ArxivResult{
			Title:    strings.TrimSpace(e.Title),
			Abstract: strings.TrimSpace(e.Summary),
		}

		for _, a := range e.Authors {
			r.Authors = append(r.Authors, a.Name)
		}

		// prefer the HTML link; fall back to first link
		for _, l := range e.Links {
			if l.Type == "text/html" {
				r.URL = l.Href
				break
			}
		}
		if r.URL == "" && len(e.Links) > 0 {
			r.URL = e.Links[0].Href
		}

		if e.Published != "" {
			r.Published, _ = time.Parse(time.RFC3339, e.Published)
		}

		results = append(results, r)
	}

	return results, nil
}
