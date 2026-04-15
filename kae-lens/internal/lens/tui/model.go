package tui

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/meistro/kae/internal/graph"
)

// msgFinding is a Bubbletea message carrying a new finding event.
type msgFinding struct{ event graph.FindingEvent }

// msgBatchStart is a Bubbletea message for a batch starting.
type msgBatchStart struct{ event graph.BatchStartEvent }

// msgBatchDone is a Bubbletea message for a batch completing.
type msgBatchDone struct{ event graph.BatchDoneEvent }

// msgStats is a Bubbletea message for stats updates.
type msgStats struct{ event graph.StatsEvent }

// msgTick is the periodic refresh tick.
type msgTick struct{}

// Model is the Bubbletea TUI model for KAE Lens.
type Model struct {
	// Layout
	width  int
	height int

	// State
	findings    []graph.FindingEvent
	stats       graph.StatsEvent
	activeBatch *graph.BatchStartEvent
	selectedIdx int
	showTrace   bool
	maxFeed     int

	// External events channel — receives events from the Lens pipeline
	// Use program.Send() to push events from goroutines
	program *tea.Program
}

// NewModel creates a new TUI Model.
func NewModel(maxFeedItems int) *Model {
	return &Model{
		findings: make([]graph.FindingEvent, 0),
		maxFeed:  maxFeedItems,
	}
}

// SetProgram stores a reference to the tea.Program for external event injection.
func (m *Model) SetProgram(p *tea.Program) {
	m.program = p
}

// SendFinding pushes a finding event into the TUI from an external goroutine.
func (m *Model) SendFinding(e graph.FindingEvent) {
	if m.program != nil {
		m.program.Send(msgFinding{event: e})
	}
}

// SendBatchStart pushes a batch start event.
func (m *Model) SendBatchStart(e graph.BatchStartEvent) {
	if m.program != nil {
		m.program.Send(msgBatchStart{event: e})
	}
}

// SendBatchDone pushes a batch done event.
func (m *Model) SendBatchDone(e graph.BatchDoneEvent) {
	if m.program != nil {
		m.program.Send(msgBatchDone{event: e})
	}
}

// SendStats pushes a stats update event.
func (m *Model) SendStats(e graph.StatsEvent) {
	if m.program != nil {
		m.program.Send(msgStats{event: e})
	}
}

// --- Bubbletea interface ---

// Init returns the initial command: start the refresh tick.
func (m Model) Init() tea.Cmd {
	return tickCmd()
}

// Update handles all incoming messages and updates the model.
// This is the only place state is mutated — no goroutines, per Bubbletea rules.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q":
			return m, tea.Quit
		case "up", "k":
			if m.selectedIdx > 0 {
				m.selectedIdx--
			}
		case "down", "j":
			if m.selectedIdx < len(m.findings)-1 {
				m.selectedIdx++
			}
		case "enter", " ":
			m.showTrace = !m.showTrace
		case "esc":
			m.showTrace = false
		}

	case msgFinding:
		m.findings = append(m.findings, msg.event)
		if len(m.findings) > m.maxFeed {
			m.findings = m.findings[len(m.findings)-m.maxFeed:]
		}
		// Keep selection on latest if already at bottom
		if m.selectedIdx >= len(m.findings)-2 {
			m.selectedIdx = len(m.findings) - 1
		}
		// Update stats
		m.stats.FindingsInSession++
		switch msg.event.Type {
		case "connection":
			m.stats.FindingsByType.Connections++
		case "contradiction":
			m.stats.FindingsByType.Contradictions++
		case "cluster":
			m.stats.FindingsByType.Clusters++
		case "anomaly":
			m.stats.FindingsByType.Anomalies++
		}

	case msgBatchStart:
		m.activeBatch = &msg.event
		m.stats.ActiveBatch = true
		m.stats.BatchProgress = fmt.Sprintf("%s (%d pts)", msg.event.BatchID, msg.event.PointCount)

	case msgBatchDone:
		m.activeBatch = nil
		m.stats.ActiveBatch = false
		m.stats.BatchProgress = ""

	case msgStats:
		// Update global stats from Qdrant
		m.stats.TotalKnowledgePoints = msg.event.TotalKnowledgePoints
		m.stats.TotalFindings = msg.event.TotalFindings

	case msgTick:
		return m, tickCmd()
	}

	return m, nil
}

// View renders the full TUI.
func (m Model) View() string {
	if m.width == 0 {
		return "initializing..."
	}

	// Split layout: left stats panel + right feed panel
	leftWidth := 35
	rightWidth := m.width - leftWidth - 4 // 4 for borders/padding

	left := m.renderStatsPanel(leftWidth)
	right := m.renderFeedPanel(rightWidth)

	top := lipgloss.JoinHorizontal(lipgloss.Top, left, right)

	// Bottom: reasoning trace if a finding is selected and expanded
	bottom := ""
	if m.showTrace && len(m.findings) > 0 && m.selectedIdx < len(m.findings) {
		bottom = "\n" + m.renderTracePanel(m.width)
	}

	// Help bar
	help := styleMuted.Render("  ↑↓ navigate  enter toggle trace+correction  q quit")

	return m.renderTitleBar() + "\n" + top + bottom + "\n" + help
}

// --- render helpers ---

func (m Model) renderTitleBar() string {
	status := styleIdle.Render("● IDLE")
	if m.stats.ActiveBatch {
		status = styleActive.Render("● ACTIVE  " + m.stats.BatchProgress)
	}

	title := styleBold.Render("KAE LENS")
	version := styleMuted.Render("v0.1.0")
	counts := styleMuted.Render(fmt.Sprintf(
		"findings: %d  processed: %d",
		m.stats.FindingsInSession,
		m.stats.ProcessedInSession,
	))

	left := lipgloss.JoinHorizontal(lipgloss.Center, " ", title, "  ", version)
	right := lipgloss.JoinHorizontal(lipgloss.Center, counts, "  ", status, " ")
	spacer := strings.Repeat(" ", max(0, m.width-lipgloss.Width(left)-lipgloss.Width(right)))

	bar := lipgloss.NewStyle().
		Background(lipgloss.Color("#1a1a2e")).
		Foreground(colorPrimary).
		Width(m.width).
		Render(left + spacer + right)

	return bar
}

func (m Model) renderStatsPanel(width int) string {
	s := m.stats
	lines := []string{
		styleHeader.Render("STATS"),
		"",
		row("Knowledge pts", fmt.Sprintf("%d", s.TotalKnowledgePoints)),
		row("Total findings", fmt.Sprintf("%d", s.TotalFindings)),
		row("Session pts", fmt.Sprintf("%d", s.ProcessedInSession)),
		row("Session finds", fmt.Sprintf("%d", s.FindingsInSession)),
		"",
		styleHeader.Render("BY TYPE"),
		"",
		row("🔗 Connections", fmt.Sprintf("%d", s.FindingsByType.Connections)),
		row("⚡ Contradictions", fmt.Sprintf("%d", s.FindingsByType.Contradictions)),
		row("🌀 Clusters", fmt.Sprintf("%d", s.FindingsByType.Clusters)),
		row("🔴 Anomalies", fmt.Sprintf("%d", s.FindingsByType.Anomalies)),
	}

	content := strings.Join(lines, "\n")
	return stylePanel.Width(width).Height(m.feedHeight()).Render(content)
}

func (m Model) renderFeedPanel(width int) string {
	header := styleHeader.Render(fmt.Sprintf("LIVE FINDINGS FEED  (%d)", len(m.findings)))

	if len(m.findings) == 0 {
		empty := styleMuted.Render("  waiting for KAE to ingest data...")
		return stylePanelActive.Width(width).Height(m.feedHeight()).Render(header + "\n\n" + empty)
	}

	// Show findings newest-first (reverse order)
	var lines []string
	visibleCount := min(m.feedHeight()-4, len(m.findings))
	start := len(m.findings) - visibleCount

	for i := len(m.findings) - 1; i >= start; i-- {
		f := m.findings[i]
		selected := i == m.selectedIdx

		prefix := "  "
		if selected {
			prefix = styleActive.Render("▶ ")
		}

		badge := findingBadge(f.Type)
		conf := fmt.Sprintf("%.2f", f.Confidence)
		confBar := confidenceBar(f.Confidence)
		age := formatAge(f.CreatedAt)

		// Truncate summary to fit panel width
		summary := f.Summary
		maxSummaryLen := width - 6
		if len(summary) > maxSummaryLen {
			summary = summary[:maxSummaryLen-3] + "..."
		}

		domains := styleMuted.Render(strings.Join(f.Domains, " · "))

		line := fmt.Sprintf("%s%s  %s %s  %s\n   %s  %s",
			prefix, badge, confBar, conf, age,
			summary, domains)

		if selected {
			line = lipgloss.NewStyle().
				Background(lipgloss.Color("#1a1a2e")).
				Render(line)
		}

		lines = append(lines, line)
	}

	content := header + "\n\n" + strings.Join(lines, "\n\n")
	return stylePanelActive.Width(width).Height(m.feedHeight()).Render(content)
}

func (m Model) renderTracePanel(width int) string {
	if m.selectedIdx >= len(m.findings) {
		return ""
	}
	f := m.findings[m.selectedIdx]

	title := styleHeader.Render(fmt.Sprintf("REASONING TRACE — %s", f.Summary[:min(60, len(f.Summary))]))

	trace := f.ReasoningTrace
	if trace == "" {
		trace = styleMuted.Render("(no reasoning trace available)")
	}

	content := title + "\n\n" + wordWrap(trace, width-6)

	// Show data-grounded correction for anomaly/contradiction findings
	if f.Correction != "" {
		corrLabel := styleHeader.Render("DATA CORRECTION")
		content += "\n\n" + corrLabel + "\n\n" + wordWrap(f.Correction, width-6)
	}

	// Show source paper links
	if len(f.SourceURLs) > 0 {
		srcLabel := styleHeader.Render("SOURCES")
		var srcLines []string
		for id, u := range f.SourceURLs {
			short := id
			if len(short) > 12 {
				short = short[:12] + "…"
			}
			srcLines = append(srcLines, fmt.Sprintf("[%s] %s", short, u))
		}
		content += "\n\n" + srcLabel + "\n\n" + strings.Join(srcLines, "\n")
	}

	return stylePanel.Width(width).Render(content)
}

func (m Model) feedHeight() int {
	h := m.height - 8 // title bar + help bar + padding
	if h < 10 {
		return 10
	}
	return h
}

// --- commands ---

func tickCmd() tea.Cmd {
	return tea.Tick(250*time.Millisecond, func(t time.Time) tea.Msg {
		return msgTick{}
	})
}

// --- utility ---

func row(label, value string) string {
	return styleStatLabel.Render(label) + styleStatValue.Render(value)
}

func formatAge(t time.Time) string {
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return styleMuted.Render(fmt.Sprintf("%ds ago", int(d.Seconds())))
	case d < time.Hour:
		return styleMuted.Render(fmt.Sprintf("%dm ago", int(d.Minutes())))
	default:
		return styleMuted.Render(fmt.Sprintf("%dh ago", int(d.Hours())))
	}
}

func wordWrap(text string, width int) string {
	if width <= 0 {
		return text
	}
	words := strings.Fields(text)
	var lines []string
	var current strings.Builder

	for _, word := range words {
		if current.Len()+len(word)+1 > width {
			lines = append(lines, current.String())
			current.Reset()
		}
		if current.Len() > 0 {
			current.WriteString(" ")
		}
		current.WriteString(word)
	}
	if current.Len() > 0 {
		lines = append(lines, current.String())
	}
	return strings.Join(lines, "\n")
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
