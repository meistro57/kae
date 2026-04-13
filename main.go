package main

import (
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/meistro57/kae/internal/agent"
	"github.com/meistro57/kae/internal/anomaly"
	"github.com/meistro57/kae/internal/config"
	"github.com/meistro57/kae/internal/metagraph"
	"github.com/meistro57/kae/internal/report"
	"github.com/meistro57/kae/internal/store"
	"github.com/meistro57/kae/internal/ui"
)

func main() {
	// ── Core flags ─────────────────────────────────────────────────────────────
	model := flag.String("model", "deepseek/deepseek-r1",
		`Primary reasoning model.  Accepts "provider:model" syntax:
  openrouter:deepseek/deepseek-r1   (default)
  anthropic:claude-opus-4-6
  openai:gpt-4o
  gemini:gemini-2.5-flash
  ollama:llama3.1`)
	fast := flag.String("fast", "google/gemini-2.5-flash",
		"Fast model for bulk passes (same provider:model syntax)")
	cycles := flag.Int("cycles", 0,
		"Max cycles — 0 means run until graph stabilizes")
	seed := flag.String("seed", "",
		"Optional seed topic — leave empty for autonomous start")
	shared := flag.Bool("shared", false,
		"Use shared memory across runs")
	resumeGraph := flag.String("resume-graph", "",
		"Optional path to a saved graph JSON snapshot to resume from")
	saveGraphPath := flag.String("save-graph", "",
		"Optional path to save graph JSON snapshot on exit")
	debug := flag.Bool("debug", false,
		"Log debug output to debug.log")
	headless := flag.Bool("headless", false,
		"Run without TUI (for scripts/MCP servers)")
	autoRestart := flag.Bool("auto-restart", false,
		"Save report and restart automatically when the graph stagnates")

	// ── Ensemble flags (Tier 1.1) ──────────────────────────────────────────────
	ensembleMode := flag.Bool("ensemble", false,
		"Enable multi-model ensemble reasoning")
	ensembleModels := flag.String("models", "",
		`Comma-separated list of models for ensemble mode.
  Example: --models "anthropic:claude-opus-4-6,openai:gpt-4o,gemini:gemini-2.5-flash"`)

	// ── Run controller flags (Tier 1.2) ───────────────────────────────────────
	noveltyThreshold := flag.Float64("novelty-threshold", 0.05,
		"New-nodes/total ratio below which a cycle counts as stagnant")
	stagnationWindow := flag.Int("stagnation-window", 3,
		"Consecutive stagnant cycles before auto-stop")
	branchThreshold := flag.Float64("branch-threshold", 0.7,
		"Ensemble controversy score above which a branch is triggered")
	maxBranches := flag.Int("max-branches", 4,
		"Maximum auto-branches per run (0 = unlimited)")

	// ── Meta-analysis flags (Tier 1.3) ────────────────────────────────────────
	analyze := flag.Bool("analyze", false,
		"Run cross-run anomaly meta-analysis instead of a new archaeology run")
	minRuns := flag.Int("min-runs", 2,
		"Minimum distinct runs for a cluster to appear in meta-analysis")

	// ── Tier 2 flags ───────────────────────────────────────────────────────────
	showAttractors := flag.Bool("attractors", false,
		"Print attractor report from persistent meta-graph and exit")
	attractorMinRuns := flag.Int("attractor-min-runs", 3,
		"Minimum run occurrences for a concept to be flagged as an attractor")
	domainAnalysis := flag.Bool("domain-analysis", false,
		"Print domain bridge/moat analysis from meta-graph and exit")
	noMetaGraph := flag.Bool("no-meta-graph", false,
		"Skip updating the persistent meta-graph after this run")

	flag.Parse()

	if *debug {
		f, err := tea.LogToFile("debug.log", "kae")
		if err != nil {
			fmt.Fprintln(os.Stderr, "could not open debug log:", err)
			os.Exit(1)
		}
		defer f.Close()
	}

	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintln(os.Stderr, "config error:", err)
		os.Exit(1)
	}

	// Apply flag overrides
	cfg.Model = *model
	cfg.FastModel = *fast
	cfg.MaxCycles = *cycles
	cfg.Seed = *seed
	cfg.SharedMemory = *shared
	cfg.ResumeGraphPath = *resumeGraph
	cfg.SaveGraphPath = *saveGraphPath

	cfg.EnsembleMode = *ensembleMode
	if *ensembleModels != "" {
		for _, m := range strings.Split(*ensembleModels, ",") {
			m = strings.TrimSpace(m)
			if m != "" {
				cfg.EnsembleModels = append(cfg.EnsembleModels, m)
			}
		}
	}

	cfg.NoveltyThreshold = *noveltyThreshold
	cfg.StagnationWindow = *stagnationWindow
	cfg.BranchThreshold = *branchThreshold
	cfg.MaxBranches = *maxBranches

	cfg.RunAnalysis = *analyze
	cfg.MinAnalysisRuns = *minRuns

	// ── Meta-analysis mode ─────────────────────────────────────────────────────
	if cfg.RunAnalysis {
		runMetaAnalysis(cfg)
		return
	}

	// ── Tier 2: attractor / domain-analysis modes ──────────────────────────────
	if *showAttractors {
		qdrant := store.NewClient(cfg.QdrantURL)
		md, err := metagraph.AttractorReport(qdrant, *attractorMinRuns)
		if err != nil {
			fmt.Fprintln(os.Stderr, "attractor report error:", err)
			os.Exit(1)
		}
		fmt.Print(md)
		return
	}
	if *domainAnalysis {
		qdrant := store.NewClient(cfg.QdrantURL)
		nodes, err := qdrant.GetAllMetaNodes(0)
		if err != nil {
			fmt.Fprintln(os.Stderr, "meta-graph fetch error:", err)
			os.Exit(1)
		}
		fmt.Print(metagraph.DomainBoundaryReport(nodes))
		return
	}

	// Require at least one provider key for a normal run
	if err := cfg.Validate(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		fmt.Fprintln(os.Stderr, "Set OPENROUTER_API_KEY, ANTHROPIC_API_KEY, OPENAI_API_KEY, or GEMINI_API_KEY")
		os.Exit(1)
	}

	// ── Normal archaeology run (with optional auto-restart on stagnation) ─────
	runNum := 0
	for {
		runNum++
		if runNum > 1 {
			fmt.Fprintf(os.Stderr, "\n─── Auto-restart #%d ───\n\n", runNum)
		}

		eng := agent.NewEngine(cfg)

		if *headless {
			runHeadless(eng, cfg)
		} else {
			app := ui.NewApp(eng)

			p := tea.NewProgram(app,
				tea.WithAltScreen(),
				tea.WithMouseCellMotion(),
			)

			if _, err := p.Run(); err != nil {
				fmt.Fprintln(os.Stderr, "UI error:", err)
				os.Exit(1)
			}
		}

		saveReport(eng)
		saveGraph(eng, cfg.SaveGraphPath)
		if !*noMetaGraph {
			mergeMetaGraph(eng.RunID(), cfg.QdrantURL, *attractorMinRuns)
		}

		if *autoRestart && eng.StoppedByStagnation() {
			fmt.Fprintln(os.Stderr, "Stagnation detected — restarting fresh run...")
			continue
		}
		break
	}
}

// runMetaAnalysis performs cross-run anomaly clustering and prints a report.
func runMetaAnalysis(cfg *config.Config) {
	qdrant := store.NewClient(cfg.QdrantURL)
	ma := anomaly.NewMetaAnalyzer(qdrant, cfg.MinAnalysisRuns)

	fmt.Fprintf(os.Stderr, "Fetching anomaly nodes from Qdrant at %s...\n", cfg.QdrantURL)
	clusters, err := ma.FindConvergentHeresies()
	if err != nil {
		fmt.Fprintln(os.Stderr, "meta-analysis error:", err)
		os.Exit(1)
	}

	md := anomaly.Report(clusters, cfg.Seed)

	// Write markdown
	base := report.BuildBaseFilename("meta_analysis", time.Now())
	mdPath, htmlPath := report.ArtifactPaths(base)

	if err := report.SaveMarkdown(mdPath, md); err != nil {
		fmt.Fprintln(os.Stderr, "could not save markdown:", err)
	} else {
		fmt.Println("meta-analysis saved:", mdPath)
	}

	if err := report.SaveHTML(htmlPath, "KAE Meta-Analysis — Convergent Heresies", md); err != nil {
		fmt.Fprintln(os.Stderr, "could not save HTML:", err)
	} else {
		fmt.Println("html saved:", htmlPath)
	}

	fmt.Print(md)
}

func runHeadless(eng *agent.Engine, cfg *config.Config) {
	// Start the engine
	eng.Start()

	// Listen to events and print progress
	events := eng.Events()

	fmt.Fprintf(os.Stderr, "KAE headless run started (max cycles: %d)\n", cfg.MaxCycles)
	if cfg.Seed != "" {
		fmt.Fprintf(os.Stderr, "Seed: %s\n", cfg.Seed)
	}

	for ev := range events {
		if ev.Err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", ev.Err)
			continue
		}

		// Print phase changes
		if ev.Phase != "" {
			fmt.Fprintf(os.Stderr, "[%s] %s\n", ev.Phase, ev.Focus)
		}

		// Print thinking output as it arrives
		if ev.ThinkChunk != "" {
			fmt.Fprint(os.Stderr, ev.ThinkChunk)
		}

		// Print regular output
		if ev.OutputChunk != "" {
			fmt.Fprint(os.Stderr, ev.OutputChunk)
		}

		// Print report updates
		if ev.ReportLine != "" {
			fmt.Fprintln(os.Stderr, ev.ReportLine)
		}

		// Check if graph is stable (end condition)
		if ev.Phase == agent.PhaseStable {
			fmt.Fprintln(os.Stderr, "Graph stabilized — run complete")
			break
		}

		// Check max cycles
		if cfg.MaxCycles > 0 && ev.GraphSnap.Cycles >= cfg.MaxCycles {
			fmt.Fprintf(os.Stderr, "Reached max cycles (%d) — run complete\n", cfg.MaxCycles)
			break
		}
	}

	// Give engine a moment to finish
	time.Sleep(100 * time.Millisecond)
	fmt.Fprintln(os.Stderr, "Headless run finished")
}

func saveReport(eng *agent.Engine) {
	reportBody := eng.Report()
	if strings.TrimSpace(reportBody) == "" {
		return
	}

	base := report.BuildBaseFilename(eng.Focus(), time.Now())
	mdPath, htmlPath := report.ArtifactPaths(base)

	if err := report.SaveMarkdown(mdPath, reportBody); err != nil {
		fmt.Fprintln(os.Stderr, "could not save markdown report:", err)
		return
	}
	fmt.Println("report saved:", mdPath)

	title := fmt.Sprintf("KAE Run Report — %s", eng.Focus())
	if strings.TrimSpace(eng.Focus()) == "" {
		title = "KAE Run Report"
	}
	if err := report.SaveHTML(htmlPath, title, reportBody); err != nil {
		fmt.Fprintln(os.Stderr, "could not save html report:", err)
		return
	}
	fmt.Println("html report saved:", htmlPath)
}

func mergeMetaGraph(runID, qdrantURL string, attractorMinRuns int) {
	qdrant := store.NewClient(qdrantURL)
	merged, created, err := metagraph.MergeRun(qdrant, runID, attractorMinRuns)
	if err != nil {
		fmt.Fprintf(os.Stderr, "meta-graph merge error: %v\n", err)
		return
	}
	fmt.Fprintf(os.Stderr, "meta-graph updated: %d merged, %d new concepts\n", merged, created)
}

func saveGraph(eng *agent.Engine, path string) {
	if strings.TrimSpace(path) == "" {
		return
	}
	if err := eng.SaveGraph(path); err != nil {
		fmt.Fprintln(os.Stderr, "could not save graph snapshot:", err)
		return
	}
	fmt.Println("graph snapshot saved:", path)
}
