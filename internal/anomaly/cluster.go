// Package anomaly provides cross-run anomaly clustering and meta-analysis.
// It groups KAE anomaly nodes by cosine similarity to surface "convergent
// heresies" — concepts independently flagged across multiple separate runs.
package anomaly

import (
	"math"
	"sort"

	"github.com/meistro57/kae/internal/store"
)

// AnomalyCluster groups semantically similar anomaly nodes from Qdrant.
type AnomalyCluster struct {
	Center  string // label of the most representative / heaviest node
	Members []*store.AnomalyNode
	RunIDs  []string // distinct run IDs that contributed members
	Weight  float64  // highest anomaly weight in the cluster
}

// MetaAnalyzer fetches anomaly nodes from Qdrant and clusters them.
type MetaAnalyzer struct {
	qdrant  *store.Client
	minRuns int // minimum distinct runs for a cluster to count as convergent
}

// NewMetaAnalyzer creates a MetaAnalyzer targeting the given Qdrant client.
// minRuns sets the minimum number of distinct runs required for a cluster to
// appear in FindConvergentHeresies results.
func NewMetaAnalyzer(qdrant *store.Client, minRuns int) *MetaAnalyzer {
	return &MetaAnalyzer{qdrant: qdrant, minRuns: minRuns}
}

// FindConvergentHeresies fetches all anomaly nodes, clusters them by cosine
// similarity (threshold 0.75), and returns only clusters seen across ≥ minRuns
// distinct runs, sorted by number of convergent runs descending.
func (m *MetaAnalyzer) FindConvergentHeresies() ([]*AnomalyCluster, error) {
	nodes, err := m.qdrant.FetchAnomalyNodes(1000)
	if err != nil {
		return nil, err
	}
	if len(nodes) == 0 {
		return nil, nil
	}

	clusters := clusterByCosine(nodes, 0.75)

	var result []*AnomalyCluster
	for _, cl := range clusters {
		seen := make(map[string]bool)
		for _, n := range cl.Members {
			if n.RunID != "" {
				seen[n.RunID] = true
			}
		}
		if len(seen) < m.minRuns {
			continue
		}
		runs := make([]string, 0, len(seen))
		for r := range seen {
			runs = append(runs, r)
		}
		sort.Strings(runs)
		cl.RunIDs = runs
		result = append(result, cl)
	}

	sort.Slice(result, func(i, j int) bool {
		return len(result[i].RunIDs) > len(result[j].RunIDs)
	})
	return result, nil
}

// clusterByCosine groups nodes greedily: each node joins the first existing
// cluster whose seed vector exceeds the threshold, or starts a new cluster.
func clusterByCosine(nodes []*store.AnomalyNode, threshold float64) []*AnomalyCluster {
	var clusters []*AnomalyCluster

	for _, n := range nodes {
		if len(n.Vector) == 0 {
			continue
		}

		bestCluster := -1
		bestSim := threshold

		for i, cl := range clusters {
			if len(cl.Members) == 0 || len(cl.Members[0].Vector) == 0 {
				continue
			}
			sim := cosineSim(n.Vector, cl.Members[0].Vector)
			if sim > bestSim {
				bestSim = sim
				bestCluster = i
			}
		}

		if bestCluster >= 0 {
			cl := clusters[bestCluster]
			cl.Members = append(cl.Members, n)
			if n.Weight > cl.Weight {
				cl.Weight = n.Weight
				cl.Center = n.Label
			}
		} else {
			clusters = append(clusters, &AnomalyCluster{
				Center:  n.Label,
				Members: []*store.AnomalyNode{n},
				Weight:  n.Weight,
			})
		}
	}
	return clusters
}

func cosineSim(a, b []float32) float64 {
	if len(a) != len(b) || len(a) == 0 {
		return 0
	}
	var dot, na, nb float64
	for i := range a {
		dot += float64(a[i]) * float64(b[i])
		na += float64(a[i]) * float64(a[i])
		nb += float64(b[i]) * float64(b[i])
	}
	if na == 0 || nb == 0 {
		return 0
	}
	return dot / (math.Sqrt(na) * math.Sqrt(nb))
}
