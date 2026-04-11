// Package ensemble fans a prompt out to multiple LLM providers in parallel
// and measures controversy — how much the models disagree with each other.
package ensemble

import (
	"strings"
	"sync"

	"github.com/meistro57/kae/internal/llm"
)

// ModelResponse holds the raw text output from one provider.
type ModelResponse struct {
	Model    string
	Output   string
	concepts []string // extracted for Jaccard comparison
}

// Result is the combined output of an ensemble run.
type Result struct {
	Responses   []ModelResponse
	Controversy float64  // 0 = full consensus, 1 = maximum disagreement
	Merged      string   // all outputs concatenated with headers
	Dissenting  []string // model names that diverged significantly
}

// Ensemble fans a single prompt out to multiple Providers.
type Ensemble struct {
	providers []llm.Provider
}

// New creates an Ensemble from a slice of providers.
func New(providers []llm.Provider) *Ensemble {
	return &Ensemble{providers: providers}
}

// Run sends the same prompt to every provider in parallel and returns a Result.
func (e *Ensemble) Run(system string, msgs []llm.Message) *Result {
	var mu sync.Mutex
	var wg sync.WaitGroup
	responses := make([]ModelResponse, 0, len(e.providers))

	for _, p := range e.providers {
		p := p
		wg.Add(1)
		go func() {
			defer wg.Done()
			var sb strings.Builder
			for chunk := range p.Stream(system, msgs) {
				if chunk.Type == llm.ChunkText {
					sb.WriteString(chunk.Text)
				}
			}
			text := sb.String()
			mu.Lock()
			responses = append(responses, ModelResponse{
				Model:    p.ModelName(),
				Output:   text,
				concepts: extractConcepts(text),
			})
			mu.Unlock()
		}()
	}
	wg.Wait()

	return &Result{
		Responses:   responses,
		Controversy: controversyScore(responses),
		Merged:      mergeOutputs(responses),
		Dissenting:  findDissenters(responses),
	}
}

// controversyScore returns a value in [0,1] measuring how much the models
// disagree.  It computes 1 − Jaccard(concepts) for every pair and averages.
func controversyScore(responses []ModelResponse) float64 {
	if len(responses) < 2 {
		return 0
	}
	total := 0.0
	pairs := 0
	for i := 0; i < len(responses); i++ {
		for j := i + 1; j < len(responses); j++ {
			sim := jaccardSim(responses[i].concepts, responses[j].concepts)
			total += 1.0 - sim
			pairs++
		}
	}
	if pairs == 0 {
		return 0
	}
	return total / float64(pairs)
}

func jaccardSim(a, b []string) float64 {
	setA := make(map[string]bool, len(a))
	for _, w := range a {
		setA[w] = true
	}
	inter := 0
	for _, w := range b {
		if setA[w] {
			inter++
		}
	}
	union := len(setA) + len(b) - inter
	if union == 0 {
		return 1.0
	}
	return float64(inter) / float64(union)
}

// extractConcepts returns a deduplicated list of meaningful words (len>4,
// not a stop word) from text for Jaccard comparison.
func extractConcepts(text string) []string {
	stop := map[string]bool{
		"the": true, "a": true, "an": true, "is": true, "are": true,
		"was": true, "be": true, "been": true, "being": true, "have": true,
		"has": true, "had": true, "does": true, "did": true, "will": true,
		"would": true, "could": true, "should": true, "may": true, "might": true,
		"must": true, "can": true, "to": true, "of": true, "in": true,
		"on": true, "at": true, "by": true, "for": true, "with": true,
		"from": true, "or": true, "and": true, "but": true, "not": true,
		"this": true, "that": true, "it": true, "its": true, "we": true,
		"they": true, "their": true, "also": true, "which": true, "when": true,
	}
	words := strings.Fields(strings.ToLower(text))
	seen := make(map[string]bool)
	var out []string
	for _, w := range words {
		w = strings.Trim(w, `.,;:!?"'()[]{}-`)
		if len(w) > 4 && !stop[w] && !seen[w] {
			seen[w] = true
			out = append(out, w)
		}
	}
	return out
}

// mergeOutputs joins all responses with model-name headers.
func mergeOutputs(responses []ModelResponse) string {
	if len(responses) == 0 {
		return ""
	}
	if len(responses) == 1 {
		return responses[0].Output
	}
	var sb strings.Builder
	for _, r := range responses {
		sb.WriteString("=== ")
		sb.WriteString(r.Model)
		sb.WriteString(" ===\n")
		out := r.Output
		if len(out) > 600 {
			out = out[:600] + "…"
		}
		sb.WriteString(out)
		sb.WriteString("\n\n")
	}
	return strings.TrimRight(sb.String(), "\n")
}

// findDissenters returns models whose concept overlap with the majority is
// significantly lower than average — only meaningful with ≥3 providers.
func findDissenters(responses []ModelResponse) []string {
	if len(responses) < 3 {
		return nil
	}
	sims := make([]float64, len(responses))
	for i, r := range responses {
		total := 0.0
		for j, other := range responses {
			if i == j {
				continue
			}
			total += jaccardSim(r.concepts, other.concepts)
		}
		sims[i] = total / float64(len(responses)-1)
	}
	mean := 0.0
	for _, s := range sims {
		mean += s
	}
	mean /= float64(len(sims))

	var out []string
	for i, s := range sims {
		if s < mean*0.6 {
			out = append(out, responses[i].Model)
		}
	}
	return out
}
