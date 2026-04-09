package scoring

import (
	"fmt"
	"strings"
)

// Stance represents a source's position on a claim
type Stance int

const (
	StanceSupport     Stance = iota // source explicitly supports the claim
	StanceContradict                // source explicitly contradicts the claim
	StanceSilent                    // source is relevant but says nothing about the claim
)

// Evidence is a single source's position on a claim
type Evidence struct {
	Source  string
	Stance  Stance
	Excerpt string  // the relevant passage
	Weight  float64 // source credibility weight (0.0–1.0)
}

// ContradictionScore is the result of scoring a claim against its evidence
type ContradictionScore struct {
	Claim         string
	Support       float64  // weighted sum of supporting evidence
	Contradiction float64  // weighted sum of contradicting evidence
	Silence       float64  // weighted sum of relevant-but-silent sources
	Total         float64  // total weight of all evidence considered
	AnomalyScore  float64  // 0.0–1.0, higher = more anomalous
	IsAnomaly     bool     // true if silence dominates or contradiction > support
	Explanation   string
}

// Score evaluates a claim against a set of evidence
func Score(claim string, evidence []Evidence) *ContradictionScore {
	cs := &ContradictionScore{Claim: claim}

	for _, e := range evidence {
		w := e.Weight
		if w == 0 {
			w = 1.0
		}
		cs.Total += w
		switch e.Stance {
		case StanceSupport:
			cs.Support += w
		case StanceContradict:
			cs.Contradiction += w
		case StanceSilent:
			cs.Silence += w
		}
	}

	if cs.Total == 0 {
		return cs
	}

	// Anomaly score formula:
	// High silence from mainstream = suspicious
	// Contradiction > support = suspicious
	// Both = very suspicious
	silenceRatio := cs.Silence / cs.Total
	contradictRatio := cs.Contradiction / cs.Total

	cs.AnomalyScore = (silenceRatio*0.6 + contradictRatio*0.4)

	cs.IsAnomaly = cs.AnomalyScore > 0.4 ||
		(cs.Contradiction > cs.Support && cs.Total > 2)

	cs.Explanation = explainScore(cs)
	return cs
}

// ClassifyStance uses simple heuristics to classify a passage's stance on a claim
// In Phase 3 this will be replaced by an LLM call
func ClassifyStance(claim, passage string) Stance {
	claim = strings.ToLower(claim)
	passage = strings.ToLower(passage)

	// Check if passage even mentions the claim concepts
	claimWords := strings.Fields(claim)
	relevantWords := 0
	for _, word := range claimWords {
		if len(word) > 4 && strings.Contains(passage, word) {
			relevantWords++
		}
	}

	// Not relevant — silence
	if relevantWords == 0 {
		return StanceSilent
	}

	// Look for contradiction signals
	contradictPhrases := []string{
		"no evidence", "not supported", "disproven", "refuted",
		"no scientific basis", "pseudoscience", "debunked",
		"lacks evidence", "not accepted", "contradicts",
	}
	for _, phrase := range contradictPhrases {
		if strings.Contains(passage, phrase) {
			return StanceContradict
		}
	}

	// Look for silence signals (mentions but avoids taking position)
	silencePhrases := []string{
		"remains unclear", "not well understood", "is unknown",
		"is debated", "is controversial", "some researchers",
		"further research", "not yet", "poorly understood",
	}
	for _, phrase := range silencePhrases {
		if strings.Contains(passage, phrase) {
			return StanceSilent
		}
	}

	// Default: if it mentions the concept and doesn't contradict, treat as support
	return StanceSupport
}

func explainScore(cs *ContradictionScore) string {
	if cs.Total == 0 {
		return "No evidence collected yet"
	}

	parts := []string{}

	supportPct := int(100 * cs.Support / cs.Total)
	contradictPct := int(100 * cs.Contradiction / cs.Total)
	silencePct := int(100 * cs.Silence / cs.Total)

	parts = append(parts, fmt.Sprintf(
		"Support: %d%% | Contradiction: %d%% | Silence: %d%%",
		supportPct, contradictPct, silencePct,
	))

	if cs.IsAnomaly {
		if cs.Silence > cs.Support {
			parts = append(parts, "⚠ Mainstream sources are anomalously silent on this claim")
		}
		if cs.Contradiction > cs.Support {
			parts = append(parts, "⚠ More sources contradict than support this claim")
		}
	}

	return strings.Join(parts, " — ")
}
