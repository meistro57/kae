package ingestion

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
)

const openAlexAPI = "https://api.openalex.org/works"

// OpenAlexWork is a scholarly work returned by the OpenAlex API.
type OpenAlexWork struct {
	Title    string
	Abstract string
	DOI      string
	URL      string
	Concepts []string // high-level OpenAlex concept tags
}

// OpenAlexSearch returns works matching topic from OpenAlex.
// No API key required. The polite pool (mailto param) increases rate limits.
// OpenAlex is particularly valuable because it tags works with broad
// philosophical/scientific "Concepts" that help KAE map cross-domain links.
func OpenAlexSearch(topic string, limit int) ([]*OpenAlexWork, error) {
	params := url.Values{
		"search":   {topic},
		"per-page": {fmt.Sprintf("%d", limit)},
		"select":   {"title,abstract_inverted_index,doi,primary_location,concepts"},
		"mailto":   {"kae-bot@localhost"},
	}

	req, err := http.NewRequest("GET", openAlexAPI+"?"+params.Encode(), nil)
	if err != nil {
		return nil, fmt.Errorf("openalex request: %w", err)
	}
	req.Header.Set("User-Agent", "KAE-Bot/1.0 (knowledge-archaeology-engine)")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("openalex fetch: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("openalex returned %d", resp.StatusCode)
	}

	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var response struct {
		Results []struct {
			Title                 string           `json:"title"`
			AbstractInvertedIndex map[string][]int `json:"abstract_inverted_index"`
			DOI                   string           `json:"doi"`
			PrimaryLocation       *struct {
				LandingPageURL string `json:"landing_page_url"`
			} `json:"primary_location"`
			Concepts []struct {
				DisplayName string  `json:"display_name"`
				Score       float64 `json:"score"`
			} `json:"concepts"`
		} `json:"results"`
	}

	if err := json.Unmarshal(b, &response); err != nil {
		return nil, fmt.Errorf("openalex parse: %w", err)
	}

	works := make([]*OpenAlexWork, 0, len(response.Results))
	for _, r := range response.Results {
		if r.Title == "" {
			continue
		}
		w := &OpenAlexWork{
			Title:    strings.TrimSpace(r.Title),
			Abstract: reconstructAbstract(r.AbstractInvertedIndex),
			DOI:      r.DOI,
		}
		if r.PrimaryLocation != nil {
			w.URL = r.PrimaryLocation.LandingPageURL
		}
		if w.URL == "" {
			w.URL = r.DOI
		}
		// Collect concept names (top 5 by score)
		type scoredConcept struct {
			name  string
			score float64
		}
		sc := make([]scoredConcept, 0, len(r.Concepts))
		for _, c := range r.Concepts {
			sc = append(sc, scoredConcept{c.DisplayName, c.Score})
		}
		sort.Slice(sc, func(i, j int) bool { return sc[i].score > sc[j].score })
		for i, c := range sc {
			if i >= 5 {
				break
			}
			w.Concepts = append(w.Concepts, c.name)
		}
		works = append(works, w)
	}

	return works, nil
}

// OpenAlexWorkToChunks converts a work to embeddable chunks.
// Includes concept tags so KAE's graph picks up cross-domain signals.
func OpenAlexWorkToChunks(w *OpenAlexWork) []string {
	var parts []string
	parts = append(parts, fmt.Sprintf("Title: %s", w.Title))
	if len(w.Concepts) > 0 {
		parts = append(parts, fmt.Sprintf("Concepts: %s", strings.Join(w.Concepts, ", ")))
	}
	if w.Abstract != "" {
		parts = append(parts, fmt.Sprintf("Abstract: %s", w.Abstract))
	}
	full := strings.Join(parts, "\n\n")
	return Chunk(full, 200, 30)
}

// reconstructAbstract rebuilds a plain-text abstract from OpenAlex's
// inverted index format (word → list of positions).
func reconstructAbstract(inv map[string][]int) string {
	if len(inv) == 0 {
		return ""
	}

	type posWord struct {
		pos  int
		word string
	}

	words := make([]posWord, 0, len(inv)*2)
	for word, positions := range inv {
		for _, pos := range positions {
			words = append(words, posWord{pos, word})
		}
	}

	sort.Slice(words, func(i, j int) bool { return words[i].pos < words[j].pos })

	result := make([]string, len(words))
	for i, pw := range words {
		result[i] = pw.word
	}
	return strings.Join(result, " ")
}
