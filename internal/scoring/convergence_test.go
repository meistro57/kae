package scoring

import (
	"strings"
	"testing"
)

func TestCompareRunsCalculatesOverlapAndUniqueNodes(t *testing.T) {
	t.Parallel()

	runs := []RunSummary{
		{
			RunID: "run-1",
			TopNodes: []NodeSummary{
				{Label: "Quantum Coherence", Weight: 1.0},
				{Label: "Neural Noise", Weight: 0.8},
			},
		},
		{
			RunID: "run-2",
			TopNodes: []NodeSummary{
				{Label: "quantum-coherence", Weight: 1.2, Anomaly: true},
				{Label: "Ancient Text", Weight: 0.4},
			},
		},
	}

	result := CompareRuns(runs, 0.7)
	if result == nil {
		t.Fatal("expected non-nil result")
	}

	if len(result.SharedNodes) != 1 {
		t.Fatalf("expected 1 shared node, got %d", len(result.SharedNodes))
	}
	shared := result.SharedNodes[0]
	if shared.Label != "quantum_coherence" {
		t.Fatalf("unexpected normalized label: %q", shared.Label)
	}
	if !shared.IsAnomaly {
		t.Fatal("expected anomaly to carry through shared node")
	}

	if len(result.UniqueNodes["run-1"]) != 1 || result.UniqueNodes["run-1"][0] != "neural_noise" {
		t.Fatalf("unexpected unique nodes for run-1: %#v", result.UniqueNodes["run-1"])
	}
	if len(result.UniqueNodes["run-2"]) != 1 || result.UniqueNodes["run-2"][0] != "ancient_text" {
		t.Fatalf("unexpected unique nodes for run-2: %#v", result.UniqueNodes["run-2"])
	}

	if result.OverlapScore <= 0 {
		t.Fatalf("expected positive overlap score, got %f", result.OverlapScore)
	}
	if result.Verdict == "" {
		t.Fatal("expected non-empty verdict")
	}
}

func TestCompareRunsNeedsAtLeastTwoRuns(t *testing.T) {
	t.Parallel()

	result := CompareRuns([]RunSummary{{RunID: "solo"}}, 0.7)
	if result.Verdict != "Need at least 2 runs to compare" {
		t.Fatalf("unexpected verdict: %q", result.Verdict)
	}
}

func TestConvergenceReportIncludesSections(t *testing.T) {
	t.Parallel()

	cr := &ConvergenceResult{
		Runs:         []string{"run-1", "run-2", "run-3"},
		OverlapScore: 0.8,
		Verdict:      "STRONG CONVERGENCE",
		SharedNodes: []ConvergedNode{
			{Label: "signal", Runs: []string{"run-1", "run-2", "run-3"}, AvgWeight: 2.0, IsAnomaly: true, Confidence: 1.0},
		},
	}

	report := cr.Report()
	checks := []string{
		"# Cross-Run Convergence Report",
		"STRONG CONVERGENCE",
		"## Converged Concepts",
		"## ⚠ High-Confidence Convergent Anomalies",
	}
	for _, needle := range checks {
		if !strings.Contains(report, needle) {
			t.Fatalf("expected report to include %q", needle)
		}
	}
}
