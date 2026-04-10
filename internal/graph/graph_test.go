package graph

import (
	"testing"

	"github.com/meistro57/kae/internal/scoring"
)

func TestUpsertNodeMergesData(t *testing.T) {
	t.Parallel()

	g := New()
	g.UpsertNode(&Node{ID: "n1", Label: "alpha", Domain: "ingested", Sources: []string{"s1"}, Weight: 1.0})
	g.UpsertNode(&Node{
		ID:      "n1",
		Label:   "alpha",
		Domain:  "inferred",
		Sources: []string{"s2"},
		Weight:  0.5,
		Anomaly: true,
		Notes:   "updated",
		Vector:  []float32{1, 2, 3},
		ContradictionScore: &scoring.ContradictionScore{
			Claim: "test",
		},
	})

	if g.NodeCount() != 1 {
		t.Fatalf("expected 1 node, got %d", g.NodeCount())
	}

	n := g.AllNodes()[0]
	if n.Weight != 1.5 {
		t.Fatalf("expected accumulated weight 1.5, got %f", n.Weight)
	}
	if !n.Anomaly {
		t.Fatal("expected anomaly flag to be true")
	}
	if n.Notes != "updated" {
		t.Fatalf("expected notes updated, got %q", n.Notes)
	}
	if len(n.Sources) != 2 {
		t.Fatalf("expected merged sources, got %#v", n.Sources)
	}
	if len(n.Vector) != 3 {
		t.Fatalf("expected vector to be updated, got %#v", n.Vector)
	}
	if n.ContradictionScore == nil {
		t.Fatal("expected contradiction score to be set")
	}
}

func TestAddEdgeIncreasesNodeWeights(t *testing.T) {
	t.Parallel()

	g := New()
	g.UpsertNode(&Node{ID: "a", Label: "A", Weight: 1.0})
	g.UpsertNode(&Node{ID: "b", Label: "B", Weight: 2.0})
	g.AddEdge(&Edge{From: "a", To: "b", Relation: "connects_to", Confidence: 0.7})

	if g.EdgeCount() != 1 {
		t.Fatalf("expected 1 edge, got %d", g.EdgeCount())
	}

	nodes := g.AllNodes()
	weights := map[string]float64{}
	for _, n := range nodes {
		weights[n.ID] = n.Weight
	}

	if weights["a"] != 1.7 {
		t.Fatalf("expected node a weight 1.7, got %f", weights["a"])
	}
	if weights["b"] != 2.7 {
		t.Fatalf("expected node b weight 2.7, got %f", weights["b"])
	}
}

func TestCleanSummaryFiltersJunkNodes(t *testing.T) {
	t.Parallel()

	g := New()
	g.UpsertNode(&Node{ID: "real", Label: "Quantum coherence", Weight: 1.0})
	g.UpsertNode(&Node{ID: "junk", Label: "NO SOURCE VERIFICATION AVAILABLE", Weight: 1.0})

	summary := g.CleanSummary()
	want := "Nodes: 1 | Edges: 0 | Anomalies: 0"
	if summary != want {
		t.Fatalf("unexpected clean summary: want %q got %q", want, summary)
	}
}
