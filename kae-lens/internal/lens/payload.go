package lens

import (
	"strings"

	qdrantclient "github.com/meistro/kae/internal/qdrantclient"
	"github.com/qdrant/go-client/qdrant"
)

// stringFromMap returns the first non-empty string value found for the given
// keys, checked in the order provided. Key order therefore defines precedence:
// the first key whose value is a non-empty string wins.
func stringFromMap(m map[string]any, keys ...string) string {
	for _, k := range keys {
		if v, ok := m[k]; ok {
			if s, ok := v.(string); ok && s != "" {
				return s
			}
		}
	}
	return ""
}

// neighborSummaryFromPoint decodes a scored point's payload into a
// neighborSummary, accepting both chunk schemas KAE writes:
//
//	kae_knowledge: title / content / domain / url
//	kae_chunks:    source / text / semantic_domain / source
//
// Precedence when both schemas are present:
//
//	body:   "content" then "text"
//	domain: "domain" then "semantic_domain" then "topic"
//	title:  "title" then "source"
//	url:    "url" then "source"
func neighborSummaryFromPoint(p *qdrant.ScoredPoint) neighborSummary {
	payload := qdrantclient.PayloadToMap(p.Payload)

	content := stringFromMap(payload, "content", "text")
	if len(content) > 400 {
		content = content[:400] + "..."
	}

	srcURL := stringFromMap(payload, "url", "source")
	url := ""
	if strings.HasPrefix(srcURL, "http://") || strings.HasPrefix(srcURL, "https://") {
		url = srcURL
	}

	return neighborSummary{
		ID:      qdrantclient.PointIDStr(p.Id),
		Score:   p.Score,
		Title:   stringFromMap(payload, "title", "source"),
		Domain:  stringFromMap(payload, "domain", "semantic_domain", "topic"),
		Content: content,
		URL:     url,
	}
}
