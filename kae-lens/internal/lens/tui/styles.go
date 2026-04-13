package tui

import "github.com/charmbracelet/lipgloss"

// Color palette — dark terminal aesthetic matching KAE
var (
	colorPrimary   = lipgloss.Color("#00D4FF") // cyan
	colorSecondary = lipgloss.Color("#9B59B6") // purple
	colorSuccess   = lipgloss.Color("#2ECC71") // green
	colorWarning   = lipgloss.Color("#F39C12") // orange
	colorDanger    = lipgloss.Color("#E74C3C") // red
	colorMuted     = lipgloss.Color("#636E72") // grey
	colorBright    = lipgloss.Color("#FFFFFF") // white
	colorDim       = lipgloss.Color("#2D3436") // dark bg

	// Finding type colors
	colorConnection    = lipgloss.Color("#00D4FF") // cyan
	colorContradiction = lipgloss.Color("#E74C3C") // red
	colorCluster       = lipgloss.Color("#9B59B6") // purple
	colorAnomaly       = lipgloss.Color("#F39C12") // orange
)

// Base styles
var (
	styleBase = lipgloss.NewStyle().
			Foreground(colorBright)

	styleMuted = lipgloss.NewStyle().
			Foreground(colorMuted)

	styleBold = lipgloss.NewStyle().
			Bold(true).
			Foreground(colorBright)

	styleTitle = lipgloss.NewStyle().
			Bold(true).
			Foreground(colorPrimary).
			BorderStyle(lipgloss.NormalBorder()).
			BorderBottom(true).
			BorderForeground(colorPrimary).
			PaddingBottom(1)

	styleHeader = lipgloss.NewStyle().
			Bold(true).
			Foreground(colorPrimary).
			Padding(0, 1)

	styleActive = lipgloss.NewStyle().
			Foreground(colorSuccess).
			Bold(true)

	styleIdle = lipgloss.NewStyle().
			Foreground(colorMuted)
)

// Panel styles
var (
	stylePanel = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colorMuted).
			Padding(0, 1)

	stylePanelActive = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(colorPrimary).
				Padding(0, 1)
)

// Finding type badge styles
var (
	styleBadgeConnection = lipgloss.NewStyle().
				Bold(true).
				Foreground(colorConnection)

	styleBadgeContradiction = lipgloss.NewStyle().
				Bold(true).
				Foreground(colorContradiction)

	styleBadgeCluster = lipgloss.NewStyle().
				Bold(true).
				Foreground(colorCluster)

	styleBadgeAnomaly = lipgloss.NewStyle().
				Bold(true).
				Foreground(colorAnomaly)
)

// Stat value styles
var (
	styleStatLabel = lipgloss.NewStyle().
			Foreground(colorMuted).
			Width(20)

	styleStatValue = lipgloss.NewStyle().
			Foreground(colorPrimary).
			Bold(true)
)

// findingBadge returns a colored type label for a finding type string.
func findingBadge(findingType string) string {
	switch findingType {
	case "connection":
		return styleBadgeConnection.Render("🔗 CONNECTION")
	case "contradiction":
		return styleBadgeContradiction.Render("⚡ CONTRADICTION")
	case "cluster":
		return styleBadgeCluster.Render("🌀 CLUSTER")
	case "anomaly":
		return styleBadgeAnomaly.Render("🔴 ANOMALY")
	default:
		return styleMuted.Render("? UNKNOWN")
	}
}

// confidenceBar renders a simple ASCII confidence indicator.
func confidenceBar(confidence float64) string {
	filled := int(confidence * 10)
	bar := ""
	for i := 0; i < 10; i++ {
		if i < filled {
			bar += "█"
		} else {
			bar += "░"
		}
	}
	color := colorSuccess
	if confidence < 0.7 {
		color = colorWarning
	}
	return lipgloss.NewStyle().Foreground(color).Render(bar)
}
