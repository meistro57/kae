package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"sort"
	"strings"

	"github.com/olekukonko/tablewriter"
	"github.com/spf13/cobra"
)

var (
	qdrantURL  string
	collection string
)

type ConceptNode struct {
	Name      string  `json:"name"`
	Weight    float64 `json:"weight"`
	Cycle     int     `json:"cycle"`
	RunID     string  `json:"run_id"`
	IsAnomaly bool    `json:"is_anomaly"`
	Domain    string  `json:"domain"`
}

type KAERun struct {
	ID        string
	Nodes     int
	Anomalies int
	MaxWeight float64
}

func main() {
	rootCmd := &cobra.Command{
		Use:   "kae-analyzer",
		Short: "Analyze Knowledge Archaeology Engine data in Qdrant",
		Long: `CLI tool for analyzing KAE concept graphs, anomalies, and convergence patterns.
		
Examples:
  kae-analyzer runs                                    # List all runs
  kae-analyzer analyze --run-id run_1775826869        # Analyze specific run
  kae-analyzer compare --runs run_123,run_456          # Compare runs
  kae-analyzer anomalies --min-weight 4.0             # Find high-weight anomalies
  kae-analyzer search --query "pseudo-psychology"      # Search concepts
  kae-analyzer convergence --seed pseudopsychology     # Analyze convergence`,
	}

	rootCmd.PersistentFlags().StringVar(&qdrantURL, "url", "http://localhost:6333", "Qdrant server URL")
	rootCmd.PersistentFlags().StringVar(&collection, "collection", "kae_nodes", "Qdrant collection name")

	rootCmd.AddCommand(
		listRunsCmd(),
		analyzeRunCmd(),
		compareRunsCmd(),
		findAnomaliesCmd(),
		searchConceptsCmd(),
		convergenceCmd(),
		exportCmd(),
		statsCmd(),
	)

	if err := rootCmd.Execute(); err != nil {
		log.Fatal(err)
	}
}

func listRunsCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "runs",
		Short: "List all KAE runs",
		Run: func(cmd *cobra.Command, args []string) {
			runs := getAllRuns()

			table := tablewriter.NewWriter(os.Stdout)
			table.SetHeader([]string{"Run ID", "Nodes", "Anomalies", "Max Weight", "Anomaly %"})
			table.SetBorder(true)

			for _, run := range runs {
				anomalyPct := float64(run.Anomalies) / float64(run.Nodes) * 100
				table.Append([]string{
					run.ID,
					fmt.Sprintf("%d", run.Nodes),
					fmt.Sprintf("%d", run.Anomalies),
					fmt.Sprintf("%.2f", run.MaxWeight),
					fmt.Sprintf("%.1f%%", anomalyPct),
				})
			}

			table.Render()
			fmt.Printf("\nTotal runs: %d\n", len(runs))
		},
	}
}

func analyzeRunCmd() *cobra.Command {
	var runID string
	var topN int

	cmd := &cobra.Command{
		Use:   "analyze",
		Short: "Analyze a specific KAE run in detail",
		Long: `Analyze a KAE run showing:
- Top concepts by weight
- Anomaly distribution  
- Cycle progression
- Domain classification`,
		Run: func(cmd *cobra.Command, args []string) {
			if runID == "" {
				log.Fatal("--run-id required")
			}

			nodes := getRunNodes(runID, topN)

			fmt.Printf("=== Analysis: %s ===\n\n", runID)

			// Top concepts by weight
			fmt.Println("🔝 Top Concepts by Weight:")
			for i, node := range nodes[:min(10, len(nodes))] {
				marker := ""
				if node.IsAnomaly {
					marker = " ⚠️"
				}
				fmt.Printf("%2d. %s (%.2f)%s\n", i+1, node.Name, node.Weight, marker)
			}

			// Anomaly analysis
			anomalies := filterAnomalies(nodes)
			fmt.Printf("\n⚡ Anomalies: %d / %d (%.1f%%)\n", len(anomalies), len(nodes),
				float64(len(anomalies))/float64(len(nodes))*100)

			if len(anomalies) > 0 {
				fmt.Println("\nTop Anomalies:")
				for i, node := range anomalies[:min(5, len(anomalies))] {
					fmt.Printf("%2d. %s (weight: %.2f)\n", i+1, node.Name, node.Weight)
				}
			}

			// Cycle distribution
			cycleMap := make(map[int]int)
			for _, node := range nodes {
				cycleMap[node.Cycle]++
			}

			fmt.Printf("\n📊 Concept Distribution Across Cycles:\n")
			cycles := make([]int, 0, len(cycleMap))
			for cycle := range cycleMap {
				cycles = append(cycles, cycle)
			}
			sort.Ints(cycles)

			for _, cycle := range cycles[:min(5, len(cycles))] {
				fmt.Printf("Cycle %2d: %d concepts\n", cycle, cycleMap[cycle])
			}

			// Domain distribution
			domainMap := make(map[string]int)
			for _, node := range nodes {
				domainMap[node.Domain]++
			}
			fmt.Printf("\n🏷️  Domains:\n")
			for domain, count := range domainMap {
				fmt.Printf("  %s: %d\n", domain, count)
			}
		},
	}

	cmd.Flags().StringVar(&runID, "run-id", "", "Run ID to analyze (e.g. run_1775826869)")
	cmd.Flags().IntVar(&topN, "top", 100, "Number of top nodes to retrieve")
	cmd.MarkFlagRequired("run-id")

	return cmd
}

func compareRunsCmd() *cobra.Command {
	var runIDs []string

	cmd := &cobra.Command{
		Use:   "compare",
		Short: "Compare multiple KAE runs for convergence and divergence",
		Long: `Compare runs to find:
- Convergent concepts (appear in multiple runs)
- Unique concepts per run
- Overlap percentage`,
		Run: func(cmd *cobra.Command, args []string) {
			if len(runIDs) < 2 {
				log.Fatal("Need at least 2 run IDs to compare")
			}

			// Get nodes for each run
			runNodes := make(map[string][]ConceptNode)
			for _, runID := range runIDs {
				runNodes[runID] = getRunNodes(runID, 100)
			}

			// Find convergent concepts
			conceptOccurrences := make(map[string][]string)
			for runID, nodes := range runNodes {
				for _, node := range nodes {
					conceptOccurrences[node.Name] = append(conceptOccurrences[node.Name], runID)
				}
			}

			convergent := make(map[string][]string)
			for concept, runs := range conceptOccurrences {
				if len(runs) >= 2 {
					convergent[concept] = runs
				}
			}

			fmt.Printf("=== Run Comparison ===\n\n")
			fmt.Printf("Comparing %d runs:\n", len(runIDs))
			for _, runID := range runIDs {
				fmt.Printf("  - %s (%d concepts)\n", runID, len(runNodes[runID]))
			}

			overlapPct := float64(len(convergent)) / float64(len(conceptOccurrences)) * 100
			fmt.Printf("\n🔗 Convergent Concepts: %d / %d (%.1f%%)\n",
				len(convergent), len(conceptOccurrences), overlapPct)

			if len(convergent) > 0 {
				fmt.Println("\nTop Convergent Concepts:")
				type convergenceEntry struct {
					concept string
					runs    []string
				}
				entries := make([]convergenceEntry, 0, len(convergent))
				for concept, runs := range convergent {
					entries = append(entries, convergenceEntry{concept, runs})
				}
				sort.Slice(entries, func(i, j int) bool {
					return len(entries[i].runs) > len(entries[j].runs)
				})

				for i, entry := range entries[:min(10, len(entries))] {
					fmt.Printf("%2d. %s (in %d/%d runs)\n",
						i+1, entry.concept, len(entry.runs), len(runIDs))
				}
			}

			fmt.Printf("\n🌿 Unique Concepts per Run:\n")
			for _, runID := range runIDs {
				uniqueCount := 0
				for _, node := range runNodes[runID] {
					if len(conceptOccurrences[node.Name]) == 1 {
						uniqueCount++
					}
				}
				fmt.Printf("  %s: %d unique\n", runID, uniqueCount)
			}
		},
	}

	cmd.Flags().StringSliceVar(&runIDs, "runs", nil, "Comma-separated run IDs")
	cmd.MarkFlagRequired("runs")

	return cmd
}

func findAnomaliesCmd() *cobra.Command {
	var minWeight float64
	var limit int

	cmd := &cobra.Command{
		Use:   "anomalies",
		Short: "Find high-weight anomalies across all runs",
		Run: func(cmd *cobra.Command, args []string) {
			runs := getAllRuns()

			allAnomalies := []ConceptNode{}
			for _, run := range runs {
				nodes := getRunNodes(run.ID, 100)
				anomalies := filterAnomalies(nodes)
				for _, a := range anomalies {
					if a.Weight >= minWeight {
						allAnomalies = append(allAnomalies, a)
					}
				}
			}

			sort.Slice(allAnomalies, func(i, j int) bool {
				return allAnomalies[i].Weight > allAnomalies[j].Weight
			})

			fmt.Printf("=== Anomaly Analysis ===\n\n")
			fmt.Printf("Found %d anomalies with weight >= %.1f\n\n", len(allAnomalies), minWeight)

			table := tablewriter.NewWriter(os.Stdout)
			table.SetHeader([]string{"Concept", "Weight", "Cycle", "Run"})
			table.SetBorder(true)

			for _, a := range allAnomalies[:min(limit, len(allAnomalies))] {
				table.Append([]string{
					truncate(a.Name, 60),
					fmt.Sprintf("%.2f", a.Weight),
					fmt.Sprintf("%d", a.Cycle),
					a.RunID,
				})
			}

			table.Render()
		},
	}

	cmd.Flags().Float64Var(&minWeight, "min-weight", 2.0, "Minimum weight threshold")
	cmd.Flags().IntVar(&limit, "limit", 20, "Max results to display")

	return cmd
}

func searchConceptsCmd() *cobra.Command {
	var query string

	cmd := &cobra.Command{
		Use:   "search",
		Short: "Search for concepts by name across all runs",
		Run: func(cmd *cobra.Command, args []string) {
			if query == "" {
				log.Fatal("--query required")
			}

			runs := getAllRuns()
			matches := []ConceptNode{}

			for _, run := range runs {
				nodes := getRunNodes(run.ID, 100)
				for _, node := range nodes {
					if strings.Contains(strings.ToLower(node.Name), strings.ToLower(query)) {
						matches = append(matches, node)
					}
				}
			}

			sort.Slice(matches, func(i, j int) bool {
				return matches[i].Weight > matches[j].Weight
			})

			fmt.Printf("=== Search Results: '%s' ===\n\n", query)
			fmt.Printf("Found %d matches\n\n", len(matches))

			table := tablewriter.NewWriter(os.Stdout)
			table.SetHeader([]string{"Concept", "Weight", "Cycle", "Run", "Anomaly"})
			table.SetBorder(true)

			for _, m := range matches[:min(20, len(matches))] {
				anomaly := ""
				if m.IsAnomaly {
					anomaly = "⚠️"
				}
				table.Append([]string{
					truncate(m.Name, 50),
					fmt.Sprintf("%.2f", m.Weight),
					fmt.Sprintf("%d", m.Cycle),
					m.RunID,
					anomaly,
				})
			}

			table.Render()
		},
	}

	cmd.Flags().StringVar(&query, "query", "", "Search query (case-insensitive)")
	cmd.MarkFlagRequired("query")

	return cmd
}

func convergenceCmd() *cobra.Command {
	var seed string

	cmd := &cobra.Command{
		Use:   "convergence",
		Short: "Analyze convergence patterns for a seed concept",
		Long: `Analyze how runs with the same seed converge or diverge.
Shows concepts that appear across multiple runs from the same seed.`,
		Run: func(cmd *cobra.Command, args []string) {
			runs := getAllRuns()

			seedRuns := []KAERun{}
			for _, run := range runs {
				if seed == "" || strings.Contains(strings.ToLower(run.ID), strings.ToLower(seed)) {
					seedRuns = append(seedRuns, run)
				}
			}

			if len(seedRuns) == 0 {
				fmt.Printf("No runs found for seed: %s\n", seed)
				return
			}

			fmt.Printf("=== Convergence Analysis ===\n")
			fmt.Printf("Seed: %s\n", seed)
			fmt.Printf("Runs analyzed: %d\n\n", len(seedRuns))

			allConcepts := make(map[string]int)
			for _, run := range seedRuns {
				nodes := getRunNodes(run.ID, 50)
				for _, node := range nodes {
					allConcepts[node.Name]++
				}
			}

			convergent := make(map[string]int)
			for concept, count := range allConcepts {
				if count >= 2 {
					convergent[concept] = count
				}
			}

			fmt.Printf("📈 Convergence Rate: %.1f%%\n",
				float64(len(convergent))/float64(len(allConcepts))*100)

			fmt.Printf("\nConcepts appearing in multiple runs:\n")
			type convergenceStats struct {
				concept string
				count   int
			}
			stats := make([]convergenceStats, 0, len(convergent))
			for concept, count := range convergent {
				stats = append(stats, convergenceStats{concept, count})
			}
			sort.Slice(stats, func(i, j int) bool {
				return stats[i].count > stats[j].count
			})

			for i, s := range stats[:min(15, len(stats))] {
				fmt.Printf("%2d. %s (in %d/%d runs)\n",
					i+1, s.concept, s.count, len(seedRuns))
			}
		},
	}

	cmd.Flags().StringVar(&seed, "seed", "", "Seed concept to filter runs (e.g. 'pseudopsychology')")

	return cmd
}

func statsCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "stats",
		Short: "Show overall KAE statistics",
		Run: func(cmd *cobra.Command, args []string) {
			runs := getAllRuns()

			totalNodes := 0
			totalAnomalies := 0
			maxWeightGlobal := 0.0

			for _, run := range runs {
				totalNodes += run.Nodes
				totalAnomalies += run.Anomalies
				if run.MaxWeight > maxWeightGlobal {
					maxWeightGlobal = run.MaxWeight
				}
			}

			fmt.Println("=== KAE Global Statistics ===")
			fmt.Println()
			fmt.Printf("Total Runs:       %d\n", len(runs))
			fmt.Printf("Total Concepts:   %d\n", totalNodes)
			fmt.Printf("Total Anomalies:  %d (%.1f%%)\n",
				totalAnomalies, float64(totalAnomalies)/float64(totalNodes)*100)
			fmt.Printf("Max Weight:       %.2f\n", maxWeightGlobal)
			fmt.Printf("Avg Concepts/Run: %.1f\n", float64(totalNodes)/float64(len(runs)))
		},
	}
}

func exportCmd() *cobra.Command {
	var outputFile string

	cmd := &cobra.Command{
		Use:   "export",
		Short: "Export analysis data to JSON",
		Run: func(cmd *cobra.Command, args []string) {
			runs := getAllRuns()

			data := make(map[string]interface{})
			data["runs"] = runs
			data["total_runs"] = len(runs)

			runData := make(map[string][]ConceptNode)
			for _, run := range runs {
				nodes := getRunNodes(run.ID, 50)
				runData[run.ID] = nodes
			}
			data["concepts"] = runData

			output, err := json.MarshalIndent(data, "", "  ")
			if err != nil {
				log.Fatal(err)
			}

			if outputFile != "" {
				err = os.WriteFile(outputFile, output, 0644)
				if err != nil {
					log.Fatal(err)
				}
				fmt.Printf("Exported to: %s\n", outputFile)
			} else {
				fmt.Println(string(output))
			}
		},
	}

	cmd.Flags().StringVar(&outputFile, "output", "", "Output file (default: stdout)")

	return cmd
}

// qdrantPost sends a POST request to the Qdrant REST API and returns the decoded body.
func qdrantPost(path string, body any) (map[string]any, error) {
	b, _ := json.Marshal(body)
	base := qdrantURL
	if !strings.HasPrefix(base, "http") {
		base = "http://" + base
	}
	resp, err := http.Post(base+path, "application/json", bytes.NewReader(b))
	if err != nil {
		return nil, fmt.Errorf("qdrant unreachable: %w", err)
	}
	defer resp.Body.Close()
	rb, _ := io.ReadAll(resp.Body)
	var result map[string]any
	if err := json.Unmarshal(rb, &result); err != nil {
		return nil, fmt.Errorf("qdrant response parse: %w", err)
	}
	return result, nil
}

func getAllRuns() []KAERun {
	data, err := qdrantPost("/collections/kae_nodes/points/scroll", map[string]any{
		"limit":        1000,
		"with_payload": true,
		"with_vector":  false,
	})
	if err != nil {
		log.Printf("Warning: Failed to query Qdrant: %v\n", err)
		return []KAERun{}
	}

	result, _ := data["result"].(map[string]any)
	points, _ := result["points"].([]any)

	type runStats struct {
		count   int
		anomaly int
		maxW    float64
	}
	runs := make(map[string]*runStats)

	for _, p := range points {
		pt, _ := p.(map[string]any)
		payload, _ := pt["payload"].(map[string]any)
		runID, _ := payload["run_id"].(string)
		if runID == "" {
			continue
		}
		if runs[runID] == nil {
			runs[runID] = &runStats{}
		}
		r := runs[runID]
		r.count++
		if w, ok := payload["weight"].(float64); ok && w > r.maxW {
			r.maxW = w
		}
		if a, ok := payload["anomaly"].(bool); ok && a {
			r.anomaly++
		}
	}

	out := make([]KAERun, 0, len(runs))
	for id, r := range runs {
		out = append(out, KAERun{
			ID:        id,
			Nodes:     r.count,
			Anomalies: r.anomaly,
			MaxWeight: r.maxW,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

func getRunNodes(runID string, limit int) []ConceptNode {
	body := map[string]any{
		"limit":        limit * 3, // fetch extra to sort by weight
		"with_payload": true,
		"with_vector":  false,
	}
	if runID != "" {
		body["filter"] = map[string]any{
			"must": []map[string]any{
				{"key": "run_id", "match": map[string]any{"value": runID}},
			},
		}
	}

	data, err := qdrantPost("/collections/kae_nodes/points/scroll", body)
	if err != nil {
		log.Printf("Warning: Failed to query Qdrant: %v\n", err)
		return []ConceptNode{}
	}

	result, _ := data["result"].(map[string]any)
	points, _ := result["points"].([]any)

	nodes := make([]ConceptNode, 0, len(points))
	for _, p := range points {
		pt, _ := p.(map[string]any)
		payload, _ := pt["payload"].(map[string]any)
		label, _ := payload["label"].(string)
		if label == "" {
			continue
		}
		weight, _ := payload["weight"].(float64)
		anomaly, _ := payload["anomaly"].(bool)
		domain, _ := payload["domain"].(string)
		rid, _ := payload["run_id"].(string)
		cycle := 0
		switch v := payload["cycle"].(type) {
		case float64:
			cycle = int(v)
		case int:
			cycle = v
		}
		nodes = append(nodes, ConceptNode{
			Name:      label,
			Weight:    weight,
			Cycle:     cycle,
			RunID:     rid,
			IsAnomaly: anomaly,
			Domain:    domain,
		})
	}

	sort.Slice(nodes, func(i, j int) bool { return nodes[i].Weight > nodes[j].Weight })
	if len(nodes) > limit {
		nodes = nodes[:limit]
	}
	return nodes
}

func filterAnomalies(nodes []ConceptNode) []ConceptNode {
	anomalies := []ConceptNode{}
	for _, node := range nodes {
		if node.IsAnomaly {
			anomalies = append(anomalies, node)
		}
	}
	return anomalies
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}
