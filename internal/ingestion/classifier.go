package ingestion

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/meistro57/kae/internal/llm"
)

// DomainResult is the classification result for a single chunk
type DomainResult struct {
	Domain     string  `json:"domain"`
	Confidence float64 `json:"confidence"`
}

// ClassifyDomainBatch classifies semantic domains for a batch of text chunks
// in a single LLM call. texts and sources must be the same length.
// Falls back to heuristic inference when the LLM returns incomplete results.
func ClassifyDomainBatch(texts []string, sources []string, provider llm.Provider) []DomainResult {
	if len(texts) == 0 {
		return nil
	}

	var builder strings.Builder
	for i, text := range texts {
		truncated := text
		if len(truncated) > 300 {
			truncated = truncated[:300]
		}
		src := ""
		if i < len(sources) {
			src = sources[i]
		}
		fmt.Fprintf(&builder, "[%d] SOURCE: %s\nTEXT: %s\n\n", i, src, truncated)
	}

	prompt := fmt.Sprintf(`Classify the semantic domain of each text chunk below.

%s
Return ONLY a JSON array with one entry per chunk, in order:
[
  {"domain": "Roman History", "confidence": 0.95},
  {"domain": "Medical Research", "confidence": 0.88}
]

Use specific, descriptive domain names. No explanation. Output ONLY valid JSON.`, builder.String())

	response := llmGenerate(provider, "You are a precise semantic domain classifier.", prompt)

	// Extract JSON array from response
	start := strings.Index(response, "[")
	end := strings.LastIndex(response, "]")
	if start != -1 && end > start {
		var results []DomainResult
		if err := json.Unmarshal([]byte(response[start:end+1]), &results); err == nil {
			// Pad any missing entries with heuristic fallbacks
			for i := len(results); i < len(texts); i++ {
				src := ""
				if i < len(sources) {
					src = sources[i]
				}
				results = append(results, DomainResult{Domain: inferDomainFromSource(src), Confidence: 0.5})
			}
			return results
		}
	}

	return fallbackDomains(sources, len(texts))
}

// llmGenerate collects a full LLM stream into a single string.
func llmGenerate(provider llm.Provider, system, prompt string) string {
	msgs := []llm.Message{{Role: "user", Content: prompt}}
	ch := provider.Stream(system, msgs)
	var sb strings.Builder
	for chunk := range ch {
		if chunk.Type == llm.ChunkText {
			sb.WriteString(chunk.Text)
		}
	}
	return sb.String()
}

func fallbackDomains(sources []string, n int) []DomainResult {
	out := make([]DomainResult, n)
	for i := range out {
		src := ""
		if i < len(sources) {
			src = sources[i]
		}
		out[i] = DomainResult{Domain: inferDomainFromSource(src), Confidence: 0.5}
	}
	return out
}

// inferDomainFromSource extracts domain from source URL/title using heuristics
func inferDomainFromSource(source string) string {
	sourceLower := strings.ToLower(source)

	// Academic sources
	if strings.Contains(sourceLower, "pubmed") {
		return "Medical Research"
	}
	if strings.Contains(sourceLower, "arxiv.org") {
		if strings.Contains(sourceLower, "cs.") {
			return "Computer Science"
		}
		if strings.Contains(sourceLower, "physics") || strings.Contains(sourceLower, "quant-ph") {
			return "Physics"
		}
		if strings.Contains(sourceLower, "math") {
			return "Mathematics"
		}
		return "Academic Research"
	}

	// Books
	if strings.Contains(sourceLower, "meditations") && strings.Contains(sourceLower, "aurelius") {
		return "Stoic Philosophy"
	}
	if strings.Contains(sourceLower, "kybalion") {
		return "Hermetic Philosophy"
	}

	// Encyclopedic
	if strings.Contains(sourceLower, "wikipedia") {
		return "Encyclopedia"
	}

	// Repositories
	if strings.Contains(sourceLower, "github") {
		return "Software Development"
	}

	// Legal/Government
	if strings.Contains(sourceLower, ".gov") {
		return "Government Documentation"
	}

	// Default
	return "General Knowledge"
}

// ValidateDomain checks if a domain string is reasonable
func ValidateDomain(domain string) bool {
	// Basic sanity checks
	if domain == "" {
		return false
	}
	if len(domain) > 100 {
		return false
	}
	// Should not contain special chars that indicate parsing errors
	if strings.Contains(domain, "{") || strings.Contains(domain, "}") {
		return false
	}
	return true
}
