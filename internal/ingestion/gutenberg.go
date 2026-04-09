package ingestion

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"encoding/json"
)

const gutenbergAPI = "https://gutendex.com/books"

// GutenbergBook is a text from Project Gutenberg
type GutenbergBook struct {
	ID       int
	Title    string
	Authors  []string
	Subjects []string
	TextURL  string
	Language string
}

// GutenbergSearch finds books by subject/keyword
func GutenbergSearch(query string, maxResults int) ([]*GutenbergBook, error) {
	params := url.Values{
		"search":    {query},
		"languages": {"en"},
	}

	resp, err := http.Get(gutenbergAPI + "?" + params.Encode())
	if err != nil {
		return nil, fmt.Errorf("gutenberg fetch: %w", err)
	}
	defer resp.Body.Close()

	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var result struct {
		Results []struct {
			ID      int    `json:"id"`
			Title   string `json:"title"`
			Authors []struct {
				Name string `json:"name"`
			} `json:"authors"`
			Subjects  []string          `json:"subjects"`
			Formats   map[string]string `json:"formats"`
			Languages []string          `json:"languages"`
		} `json:"results"`
	}

	if err := json.Unmarshal(b, &result); err != nil {
		return nil, fmt.Errorf("gutenberg parse: %w", err)
	}

	books := make([]*GutenbergBook, 0)
	for _, r := range result.Results {
		if len(books) >= maxResults {
			break
		}

		// Find plain text URL
		textURL := ""
		for format, u := range r.Formats {
			if strings.Contains(format, "text/plain") {
				textURL = u
				break
			}
		}
		if textURL == "" {
			continue
		}

		book := &GutenbergBook{
			ID:       r.ID,
			Title:    r.Title,
			Subjects: r.Subjects,
			TextURL:  textURL,
		}
		for _, a := range r.Authors {
			book.Authors = append(book.Authors, a.Name)
		}
		if len(r.Languages) > 0 {
			book.Language = r.Languages[0]
		}
		books = append(books, book)
	}

	return books, nil
}

// FetchBookText downloads and returns the first N words of a book
func FetchBookText(book *GutenbergBook, maxWords int) (string, error) {
	resp, err := http.Get(book.TextURL)
	if err != nil {
		return "", fmt.Errorf("fetch book %d: %w", book.ID, err)
	}
	defer resp.Body.Close()

	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	text := string(b)

	// Strip Gutenberg header/footer boilerplate
	text = stripGutenbergBoilerplate(text)

	// Truncate to maxWords
	words := strings.Fields(text)
	if len(words) > maxWords {
		words = words[:maxWords]
	}

	return strings.Join(words, " "), nil
}

// KAEGutenbergTopics are the most archaeologically relevant Gutenberg search terms
var KAEGutenbergTopics = []string{
	"consciousness soul",
	"hermetic philosophy",
	"ancient cosmology",
	"vedic philosophy",
	"natural philosophy",
	"alchemy",
	"sacred geometry",
	"mystery schools",
}

func stripGutenbergBoilerplate(text string) string {
	// Find start marker
	startMarkers := []string{
		"*** START OF THE PROJECT GUTENBERG",
		"*** START OF THIS PROJECT GUTENBERG",
		"*END*THE SMALL PRINT",
	}
	for _, marker := range startMarkers {
		if idx := strings.Index(text, marker); idx >= 0 {
			// Find end of the marker line
			lineEnd := strings.Index(text[idx:], "\n")
			if lineEnd >= 0 {
				text = text[idx+lineEnd+1:]
			}
		}
	}

	// Find end marker
	endMarkers := []string{
		"*** END OF THE PROJECT GUTENBERG",
		"*** END OF THIS PROJECT GUTENBERG",
		"End of the Project Gutenberg",
	}
	for _, marker := range endMarkers {
		if idx := strings.Index(text, marker); idx >= 0 {
			text = text[:idx]
		}
	}

	return strings.TrimSpace(text)
}

// BookToChunks splits book text into chunks with metadata header
func BookToChunks(book *GutenbergBook, text string) []string {
	header := fmt.Sprintf("From: %s by %s\n\n",
		book.Title, strings.Join(book.Authors, ", "))
	full := header + text
	return Chunk(full, 300, 50)
}