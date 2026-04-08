package main

import (
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/meistro/kae/internal/agent"
	"github.com/meistro/kae/internal/config"
	"github.com/meistro/kae/internal/ui"
)

func main() {
	model  := flag.String("model", "deepseek/deepseek-r1", "OpenRouter model (default: deepseek-r1 for visible thinking)")
	fast   := flag.String("fast", "google/gemini-2.5-flash", "Fast model for bulk ingestion passes")
	cycles := flag.Int("cycles", 0, "Max cycles — 0 means run until graph stabilizes")
	seed   := flag.String("seed", "", "Optional seed topic — leave empty for autonomous start")
	debug  := flag.Bool("debug", false, "Log debug output to debug.log")
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

	cfg.Model     = *model
	cfg.FastModel = *fast
	cfg.MaxCycles = *cycles
	cfg.Seed      = *seed

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
}

func saveReport(eng *agent.Engine) {
	report := eng.Report()
	if strings.TrimSpace(report) == "" {
		return
	}
	focus := eng.Focus()
	if focus == "" {
		focus = "kae"
	}
	slug := strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_' {
			return r
		}
		if r == ' ' {
			return '_'
		}
		return -1
	}, strings.ToLower(focus))
	filename := fmt.Sprintf("report_%s_%s.md", slug, time.Now().Format("20060102_150405"))
	if err := os.WriteFile(filename, []byte(report), 0644); err != nil {
		fmt.Fprintln(os.Stderr, "could not save report:", err)
		return
	}
	fmt.Println("report saved:", filename)
}
