package ingestion

import "strings"

// quoteIfNeeded wraps multi-word queries in quotes for exact phrase matching
// Used by arxiv and PubMed search to prevent "Holy See" from matching "Holy Grail"
func quoteIfNeeded(s string) string {
	if strings.Contains(s, " ") {
		return `"` + s + `"`
	}
	return s
}
