package metagraph

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/meistro57/kae/internal/store"
)

const (
	DefaultSimilarityThreshold = 0.85
	DefaultAttractorMinRuns    = 3
)

// MergeRun aggregates all nodes from a completed run into kae_meta_graph.
// Nodes whose concept vector scores >= similarityThreshold against an existing
// meta-node are merged into it; others create new meta-nodes.
func MergeRun(qdrant *store.Client, runID string, attractorMinRuns int) (merged, created int, err error) {
	nodes, err := qdrant.ScrollRunNodes(runID)
	if err != nil {
		return 0, 0, fmt.Errorf("scroll run %s: %w", runID, err)
	}

	for _, node := range nodes {
		if len(node.Vector) == 0 {
			continue
		}

		occ := store.RunOccurrenceRecord{
			RunID:   runID,
			Cycle:   node.Cycle,
			Weight:  node.Weight,
			Anomaly: node.Anomaly,
		}

		existing, err := qdrant.FindSimilarMetaNode(node.Vector, DefaultSimilarityThreshold)
		if err != nil {
			return merged, created, fmt.Errorf("find similar: %w", err)
		}

		if existing != nil {
			// Merge into existing meta-node
			existing.RunOccurrences = append(existing.RunOccurrences, occ)
			existing.OccurrenceCount++
			existing.TotalWeight += node.Weight
			// Update rolling avg anomaly
			prevSum := existing.AvgAnomaly * float64(existing.OccurrenceCount-1)
			if node.Anomaly {
				prevSum += 1.0
			}
			existing.AvgAnomaly = prevSum / float64(existing.OccurrenceCount)
			existing.Domains = mergeDomains(existing.Domains, node.Domain)
			existing.IsAttractor = existing.OccurrenceCount >= attractorMinRuns
			if err := qdrant.UpsertMetaNode(existing); err != nil {
				return merged, created, err
			}
			merged++
		} else {
			// New meta-node
			avgAnomaly := 0.0
			if node.Anomaly {
				avgAnomaly = 1.0
			}
			mn := &store.MetaNodeRecord{
				Concept:         node.Label,
				FirstSeen:       time.Now().Unix(),
				RunOccurrences:  []store.RunOccurrenceRecord{occ},
				TotalWeight:     node.Weight,
				AvgAnomaly:      avgAnomaly,
				Domains:         uniqueStrings([]string{node.Domain}),
				IsAttractor:     false,
				OccurrenceCount: 1,
				Vector:          node.Vector,
			}
			if err := qdrant.UpsertMetaNode(mn); err != nil {
				return merged, created, err
			}
			created++
		}
	}
	return merged, created, nil
}

// AttractorReport returns a markdown report of the strongest attractors.
func AttractorReport(qdrant *store.Client, minRuns int) (string, error) {
	nodes, err := qdrant.GetAttractors(minRuns)
	if err != nil {
		return "", err
	}
	if len(nodes) == 0 {
		return fmt.Sprintf("No attractors found (min %d runs).\n", minRuns), nil
	}

	sort.Slice(nodes, func(i, j int) bool {
		return nodes[i].OccurrenceCount > nodes[j].OccurrenceCount
	})

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("# Meta-Graph Attractors (min %d runs)\n\n", minRuns))
	sb.WriteString(fmt.Sprintf("Found **%d attractor concepts** converging independently across runs.\n\n", len(nodes)))

	for i, n := range nodes {
		sb.WriteString(fmt.Sprintf("## %d. %s\n", i+1, n.Concept))
		sb.WriteString(fmt.Sprintf("- **Runs:** %d | **Total weight:** %.1f | **Avg anomaly:** %.2f\n",
			n.OccurrenceCount, n.TotalWeight, n.AvgAnomaly))
		sb.WriteString(fmt.Sprintf("- **Domains:** %s\n", strings.Join(n.Domains, ", ")))
		sb.WriteString(fmt.Sprintf("- **First seen:** %s\n\n", time.Unix(n.FirstSeen, 0).Format("2006-01-02")))
	}

	return sb.String(), nil
}

// FindBridges returns concepts that span 2+ domains — cross-domain connectors.
func FindBridges(nodes []*store.MetaNodeRecord) []DomainBridge {
	var bridges []DomainBridge
	for _, n := range nodes {
		if len(n.Domains) >= 2 {
			for i := 0; i < len(n.Domains); i++ {
				for j := i + 1; j < len(n.Domains); j++ {
					bridges = append(bridges, DomainBridge{
						Concept: n.Concept,
						Domain1: n.Domains[i],
						Domain2: n.Domains[j],
						Weight:  n.TotalWeight,
						Runs:    n.OccurrenceCount,
					})
				}
			}
		}
	}
	sort.Slice(bridges, func(i, j int) bool {
		return bridges[i].Weight > bridges[j].Weight
	})
	return bridges
}

// FindMoats identifies domain pairs that share the corpus but have no bridge concepts.
func FindMoats(nodes []*store.MetaNodeRecord) []DomainMoat {
	// Build set of all domains and bridged domain pairs
	allDomains := map[string]bool{}
	bridgedPairs := map[string]bool{}

	for _, n := range nodes {
		for _, d := range n.Domains {
			allDomains[d] = true
		}
		if len(n.Domains) >= 2 {
			for i := 0; i < len(n.Domains); i++ {
				for j := i + 1; j < len(n.Domains); j++ {
					key := domainPairKey(n.Domains[i], n.Domains[j])
					bridgedPairs[key] = true
				}
			}
		}
	}

	domains := make([]string, 0, len(allDomains))
	for d := range allDomains {
		if d != "" {
			domains = append(domains, d)
		}
	}
	sort.Strings(domains)

	var moats []DomainMoat
	for i := 0; i < len(domains); i++ {
		for j := i + 1; j < len(domains); j++ {
			key := domainPairKey(domains[i], domains[j])
			if !bridgedPairs[key] {
				moats = append(moats, DomainMoat{
					Domain1:   domains[i],
					Domain2:   domains[j],
					Suspicion: 0.85,
					EdgeCount: 0,
				})
			}
		}
	}
	return moats
}

// DomainBoundaryReport returns a markdown summary of bridges and moats.
func DomainBoundaryReport(nodes []*store.MetaNodeRecord) string {
	bridges := FindBridges(nodes)
	moats := FindMoats(nodes)

	var sb strings.Builder
	sb.WriteString("# Domain Boundary Analysis\n\n")

	sb.WriteString(fmt.Sprintf("## Cross-Domain Bridges (%d)\n\n", len(bridges)))
	if len(bridges) == 0 {
		sb.WriteString("No bridge concepts found yet.\n\n")
	} else {
		for _, b := range bridges {
			sb.WriteString(fmt.Sprintf("- **%s** bridges `%s` ↔ `%s` (weight %.1f, %d runs)\n",
				b.Concept, b.Domain1, b.Domain2, b.Weight, b.Runs))
		}
		sb.WriteString("\n")
	}

	sb.WriteString(fmt.Sprintf("## Potential Domain Moats (%d)\n\n", len(moats)))
	if len(moats) == 0 {
		sb.WriteString("No isolated domain pairs found.\n\n")
	} else {
		sb.WriteString("These domain pairs co-exist in the knowledge base but have no bridging concepts:\n\n")
		for _, m := range moats {
			sb.WriteString(fmt.Sprintf("- `%s` ↔ `%s` (suspicion: %.0f%%)\n",
				m.Domain1, m.Domain2, m.Suspicion*100))
		}
	}
	return sb.String()
}

// ── helpers ───────────────────────────────────────────────────────────────────

// DomainBridge represents a concept that connects two domains.
type DomainBridge struct {
	Concept string
	Domain1 string
	Domain2 string
	Weight  float64
	Runs    int
}

// DomainMoat represents a domain pair with no bridge concepts.
type DomainMoat struct {
	Domain1   string
	Domain2   string
	Suspicion float64
	EdgeCount int
}

func mergeDomains(existing []string, newDomain string) []string {
	return uniqueStrings(append(existing, newDomain))
}

func uniqueStrings(ss []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(ss))
	for _, s := range ss {
		if s != "" && !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}

func domainPairKey(a, b string) string {
	if a > b {
		a, b = b, a
	}
	return a + "||" + b
}
