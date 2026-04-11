package anomaly

import (
	"fmt"
	"strings"
	"time"
)

// Report generates a markdown meta-analysis report from a set of clusters.
// topic is the top-level subject of the KAE session (may be empty).
func Report(clusters []*AnomalyCluster, topic string) string {
	var sb strings.Builder

	sb.WriteString("# KAE Meta-Analysis — Convergent Heresies\n\n")
	if topic != "" {
		sb.WriteString(fmt.Sprintf("**Topic:** %s  \n", topic))
	}
	sb.WriteString(fmt.Sprintf("**Generated:** %s  \n", time.Now().Format("2006-01-02 15:04:05")))
	sb.WriteString(fmt.Sprintf("**Convergent clusters found:** %d\n\n", len(clusters)))

	if len(clusters) == 0 {
		sb.WriteString("No convergent anomalies detected. ")
		sb.WriteString("Run more archaeology sessions to build cross-run evidence.\n")
		return sb.String()
	}

	sb.WriteString("## What Are Convergent Heresies?\n\n")
	sb.WriteString("These concepts were independently flagged as anomalous across multiple ")
	sb.WriteString("separate KAE runs. Independent convergence is evidence that the engine ")
	sb.WriteString("is tracking a genuine blind spot in mainstream discourse rather than ")
	sb.WriteString("hallucinating a one-off artefact.\n\n")
	sb.WriteString("---\n\n")

	for i, cl := range clusters {
		sb.WriteString(fmt.Sprintf("## %d. %s\n\n", i+1, cl.Center))
		sb.WriteString(fmt.Sprintf("| Field | Value |\n|---|---|\n"))
		sb.WriteString(fmt.Sprintf("| Convergent runs | %d (%s) |\n",
			len(cl.RunIDs), strings.Join(cl.RunIDs, ", ")))
		sb.WriteString(fmt.Sprintf("| Cluster members | %d |\n", len(cl.Members)))
		sb.WriteString(fmt.Sprintf("| Peak anomaly weight | %.2f |\n\n", cl.Weight))

		// List related concept labels (skip the center itself)
		seen := make(map[string]bool)
		seen[cl.Center] = true
		var related []string
		for _, m := range cl.Members {
			if !seen[m.Label] {
				seen[m.Label] = true
				related = append(related, m.Label)
			}
		}
		if len(related) > 0 {
			sb.WriteString("**Related concepts in cluster:**\n")
			for _, r := range related {
				sb.WriteString(fmt.Sprintf("- %s\n", r))
			}
			sb.WriteString("\n")
		}

		// Show notes from highest-weight member
		for _, m := range cl.Members {
			if m.Notes != "" && m.Weight >= cl.Weight*0.8 {
				note := m.Notes
				if len(note) > 300 {
					note = note[:300] + "…"
				}
				sb.WriteString("**Evidence excerpt:**\n> ")
				sb.WriteString(strings.ReplaceAll(note, "\n", "\n> "))
				sb.WriteString("\n\n")
				break
			}
		}
		sb.WriteString("---\n\n")
	}

	return sb.String()
}
