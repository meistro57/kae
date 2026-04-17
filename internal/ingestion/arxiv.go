package ingestion

import (
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

const arxivAPI = "http://export.arxiv.org/api/query"

// ArxivPaper is a single paper from arxiv
type ArxivPaper struct {
	ID       string
	Title    string
	Abstract string
	Authors  []string
	URL      string
	Category string
}

// ArxivSearch fetches papers related to a topic
func ArxivSearch(topic string, maxResults int) ([]*ArxivPaper, error) {
	params := url.Values{
		"search_query": {fmt.Sprintf("all:%s", quoteIfNeeded(topic))},
		"start":        {"0"},
		"max_results":  {fmt.Sprintf("%d", maxResults)},
		"sortBy":       {"relevance"},
	}

	resp, err := http.Get(arxivAPI + "?" + params.Encode())
	if err != nil {
		return nil, fmt.Errorf("arxiv fetch: %w", err)
	}
	defer resp.Body.Close()

	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	return parseArxivFeed(b)
}

// ArxivSearchMulti searches multiple categories for a topic
// Useful for finding both mainstream and fringe papers
func ArxivSearchMulti(topic string, categories []string, maxPerCat int) ([]*ArxivPaper, error) {
	var all []*ArxivPaper
	seen := make(map[string]bool)

	for _, cat := range categories {
		query := fmt.Sprintf("cat:%s AND all:%s", cat, quoteIfNeeded(topic))
		params := url.Values{
			"search_query": {query},
			"start":        {"0"},
			"max_results":  {fmt.Sprintf("%d", maxPerCat)},
		}

		resp, err := http.Get(arxivAPI + "?" + params.Encode())
		if err != nil {
			continue // don't fail on one category
		}

		b, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			continue
		}

		papers, err := parseArxivFeed(b)
		if err != nil {
			continue
		}

		for _, p := range papers {
			if !seen[p.ID] {
				seen[p.ID] = true
				p.Category = cat
				all = append(all, p)
			}
		}
	}

	return all, nil
}

// KAEArxivCategories are the categories most relevant to cross-domain archaeology
var KAEArxivCategories = []string{
	"physics.gen-ph", // general physics — where the weird stuff lives
	"quant-ph",       // quantum physics
	"cond-mat",       // condensed matter
	"q-bio.NC",       // neurons and cognition
	"cs.AI",          // AI
	"nlin.AO",        // nonlinear dynamics, complex systems
}

func parseArxivFeed(data []byte) ([]*ArxivPaper, error) {
	var feed struct {
		XMLName xml.Name `xml:"feed"`
		Entries []struct {
			ID      string `xml:"id"`
			Title   string `xml:"title"`
			Summary string `xml:"summary"`
			Authors []struct {
				Name string `xml:"name"`
			} `xml:"author"`
			Links []struct {
				Href string `xml:"href,attr"`
				Type string `xml:"type,attr"`
			} `xml:"link"`
		} `xml:"entry"`
	}

	if err := xml.Unmarshal(data, &feed); err != nil {
		return nil, fmt.Errorf("arxiv parse: %w", err)
	}

	papers := make([]*ArxivPaper, 0, len(feed.Entries))
	for _, e := range feed.Entries {
		p := &ArxivPaper{
			ID:       e.ID,
			Title:    strings.TrimSpace(e.Title),
			Abstract: strings.TrimSpace(e.Summary),
			URL:      e.ID,
		}
		for _, a := range e.Authors {
			p.Authors = append(p.Authors, a.Name)
		}
		papers = append(papers, p)
	}

	return papers, nil
}

// PaperToChunks splits a paper into chunks for embedding
func PaperToChunks(p *ArxivPaper) []string {
	// Title + abstract is usually enough signal
	full := fmt.Sprintf("Title: %s\n\nAbstract: %s", p.Title, p.Abstract)
	return Chunk(full, 200, 30)
}
