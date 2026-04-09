package ingestion

import (
	"fmt"
	"io"
	"net/http"
	"strings"
)

type GutenbergBook struct {
	ID      int
	Title   string
	Authors []string
	TextURL string
}

// GutenbergFetch fetches a known Gutenberg text directly by ID
// These are curated IDs for texts relevant to knowledge archaeology
func GutenbergFetch(bookID int, title string) (*GutenbergBook, error) {
	// Try plain text UTF-8 first, then ASCII
	urls := []string{
		fmt.Sprintf("https://www.gutenberg.org/cache/epub/%d/pg%d.txt", bookID, bookID),
		fmt.Sprintf("https://www.gutenberg.org/files/%d/%d-0.txt", bookID, bookID),
		fmt.Sprintf("https://www.gutenberg.org/files/%d/%d.txt", bookID, bookID),
	}

	for _, url := range urls {
		resp, err := http.Get(url)
		if err != nil || resp.StatusCode != 200 {
			if resp != nil {
				resp.Body.Close()
			}
			continue
		}
		defer resp.Body.Close()
		b, err := io.ReadAll(resp.Body)
		if err != nil {
			continue
		}
		text := stripGutenbergBoilerplate(string(b))
		if len(text) > 500 {
			return &GutenbergBook{
				ID:      bookID,
				Title:   title,
				TextURL: url,
			}, nil
		}
	}
	return nil, fmt.Errorf("could not fetch book %d", bookID)
}

// KAEBookList is a curated list of Gutenberg texts relevant to knowledge archaeology
// Format: {id, title}
var KAEBookList = []struct {
	ID    int
	Title string
}{
	{2680, "Meditations - Marcus Aurelius"},
	{1080, "The Republic - Plato"},
	{1497, "The Laws - Plato"},
	{3296, "Timaeus - Plato"},          // Plato's cosmology
	{8800, "The Upanishads"},
	{2411, "The Yoga Sutras of Patanjali"},
	{17921, "The Kybalion - Three Initiates"}, // Hermetic philosophy
	{974,  "The Gospel of Thomas"},
	{6748, "The Book of Enoch"},
	{3438, "Ecclesiastes"},
	{1404, "The Emerald Tablet"},
}

// BooksForTopic returns book IDs most relevant to a topic
func BooksForTopic(topic string) []struct{ ID int; Title string } {
	topic = strings.ToLower(topic)
	var relevant []struct{ ID int; Title string }

	keywords := map[string][]int{
		"cosmolog": {3296, 8800, 17921},
		"conscious": {2411, 8800, 2680},
		"ancient":   {3296, 6748, 8800, 17921},
		"philosoph": {1080, 1497, 3296, 2680},
		"hermeti":   {17921, 1404},
		"vedic":     {8800, 2411},
		"quantum":   {17921, 8800},
		"void":      {8800, 3296, 6748},
		"enoch":     {6748},
	}

	seen := make(map[int]bool)
	for keyword, ids := range keywords {
		if strings.Contains(topic, keyword) {
			for _, id := range ids {
				if !seen[id] {
					seen[id] = true
					for _, book := range KAEBookList {
						if book.ID == id {
							relevant = append(relevant, book)
						}
					}
				}
			}
		}
	}

	// Default: return first 2 from list
	if len(relevant) == 0 {
		return KAEBookList[:2]
	}
	return relevant
}

func FetchBookText(book *GutenbergBook, maxWords int) (string, error) {
	resp, err := http.Get(book.TextURL)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	text := stripGutenbergBoilerplate(string(b))
	words := strings.Fields(text)
	if len(words) > maxWords {
		words = words[:maxWords]
	}
	return strings.Join(words, " "), nil
}

func BookToChunks(book *GutenbergBook, text string) []string {
	header := fmt.Sprintf("From: %s\n\n", book.Title)
	return Chunk(header+text, 300, 50)
}

func stripGutenbergBoilerplate(text string) string {
	startMarkers := []string{
		"*** START OF THE PROJECT GUTENBERG",
		"*** START OF THIS PROJECT GUTENBERG",
	}
	for _, marker := range startMarkers {
		if idx := strings.Index(text, marker); idx >= 0 {
			lineEnd := strings.Index(text[idx:], "\n")
			if lineEnd >= 0 {
				text = text[idx+lineEnd+1:]
			}
		}
	}
	endMarkers := []string{
		"*** END OF THE PROJECT GUTENBERG",
		"*** END OF THIS PROJECT GUTENBERG",
	}
	for _, marker := range endMarkers {
		if idx := strings.Index(text, marker); idx >= 0 {
			text = text[:idx]
		}
	}
	return strings.TrimSpace(text)
}
