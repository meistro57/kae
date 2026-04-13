package ingestion

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

// CitationCrawler expands a knowledge base by following citation networks.
type CitationCrawler struct {
	MaxDepth     int     // max recursion depth (default 2)
	MaxPerLevel  int     // max papers per level (default 10)
	MinRelevance float64 // cosine similarity threshold for relevance (default 0.5)
}

// NewCitationCrawler returns a CitationCrawler with sensible defaults.
func NewCitationCrawler() *CitationCrawler {
	return &CitationCrawler{MaxDepth: 2, MaxPerLevel: 10, MinRelevance: 0.5}
}

// SuppressedLineage is a paper that is highly relevant but barely cited.
type SuppressedLineage struct {
	Paper         *SemanticPaper
	Concept       string
	RelevanceNote string // why it's relevant
}

// ExpandFromPaper follows citation chains from a seed paper ID up to MaxDepth levels.
// Returns a deduplicated list of discovered papers (excluding the seed).
func (cc *CitationCrawler) ExpandFromPaper(seedPaperID string) ([]*SemanticPaper, error) {
	visited := map[string]bool{seedPaperID: true}
	var results []*SemanticPaper

	queue := []struct {
		id    string
		depth int
	}{{seedPaperID, 0}}

	for len(queue) > 0 {
		item := queue[0]
		queue = queue[1:]

		if item.depth >= cc.MaxDepth {
			continue
		}

		// Fetch references
		refs, err := GetPaperReferences(item.id, cc.MaxPerLevel)
		if err != nil {
			// Non-fatal — paper may not be in Semantic Scholar
			continue
		}
		for _, p := range refs {
			if p.ID != "" && !visited[p.ID] {
				visited[p.ID] = true
				results = append(results, p)
				queue = append(queue, struct {
					id    string
					depth int
				}{p.ID, item.depth + 1})
			}
		}

		// Fetch citations (who cited this)
		cites, err := GetPaperCitations(item.id, cc.MaxPerLevel)
		if err != nil {
			continue
		}
		for _, p := range cites {
			if p.ID != "" && !visited[p.ID] {
				visited[p.ID] = true
				results = append(results, p)
				// Don't recurse on citing papers — avoid explosion
			}
		}
	}

	return results, nil
}

// FindSuppressedLineages finds papers that are highly relevant to a concept
// but have very few citations — potential "suppressed" or overlooked work.
// concept is used as a search query; papers with CitationCount < maxCitations
// and age > minAgeYears are flagged.
func (cc *CitationCrawler) FindSuppressedLineages(concept string, maxCitations int, minAgeYears int) ([]*SuppressedLineage, error) {
	papers, err := SemanticScholarSearch(concept, 50)
	if err != nil {
		return nil, fmt.Errorf("search: %w", err)
	}

	cutoffYear := time.Now().Year() - minAgeYears

	var lineages []*SuppressedLineage
	for _, p := range papers {
		if p.CitationCount > maxCitations {
			continue
		}
		if p.Year > cutoffYear || p.Year == 0 {
			continue
		}
		note := fmt.Sprintf("Published %d, only %d citations — relevant to %q", p.Year, p.CitationCount, concept)
		lineages = append(lineages, &SuppressedLineage{
			Paper:         p,
			Concept:       concept,
			RelevanceNote: note,
		})
	}

	sort.Slice(lineages, func(i, j int) bool {
		return lineages[i].Paper.CitationCount < lineages[j].Paper.CitationCount
	})

	return lineages, nil
}

// SuppressedLineageReport generates a markdown summary of suppressed lineages.
func SuppressedLineageReport(lineages []*SuppressedLineage) string {
	if len(lineages) == 0 {
		return "No suppressed lineages found.\n"
	}
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("# Suppressed Lineages (%d papers)\n\n", len(lineages)))
	for i, l := range lineages {
		sb.WriteString(fmt.Sprintf("## %d. %s (%d)\n", i+1, l.Paper.Title, l.Paper.Year))
		sb.WriteString(fmt.Sprintf("- Citations: %d | Concept: %s\n", l.Paper.CitationCount, l.Concept))
		if l.Paper.URL != "" {
			sb.WriteString(fmt.Sprintf("- URL: %s\n", l.Paper.URL))
		}
		if l.Paper.TLDR != "" {
			sb.WriteString(fmt.Sprintf("- %s\n", l.Paper.TLDR))
		}
		sb.WriteString(fmt.Sprintf("- *%s*\n\n", l.RelevanceNote))
	}
	return sb.String()
}
