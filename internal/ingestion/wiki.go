package ingestion

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

const wikiAPI = "https://en.wikipedia.org/w/api.php"

type WikiResult struct {
	Title   string
	Extract string
	URL     string
}

func WikiSummary(topic string) (*WikiResult, error) {
	// Search for best matching title first
	title, err := wikiSearchTitle(topic)
	if err != nil {
		return nil, err
	}
	return wikiExtract(title)
}

func wikiSearchTitle(topic string) (string, error) {
	params := url.Values{
		"action":   {"query"},
		"format":   {"json"},
		"list":     {"search"},
		"srsearch": {topic},
		"srlimit":  {"1"},
	}
	client := &http.Client{}
	req, _ := http.NewRequest("GET", wikiAPI+"?"+params.Encode(), nil)
	req.Header.Set("User-Agent", "KAE-Bot/1.0 (knowledge-archaeology-engine)")
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)

	var result struct {
		Query struct {
			Search []struct {
				Title string `json:"title"`
			} `json:"search"`
		} `json:"query"`
	}
	if err := json.Unmarshal(b, &result); err != nil {
		return "", fmt.Errorf("search parse: %w", err)
	}
	if len(result.Query.Search) == 0 {
		return "", fmt.Errorf("no results for %q", topic)
	}
	return result.Query.Search[0].Title, nil
}

func wikiExtract(title string) (*WikiResult, error) {
	params := url.Values{
		"action":          {"query"},
		"format":          {"json"},
		"titles":          {title},
		"prop":            {"extracts|info"},
		"exintro":         {"false"},
		"explaintext":     {"true"},
		"inprop":          {"url"},
		"redirects":       {"1"},
		"exsectionformat": {"plain"},
	}
	client := &http.Client{}
	req, _ := http.NewRequest("GET", wikiAPI+"?"+params.Encode(), nil)
	req.Header.Set("User-Agent", "KAE-Bot/1.0 (knowledge-archaeology-engine)")
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("wiki fetch: %w", err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)

	// Guard against HTML responses
	if len(b) > 0 && b[0] != '{' {
		return nil, fmt.Errorf("wiki returned non-JSON for %q", title)
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
			return nil, fmt.Errorf("empty extract for %q", title)
		}
		return &WikiResult{
			Title:   page.Title,
			Extract: page.Extract,
			URL:     page.FullURL,
		}, nil
	}
	return nil, fmt.Errorf("no pages for %q", title)
}

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
