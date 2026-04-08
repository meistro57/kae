package ingestion

import (
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"strings"
)

const wikiAPI = "https://en.wikipedia.org/w/api.php"

type WikiResult struct {
	Title   string
	Extract string
	URL     string
}

// WikiSummary fetches a plain-text extract for a topic
func WikiSummary(topic string) (*WikiResult, error) {
	params := url.Values{
		"action":        {"query"},
		"format":        {"json"},
		"titles":        {topic},
		"prop":          {"extracts|info"},
		"exintro":       {"true"},
		"explaintext":   {"true"},
		"inprop":        {"url"},
		"redirects":     {"1"},
	}

	resp, err := httpClient.Get(wikiAPI + "?" + params.Encode())
	if err != nil {
		return nil, fmt.Errorf("wiki fetch: %w", err)
	}
	defer resp.Body.Close()

	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var result struct {
		Query struct {
			Pages map[string]struct {
				Title   string `json:"title"`
				Extract string `json:"extract"`
				FullURL string `json:"fullurl"`
			} `json:"pages"`
		} `json:"query"`
	}

	if err := json.Unmarshal(b, &result); err != nil {
		return nil, fmt.Errorf("wiki parse: %w", err)
	}

	for _, page := range result.Query.Pages {
		if page.Extract == "" {
			return nil, fmt.Errorf("no extract for %q", topic)
		}
		return &WikiResult{
			Title:   page.Title,
			Extract: page.Extract,
			URL:     page.FullURL,
		}, nil
	}

	return nil, fmt.Errorf("no pages returned for %q", topic)
}

// Chunk splits text into overlapping chunks for embedding
func Chunk(text string, size, overlap int) []string {
	words := strings.Fields(text)
	var chunks []string
	for i := 0; i < len(words); i += size - overlap {
		end := i + size
		if end > len(words) {
			end = len(words)
		}
		chunks = append(chunks, strings.Join(words[i:end], " "))
		if end == len(words) {
			break
		}
	}
	return chunks
}
