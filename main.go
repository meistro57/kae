package main

import (
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/meistro57/kae/internal/agent"
	"github.com/meistro57/kae/internal/config"
	"github.com/meistro57/kae/internal/report"
	"github.com/meistro57/kae/internal/ui"
)

func main() {
	model := flag.String("model", "deepseek/deepseek-r1", "OpenRouter model (default: deepseek-r1 for visible thinking)")
	fast := flag.String("fast", "google/gemini-2.5-flash", "Fast model for bulk ingestion passes")
	cycles := flag.Int("cycles", 0, "Max cycles — 0 means run until graph stabilizes")
	seed := flag.String("seed", "", "Optional seed topic — leave empty for autonomous start")
	shared := flag.Bool("shared", false, "Use shared memory across runs")
	resumeGraph := flag.String("resume-graph", "", "Optional path to a saved graph JSON snapshot to resume from")
	saveGraphPath := flag.String("save-graph", "", "Optional path to save graph JSON snapshot on exit")
	debug := flag.Bool("debug", false, "Log debug output to debug.log")
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
		fmt.Fprintln(os.Stderr, "set OPENROUTER_API_KEY environment variable")
		os.Exit(1)
	}

	cfg.Model = *model
	cfg.FastModel = *fast
	cfg.MaxCycles = *cycles
	cfg.Seed = *seed
	cfg.SharedMemory = *shared
	cfg.ResumeGraphPath = *resumeGraph
	cfg.SaveGraphPath = *saveGraphPath

	eng := agent.NewEngine(cfg)
	app := ui.NewApp(eng)

	p := tea.NewProgram(app,
		tea.WithAltScreen(),
		tea.WithMouseCellMotion(),
	)

	if _, err := p.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "UI error:", err)
		os.Exit(1)
	}

	saveReport(eng)
	saveGraph(eng, cfg.SaveGraphPath)
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
