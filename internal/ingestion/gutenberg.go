package ingestion

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/meistro57/kae/internal/llm"
)

type GutenbergBook struct {
	ID      int
	Title   string
	Authors []string
	TextURL string
	Formats map[string]string
}

// gutendexBook is the API response shape from gutendex.com
type gutendexBook struct {
	ID      int               `json:"id"`
	Title   string            `json:"title"`
	Formats map[string]string `json:"formats"`
}

// BlacklistEntry represents a contaminated book entry
type BlacklistEntry struct {
	Title          string `json:"title"`
	Reason         string `json:"reason"`
	DetectionDate  string `json:"detection_date"`
}

// GutenbergBlacklist structure
type GutenbergBlacklist struct {
	Version           string           `json:"version"`
	GeneratedAt       string           `json:"generated_at"`
	Reason            string           `json:"reason"`
	BlacklistedTitles []BlacklistEntry `json:"blacklisted_titles"`
}

var cachedBlacklist *GutenbergBlacklist

// loadBlacklist loads the Gutenberg blacklist from file
func loadBlacklist() (*GutenbergBlacklist, error) {
	if cachedBlacklist != nil {
		return cachedBlacklist, nil
	}

	f, err := os.Open("gutenberg_blacklist.json")
	if err != nil {
		// Blacklist file doesn't exist - return empty blacklist
		if os.IsNotExist(err) {
			return &GutenbergBlacklist{BlacklistedTitles: []BlacklistEntry{}}, nil
		}
		return nil, err
	}
	defer f.Close()

	var blacklist GutenbergBlacklist
	if err := json.NewDecoder(f).Decode(&blacklist); err != nil {
		return nil, fmt.Errorf("parsing gutenberg_blacklist.json: %w", err)
	}

	cachedBlacklist = &blacklist
	return &blacklist, nil
}

// isBlacklisted checks if a book title is in the blacklist
func isBlacklisted(title string) (bool, string) {
	blacklist, err := loadBlacklist()
	if err != nil {
		// If we can't load blacklist, don't block ingestion
		fmt.Printf("Warning: failed to load blacklist: %v\n", err)
		return false, ""
	}

	for _, entry := range blacklist.BlacklistedTitles {
		if entry.Title == title {
			return true, entry.Reason
		}
	}
	return false, ""
}

// KAEBookList — curated Gutenberg texts for knowledge archaeology
var KAEBookList = []struct {
	ID    int
	Title string
}{
	{55201, "The Republic - Plato"},
	{1572, "Timaeus - Plato"},
	{14209, "The Kybalion - Three Initiates"},
	{2680, "Meditations - Marcus Aurelius"},
	{3438, "Ecclesiastes"},
	{6748, "The Book of Enoch"},
	{2411, "The Yoga Sutras of Patanjali"},
	{48926, "The Upanishads"},
	{45977, "The Emerald Tablet"},
	{1396, "Phaedo - Plato"},
	{1656, "The Symposium - Plato"},
	{3207, "Tao Te Ching - Lao Tzu"},
}

// BooksForTopic uses LLM to select relevant books from the catalog
func BooksForTopic(topic string, llmProvider llm.Provider) []struct {
	ID    int
	Title string
} {
	// Build book list for LLM
	var bookDescriptions strings.Builder
	for i, book := range KAEBookList {
		bookDescriptions.WriteString(fmt.Sprintf("%d. %s\n", i, book.Title))
	}

	prompt := fmt.Sprintf(`You are selecting relevant ancient/philosophical texts for knowledge archaeology.

TOPIC: %s

AVAILABLE TEXTS:
%s

Return ONLY a JSON array of indices (0-based) for relevant texts. Maximum 3 texts.
If NO texts are relevant, return empty array [].

Examples:
- Topic "consciousness" → [3,6,7] (Meditations, Yoga Sutras, Upanishads)
- Topic "Pope" → [] (no relevant ancient texts)
- Topic "cosmology" → [1,2,5] (Timaeus, Kybalion, Enoch)

Return ONLY valid JSON array of numbers. No explanation.`, topic, bookDescriptions.String())

	// Collect stream into string
	var response strings.Builder
	for chunk := range llmProvider.Stream("", []llm.Message{{Role: "user", Content: prompt}}) {
		if chunk.Type == llm.ChunkText {
			response.WriteString(chunk.Text)
		}
	}

	// Parse JSON array
	responseText := strings.TrimSpace(response.String())
	var indices []int
	if err := json.Unmarshal([]byte(responseText), &indices); err != nil {
		// Fallback: no books if parse fails
		return nil
	}

	// Validate and collect books, filtering out blacklisted titles
	var selected []struct {
		ID    int
		Title string
	}

	for _, idx := range indices {
		if idx >= 0 && idx < len(KAEBookList) {
			book := KAEBookList[idx]
			
			// Check blacklist
			if blacklisted, reason := isBlacklisted(book.Title); blacklisted {
				fmt.Printf("⚠️  BLACKLIST: Skipping '%s' - %s\n", book.Title, reason)
				continue
			}
			
			selected = append(selected, book)
		}
		// Limit to 3
		if len(selected) >= 3 {
			break
		}
	}

	return selected
}

func GutenbergFetch(bookID int, title string) (*GutenbergBook, error) {
	// Check blacklist before fetching
	if blacklisted, reason := isBlacklisted(title); blacklisted {
		return nil, fmt.Errorf("book '%s' is blacklisted: %s", title, reason)
	}

	// Ask gutendex for the book metadata so we get the real text URL from formats.
	apiURL := fmt.Sprintf("https://gutendex.com/books/%d/", bookID)
	resp, err := http.Get(apiURL)
	if err != nil {
		return nil, fmt.Errorf("gutendex lookup for book %d: %w", bookID, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("gutendex returned %d for book %d", resp.StatusCode, bookID)
	}
	var meta gutendexBook
	if err := json.NewDecoder(resp.Body).Decode(&meta); err != nil {
		return nil, fmt.Errorf("gutendex decode for book %d: %w", bookID, err)
	}

	// Find the text/plain URL — key is usually "text/plain; charset=utf-8".
	textURL := ""
	for mime, u := range meta.Formats {
		if strings.HasPrefix(mime, "text/plain") {
			textURL = u
			break
		}
	}
	if textURL == "" {
		return nil, fmt.Errorf("no text/plain format for book %d", bookID)
	}

	return &GutenbergBook{
		ID:      bookID,
		Title:   title,
		TextURL: textURL,
		Formats: meta.Formats,
	}, nil
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
