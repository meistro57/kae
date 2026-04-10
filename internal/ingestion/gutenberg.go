package ingestion

import (
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
)

type GutenbergBook struct {
	ID      int
	Title   string
	Authors []string
	TextURL string
}

// KAEBookList — verified correct Gutenberg IDs
var KAEBookList = []struct {
	ID    int
	Title string
}{
	{55201, "The Republic - Plato"},
	{1572, "Timaeus - Plato"},                 // Plato's cosmology
	{14209, "The Kybalion - Three Initiates"}, // Hermetic philosophy
	{2680, "Meditations - Marcus Aurelius"},
	{3438, "Ecclesiastes"},
	{6748, "The Book of Enoch"},
	{2411, "The Yoga Sutras of Patanjali"},
	{48926, "The Upanishads"},
	{45977, "The Emerald Tablet"},
	{1396, "Phaedo - Plato"}, // Soul and immortality
	{1656, "The Symposium - Plato"},
	{3207, "Tao Te Ching - Lao Tzu"},
}

// BooksForTopic returns relevant books for a topic
func BooksForTopic(topic string) []struct {
	ID    int
	Title string
} {
	topic = strings.ToLower(topic)

	keywords := map[string][]int{
		"cosmolog":  {1572, 14209, 55201, 6748},
		"conscious": {2411, 48926, 2680, 1396},
		"ancient":   {1572, 6748, 48926, 14209, 3207},
		"philosoph": {55201, 1396, 1656, 2680, 1572},
		"hermeti":   {14209, 45977},
		"vedic":     {48926, 2411},
		"quantum":   {14209, 48926, 2680},
		"void":      {48926, 1572, 6748, 3207},
		"soul":      {1396, 48926, 2411, 6748},
		"reincarn":  {1396, 48926, 2411, 55201},
		"mana":      {14209, 48926, 3207},
		"observer":  {2680, 14209, 48926},
		"tao":       {3207, 14209},
		"enoch":     {6748},
		"firmament": {6748, 1572, 3438},
	}

	type scoredBook struct {
		ID    int
		Score int
	}

	scoreByID := make(map[int]int)
	for keyword, ids := range keywords {
		if strings.Contains(topic, keyword) {
			for rank, id := range ids {
				// Higher-ranked ids in a keyword list get a stronger signal.
				scoreByID[id] += len(ids) - rank
			}
		}
	}

	if len(scoreByID) == 0 {
		// Default: Kybalion + Meditations — good general sources
		return []struct {
			ID    int
			Title string
		}{
			KAEBookList[2],
			KAEBookList[3],
		}
	}

	positionByID := make(map[int]int, len(KAEBookList))
	for i, book := range KAEBookList {
		positionByID[book.ID] = i
	}

	scored := make([]scoredBook, 0, len(scoreByID))
	for id, score := range scoreByID {
		scored = append(scored, scoredBook{ID: id, Score: score})
	}

	sort.Slice(scored, func(i, j int) bool {
		if scored[i].Score == scored[j].Score {
			return positionByID[scored[i].ID] < positionByID[scored[j].ID]
		}
		return scored[i].Score > scored[j].Score
	})

	relevant := make([]struct {
		ID    int
		Title string
	}, 0, 3)
	for _, candidate := range scored {
		for _, book := range KAEBookList {
			if book.ID == candidate.ID {
				relevant = append(relevant, book)
				break
			}
		}
		if len(relevant) == 3 {
			break
		}
	}

	return relevant
}

func GutenbergFetch(bookID int, title string) (*GutenbergBook, error) {
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
			return &GutenbergBook{ID: bookID, Title: title, TextURL: url}, nil
		}
	}
	return nil, fmt.Errorf("could not fetch book %d", bookID)
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
