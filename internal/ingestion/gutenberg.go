package ingestion

import (
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"strings"
)

const (
	gutendexAPI    = "https://gutendex.com/books"
	gutenbergBase  = "https://www.gutenberg.org/ebooks"
	excerptBytes   = 4096 // max bytes to fetch from a raw text file
	gutenbergHeaderEnd = "*** START OF" // standard Gutenberg header delimiter
)

// GutenbergResult holds metadata for a single Project Gutenberg book.
type GutenbergResult struct {
	ID       int
	Title    string
	Authors  []string
	Subjects []string
	URL      string // canonical Gutenberg ebook page
	TextURL  string // direct plain-text URL (may be empty)
}

// GutenbergSearch queries the Gutendex API and returns up to maxResults books for topic.
func GutenbergSearch(topic string, maxResults int) ([]GutenbergResult, error) {
	params := url.Values{
		"search": {topic},
	}

	resp, err := httpClient.Get(gutendexAPI + "?" + params.Encode())
	if err != nil {
		return nil, fmt.Errorf("gutenberg fetch: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("gutenberg: HTTP %d", resp.StatusCode)
	}

	var result struct {
		Results []struct {
			ID      int    `json:"id"`
			Title   string `json:"title"`
			Authors []struct {
				Name string `json:"name"`
			} `json:"authors"`
			Subjects []string          `json:"subjects"`
			Formats  map[string]string `json:"formats"`
		} `json:"results"`
	}

	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(b, &result); err != nil {
		return nil, fmt.Errorf("gutenberg parse: %w", err)
	}

	out := make([]GutenbergResult, 0, maxResults)
	for _, r := range result.Results {
		if len(out) >= maxResults {
			break
		}

		g := GutenbergResult{
			ID:    r.ID,
			Title: r.Title,
			URL:   fmt.Sprintf("%s/%d", gutenbergBase, r.ID),
		}

		for _, a := range r.Authors {
			g.Authors = append(g.Authors, a.Name)
		}
		g.Subjects = r.Subjects

		// pick the best plain-text URL: prefer UTF-8, then ASCII
		for _, mime := range []string{
			"text/plain; charset=utf-8",
			"text/plain; charset=us-ascii",
			"text/plain",
		} {
			if u, ok := r.Formats[mime]; ok {
				g.TextURL = u
				break
			}
		}

		out = append(out, g)
	}

	return out, nil
}

// GutenbergExcerpt fetches a short excerpt from a Gutenberg plain-text file,
// stripping the standard boilerplate header. Returns empty string on any error.
func GutenbergExcerpt(textURL string) string {
	if textURL == "" {
		return ""
	}

	resp, err := httpClient.Get(textURL)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()

	raw := make([]byte, excerptBytes*3) // fetch extra to account for the header
	n, _ := io.ReadFull(resp.Body, raw)
	text := string(raw[:n])

	// strip everything up to and including the START marker
	if idx := strings.Index(text, gutenbergHeaderEnd); idx != -1 {
		// advance past the marker line
		rest := text[idx:]
		if nl := strings.Index(rest, "\n"); nl != -1 {
			text = rest[nl+1:]
		}
	}

	text = strings.TrimSpace(text)
	if len(text) > excerptBytes {
		text = text[:excerptBytes] + "..."
	}
	return text
}

// GutenbergDigest returns a concise string of book metadata plus optional excerpts,
// suitable for injection into an LLM prompt.
func GutenbergDigest(results []GutenbergResult, withExcerpts bool) string {
	if len(results) == 0 {
		return ""
	}
	var sb strings.Builder
	for i, r := range results {
		sb.WriteString(fmt.Sprintf("[Gutenberg %d] %s", i+1, r.Title))
		if len(r.Authors) > 0 {
			sb.WriteString(fmt.Sprintf(" — %s", strings.Join(r.Authors, ", ")))
		}
		sb.WriteString("\n")
		if len(r.Subjects) > 0 {
			// cap subjects to avoid noise
			subjects := r.Subjects
			if len(subjects) > 5 {
				subjects = subjects[:5]
			}
			sb.WriteString(fmt.Sprintf("Subjects: %s\n", strings.Join(subjects, "; ")))
		}
		sb.WriteString(fmt.Sprintf("URL: %s\n", r.URL))

		if withExcerpts && r.TextURL != "" {
			excerpt := GutenbergExcerpt(r.TextURL)
			if excerpt != "" {
				sb.WriteString(fmt.Sprintf("Excerpt:\n%s\n", excerpt))
			}
		}
		sb.WriteString("\n")
	}
	return strings.TrimSpace(sb.String())
}
