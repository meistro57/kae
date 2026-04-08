package ui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/progress"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/meistro/kae/internal/agent"
)

// ── Colour palette ────────────────────────────────────────────────────────────
var (
	colorBg      = lipgloss.Color("#0d0d0d")
	colorBorder  = lipgloss.Color("#1e3a2f")
	colorAccent  = lipgloss.Color("#00ff88")
	colorDim     = lipgloss.Color("#3a3a3a")
	colorThink   = lipgloss.Color("#4a9eff")
	colorOutput  = lipgloss.Color("#e0e0e0")
	colorAnomaly = lipgloss.Color("#ff4466")
	colorPhase   = lipgloss.Color("#ffcc00")
	colorMuted   = lipgloss.Color("#555555")
	colorReport  = lipgloss.Color("#aaffcc")
	colorBar     = lipgloss.Color("#00cc66")
	colorBarBg   = lipgloss.Color("#1a2a1e")
)

// ── Styles ────────────────────────────────────────────────────────────────────
var (
	borderStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colorBorder)

	headerStyle = lipgloss.NewStyle().
			Foreground(colorAccent).
			Bold(true).
			Padding(0, 1)

	phaseStyle = lipgloss.NewStyle().
			Foreground(colorPhase).
			Bold(true)

	thinkStyle = lipgloss.NewStyle().
			Foreground(colorThink).
			Italic(true)

	outputStyle = lipgloss.NewStyle().
			Foreground(colorOutput)

	anomalyStyle = lipgloss.NewStyle().
			Foreground(colorAnomaly).
			Bold(true)

	statStyle = lipgloss.NewStyle().
			Foreground(colorMuted)

	statValueStyle = lipgloss.NewStyle().
			Foreground(colorAccent).
			Bold(true)

	reportStyle = lipgloss.NewStyle().
			Foreground(colorReport)

	dimStyle = lipgloss.NewStyle().
			Foreground(colorDim)

	barLabelStyle = lipgloss.NewStyle().
			Foreground(colorOutput)

	barFillStyle = lipgloss.NewStyle().
			Foreground(colorBar)

	barBgStyle = lipgloss.NewStyle().
			Foreground(colorBarBg)
)

// ── Bubbletea messages ────────────────────────────────────────────────────────
type tickMsg time.Time
type agentEventMsg agent.Event

// ── App model ─────────────────────────────────────────────────────────────────
type App struct {
	eng    *agent.Engine
	width  int
	height int

	// panel viewports
	thinkVP  viewport.Model
	outputVP viewport.Model
	reportVP viewport.Model

	// buffered text per panel
	thinkBuf  strings.Builder
	outputBuf strings.Builder
	reportBuf strings.Builder

	// last known snapshot
	snap  agent.Snapshot
	phase string
	focus string

	spin     spinner.Model
	cyclePB  progress.Model
	maxCycles int

	ready bool
}

func NewApp(eng *agent.Engine) *App {
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(colorAccent)

	pb := progress.New(
		progress.WithGradient(string(colorBorder), string(colorAccent)),
		progress.WithoutPercentage(),
	)

	return &App{
		eng:      eng,
		spin:     s,
		cyclePB:  pb,
		maxCycles: eng.MaxCycles(),
	}
}

func (a *App) Init() tea.Cmd {
	return tea.Batch(
		tickCmd(),
		a.spin.Tick,
		listenCmd(a.eng.Events()),
	)
}

func (a *App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch m := msg.(type) {

	case tea.WindowSizeMsg:
		a.width = m.Width
		a.height = m.Height
		a.cyclePB.Width = a.width/2 - 4
		a.initViewports()
		if !a.ready {
			a.ready = true
			a.eng.Start()
		}

	case tea.KeyMsg:
		switch m.String() {
		case "ctrl+c", "q":
			return a, tea.Quit
		}

	case spinner.TickMsg:
		var cmd tea.Cmd
		a.spin, cmd = a.spin.Update(m)
		cmds = append(cmds, cmd)

	case agentEventMsg:
		ev := agent.Event(m)
		a.phase = string(ev.Phase)
		if ev.Focus != "" {
			a.focus = ev.Focus
		}
		a.snap = ev.GraphSnap

		if ev.ThinkChunk != "" {
			a.thinkBuf.WriteString(ev.ThinkChunk)
			a.thinkVP.SetContent(thinkStyle.Render(a.thinkBuf.String()))
			a.thinkVP.GotoBottom()
		}
		if ev.OutputChunk != "" {
			a.outputBuf.WriteString(ev.OutputChunk)
			a.outputVP.SetContent(outputStyle.Render(a.outputBuf.String()))
			a.outputVP.GotoBottom()
		}
		if ev.ReportLine != "" {
			a.reportBuf.WriteString(ev.ReportLine)
			a.reportVP.SetContent(reportStyle.Render(a.reportBuf.String()))
			a.reportVP.GotoBottom()
		}
		if ev.Err != nil {
			a.outputBuf.WriteString(anomalyStyle.Render("\n⚠ ERROR: " + ev.Err.Error() + "\n"))
			a.outputVP.SetContent(a.outputBuf.String())
		}
		cmds = append(cmds, listenCmd(a.eng.Events()))

	case tickMsg:
		cmds = append(cmds, tickCmd())
	}

	var cmd tea.Cmd
	a.thinkVP, cmd = a.thinkVP.Update(msg)
	cmds = append(cmds, cmd)
	a.outputVP, cmd = a.outputVP.Update(msg)
	cmds = append(cmds, cmd)
	a.reportVP, cmd = a.reportVP.Update(msg)
	cmds = append(cmds, cmd)

	return a, tea.Batch(cmds...)
}

func (a *App) View() string {
	if !a.ready || a.width == 0 {
		return "\n  " + a.spin.View() + " Initialising Knowledge Archaeology Engine..."
	}

	header := a.renderHeader()
	bodyH  := a.height - lipgloss.Height(header) - 3
	leftW  := a.width * 2 / 5
	rightW := a.width - leftW - 3

	left  := a.renderLeft(leftW, bodyH)
	right := a.renderRight(rightW, bodyH)
	body  := lipgloss.JoinHorizontal(lipgloss.Top, left, right)
	footer := a.renderFooter()

	return lipgloss.JoinVertical(lipgloss.Left, header, body, footer)
}

func (a *App) renderHeader() string {
	title := headerStyle.Render("🧠 KNOWLEDGE ARCHAEOLOGY ENGINE")
	phase := a.spin.View() + " " + phaseStyle.Render(a.phase)
	focus := dimStyle.Render("focus: ") + outputStyle.Render(a.focus)

	qdrantDot := anomalyStyle.Render("○ qdrant offline")
	if a.snap.QdrantOK {
		qdrantDot = statValueStyle.Render("● qdrant") + "  " +
			statStyle.Render("vectors:") + " " + statValueStyle.Render(fmt.Sprintf("%d", a.snap.QdrantVectors))
	}

	stats := strings.Join([]string{
		statStyle.Render("nodes:")     + " " + statValueStyle.Render(fmt.Sprintf("%d", a.snap.Nodes)),
		statStyle.Render("edges:")     + " " + statValueStyle.Render(fmt.Sprintf("%d", a.snap.Edges)),
		statStyle.Render("anomalies:") + " " + anomalyStyle.Render(fmt.Sprintf("%d", a.snap.Anomalies)),
		statStyle.Render("cycle:")     + " " + statValueStyle.Render(fmt.Sprintf("%d", a.snap.Cycles)),
		qdrantDot,
	}, "  ")

	// cycle progress bar (only when a limit is set)
	var pbRow string
	if a.maxCycles > 0 {
		pct := float64(a.snap.Cycles) / float64(a.maxCycles)
		if pct > 1 {
			pct = 1
		}
		pbRow = "\n" + dimStyle.Render("  cycles ") + a.cyclePB.ViewAs(pct)
	}

	left := lipgloss.JoinVertical(lipgloss.Left,
		title,
		phase+" "+focus+pbRow,
	)
	row := lipgloss.NewStyle().
		Width(a.width).
		BorderBottom(true).
		BorderForeground(colorBorder).
		Padding(0, 1).
		Render(lipgloss.JoinHorizontal(lipgloss.Center,
			lipgloss.NewStyle().Width(a.width*2/5).Render(left),
			lipgloss.NewStyle().Width(a.width-a.width*2/5).Align(lipgloss.Right).Render(stats),
		))
	return row
}

func (a *App) renderLeft(w, h int) string {
	halfH := h / 2

	thinkTitle  := panelTitle("💭 THINKING", colorThink)
	thinkPanel  := borderStyle.Width(w - 2).Height(halfH - 2).Render(a.thinkVP.View())
	outputTitle := panelTitle("⚡ OUTPUT", colorAccent)
	outputPanel := borderStyle.Width(w - 2).Height(halfH - 2).Render(a.outputVP.View())

	return lipgloss.JoinVertical(lipgloss.Left,
		thinkTitle, thinkPanel,
		outputTitle, outputPanel,
	)
}

func (a *App) renderRight(w, h int) string {
	topTitle  := panelTitle("🔗 EMERGENT CONCEPTS", colorAccent)
	nodesContent := a.renderConceptBars(w - 4)
	nodesPanel   := borderStyle.Width(w - 2).Height(h/3 - 2).Render(nodesContent)

	reportTitle := panelTitle("📄 LIVE REPORT", colorReport)
	reportPanel := borderStyle.Width(w - 2).Height(h - h/3 - 4).Render(a.reportVP.View())

	return lipgloss.JoinVertical(lipgloss.Left,
		topTitle, nodesPanel,
		reportTitle, reportPanel,
	)
}

// renderConceptBars draws a horizontal bar chart for the top nodes.
func (a *App) renderConceptBars(availW int) string {
	if len(a.snap.TopNodes) == 0 {
		return dimStyle.Render("  waiting for graph data...")
	}

	// find max weight for scaling
	maxW := 0.1
	for _, w := range a.snap.TopWeights {
		if w > maxW {
			maxW = w
		}
	}

	labelW := 22
	barW   := availW - labelW - 10
	if barW < 4 {
		barW = 4
	}

	var sb strings.Builder
	for i, label := range a.snap.TopNodes {
		weight := 0.0
		if i < len(a.snap.TopWeights) {
			weight = a.snap.TopWeights[i]
		}

		// truncate label
		lbl := label
		if len(lbl) > labelW {
			lbl = lbl[:labelW-1] + "…"
		}
		lbl = fmt.Sprintf("%-*s", labelW, lbl)

		// bar
		filled := int(float64(barW) * weight / maxW)
		if filled < 1 {
			filled = 1
		}
		empty := barW - filled

		bar := barFillStyle.Render(strings.Repeat("█", filled)) +
			barBgStyle.Render(strings.Repeat("░", empty))

		weightStr := statValueStyle.Render(fmt.Sprintf(" %.1f", weight))

		sb.WriteString("  " + barLabelStyle.Render(lbl) + " " + bar + weightStr + "\n")
	}
	return sb.String()
}

func (a *App) renderFooter() string {
	help := dimStyle.Render("  q / ctrl+c — quit gracefully  |  report saves automatically")
	return lipgloss.NewStyle().
		Width(a.width).
		BorderTop(true).
		BorderForeground(colorBorder).
		Render(help)
}

func (a *App) initViewports() {
	bodyH  := a.height - 6
	leftW  := a.width*2/5 - 4
	rightW := a.width - a.width*2/5 - 4
	halfH  := bodyH / 2

	a.thinkVP  = viewport.New(leftW, halfH-4)
	a.outputVP = viewport.New(leftW, halfH-4)
	a.reportVP = viewport.New(rightW, bodyH-bodyH/3-6)
}

// ── Commands ──────────────────────────────────────────────────────────────────
func tickCmd() tea.Cmd {
	return tea.Tick(500*time.Millisecond, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

func listenCmd(ch <-chan agent.Event) tea.Cmd {
	return func() tea.Msg {
		ev, ok := <-ch
		if !ok {
			return nil
		}
		return agentEventMsg(ev)
	}
}

// ── Helpers ───────────────────────────────────────────────────────────────────
func panelTitle(label string, color lipgloss.Color) string {
	return lipgloss.NewStyle().
		Foreground(color).
		Bold(true).
		Padding(0, 1).
		Render(label)
}
