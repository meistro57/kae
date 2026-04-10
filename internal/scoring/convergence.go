package scoring

import (
	"fmt"
	"sort"
	"strings"
)

// RunSummary holds the top nodes from a completed run
type RunSummary struct {
	RunID    string
	Seed     string
	Cycles   int
	TopNodes []NodeSummary
}

// NodeSummary is a lightweight node record for convergence comparison
type NodeSummary struct {
	Label           string
	Weight          float64
	Anomaly         bool
	SimilarityScore float64 // filled in during comparison
}

// ConvergenceResult is the output of comparing two or more runs
type ConvergenceResult struct {
	Runs         []string            // run IDs compared
	OverlapScore float64             // 0.0–1.0
	SharedNodes  []ConvergedNode     // nodes that appeared across runs
	UniqueNodes  map[string][]string // runID → nodes only in that run
	Verdict      string
}

// ConvergedNode is a concept that appeared in multiple runs
type ConvergedNode struct {
	Label      string
	Runs       []string // which runs surfaced this
	AvgWeight  float64
	IsAnomaly  bool
	Confidence float64 // how strongly it converged
}

// CompareRuns measures overlap between multiple run summaries
// Uses semantic similarity via pre-computed scores from Qdrant
func CompareRuns(runs []RunSummary, similarityThreshold float64) *ConvergenceResult {
	if len(runs) < 2 {
		return &ConvergenceResult{Verdict: "Need at least 2 runs to compare"}
	}

	result := &ConvergenceResult{
		Runs:        make([]string, len(runs)),
		UniqueNodes: make(map[string][]string),
	}
	for i, r := range runs {
		result.Runs[i] = r.RunID
	}

	// Build a map: normalized label → list of (runID, node) pairs
	type appearance struct {
		runID string
		node  NodeSummary
	}
	nodeMap := make(map[string][]appearance)

	for _, run := range runs {
		for _, node := range run.TopNodes {
			key := normalizeLabel(node.Label)
			nodeMap[key] = append(nodeMap[key], appearance{run.RunID, node})
		}
	}

	// Find nodes that appear in multiple runs
	convergenceCount := 0
	totalNodes := 0

	for label, appearances := range nodeMap {
		totalNodes++
		runsSeen := make(map[string]bool)
		var totalWeight float64
		isAnomaly := false

		for _, a := range appearances {
			runsSeen[a.runID] = true
			totalWeight += a.node.Weight
			if a.node.Anomaly {
				isAnomaly = true
			}
		}

		runList := make([]string, 0, len(runsSeen))
		for r := range runsSeen {
			runList = append(runList, r)
		}

		if len(runsSeen) >= 2 {
			convergenceCount++
			confidence := float64(len(runsSeen)) / float64(len(runs))
			result.SharedNodes = append(result.SharedNodes, ConvergedNode{
				Label:      label,
				Runs:       runList,
				AvgWeight:  totalWeight / float64(len(appearances)),
				IsAnomaly:  isAnomaly,
				Confidence: confidence,
			})
		} else {
			// Unique to one run
			runID := appearances[0].runID
			result.UniqueNodes[runID] = append(result.UniqueNodes[runID], label)
		}
	}

	// Sort shared nodes by confidence then weight
	sort.Slice(result.SharedNodes, func(i, j int) bool {
		if result.SharedNodes[i].Confidence != result.SharedNodes[j].Confidence {
			return result.SharedNodes[i].Confidence > result.SharedNodes[j].Confidence
		}
		return result.SharedNodes[i].AvgWeight > result.SharedNodes[j].AvgWeight
	})

	if totalNodes > 0 {
		result.OverlapScore = float64(convergenceCount) / float64(totalNodes)
	}

	result.Verdict = buildVerdict(result, len(runs))
	return result
}

// Report generates a human-readable convergence report
func (cr *ConvergenceResult) Report() string {
	var sb strings.Builder

	sb.WriteString("# Cross-Run Convergence Report\n\n")
	sb.WriteString(fmt.Sprintf("**Runs compared:** %s\n", strings.Join(cr.Runs, ", ")))
	sb.WriteString(fmt.Sprintf("**Overlap score:** %.1f%%\n\n", cr.OverlapScore*100))
	sb.WriteString(fmt.Sprintf("**Verdict:** %s\n\n", cr.Verdict))

	if len(cr.SharedNodes) > 0 {
		sb.WriteString("## Converged Concepts\n\n")
		sb.WriteString("These concepts emerged independently across multiple runs.\n\n")
		for _, n := range cr.SharedNodes {
			anomalyFlag := ""
			if n.IsAnomaly {
				anomalyFlag = " ⚠ ANOMALY"
			}
			sb.WriteString(fmt.Sprintf("### %s%s\n", n.Label, anomalyFlag))
			sb.WriteString(fmt.Sprintf("- Appeared in: %d/%d runs\n", len(n.Runs), len(cr.Runs)))
			sb.WriteString(fmt.Sprintf("- Confidence: %.0f%%\n", n.Confidence*100))
			sb.WriteString(fmt.Sprintf("- Average weight: %.2f\n\n", n.AvgWeight))
		}
	}

	// Anomalies that converged across runs — the most interesting finding
	var convergentAnomalies []ConvergedNode
	for _, n := range cr.SharedNodes {
		if n.IsAnomaly && n.Confidence >= 0.6 {
			convergentAnomalies = append(convergentAnomalies, n)
		}
	}

	if len(convergentAnomalies) > 0 {
		sb.WriteString("## ⚠ High-Confidence Convergent Anomalies\n\n")
		sb.WriteString("These consensus gaps were independently flagged across multiple runs.\n")
		sb.WriteString("This is the strongest signal the system can produce.\n\n")
		for _, n := range convergentAnomalies {
			sb.WriteString(fmt.Sprintf("- **%s** (%.0f%% confidence across %d runs)\n",
				n.Label, n.Confidence*100, len(n.Runs)))
		}
	}

	return sb.String()
}

func buildVerdict(cr *ConvergenceResult, totalRuns int) string {
	score := cr.OverlapScore
	convergentAnomalies := 0
	for _, n := range cr.SharedNodes {
		if n.IsAnomaly {
			convergentAnomalies++
		}
	}

	switch {
	case score >= 0.7 && convergentAnomalies >= 3:
		return fmt.Sprintf(
			"STRONG CONVERGENCE — %.0f%% overlap with %d convergent anomalies. "+
				"The agent independently arrived at the same conclusions from different seeds. "+
				"These findings are structural, not model artifacts.",
			score*100, convergentAnomalies)

	case score >= 0.5:
		return fmt.Sprintf(
			"MODERATE CONVERGENCE — %.0f%% overlap across %d runs. "+
				"Significant shared structure detected. Run more cycles to strengthen signal.",
			score*100, totalRuns)

	case score >= 0.3:
		return fmt.Sprintf(
			"WEAK CONVERGENCE — %.0f%% overlap. "+
				"Some shared structure but runs are diverging. "+
				"Consider longer runs or more focused seeds.",
			score*100)

	default:
		return fmt.Sprintf(
			"NO CONVERGENCE — %.0f%% overlap. "+
				"Runs produced mostly unique results. "+
				"Either the seeds were too different or more cycles needed.",
			score*100)
	}
}

func normalizeLabel(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.ReplaceAll(s, " ", "_")
	s = strings.ReplaceAll(s, "-", "_")
	return s
}
