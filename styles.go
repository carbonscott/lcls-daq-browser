package main

import "github.com/charmbracelet/lipgloss"

var (
	// Colors
	colorRed     = lipgloss.Color("#ff6666")
	colorYellow  = lipgloss.Color("#ffcc66")
	colorGreen   = lipgloss.Color("#66ff66")
	colorBlue    = lipgloss.Color("#6699ff")
	colorGray    = lipgloss.Color("#888888")
	colorDimGray = lipgloss.Color("#555555")

	// Title bar
	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#ffffff")).
			Background(colorBlue).
			Padding(0, 1)

	// Date header
	dateHeaderStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(colorBlue).
			Padding(0, 1)

	// Error list styles
	cursorStyle = lipgloss.NewStyle().
			Foreground(colorGreen).
			Bold(true)

	selectedStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#ffffff")).
			Background(colorBlue)

	criticalStyle = lipgloss.NewStyle().
			Foreground(colorRed).
			Bold(true)

	errorStyle = lipgloss.NewStyle().
			Foreground(colorYellow)

	normalStyle = lipgloss.NewStyle().
			Foreground(colorGray)

	// Context pane
	contextBorderStyle = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(colorDimGray).
				Padding(0, 1)

	contextHeaderStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(colorBlue)

	errorLineStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(colorRed)

	lineNumberStyle = lipgloss.NewStyle().
			Foreground(colorDimGray)

	// Help bar
	helpStyle = lipgloss.NewStyle().
			Foreground(colorGray)

	helpKeyStyle = lipgloss.NewStyle().
			Foreground(colorBlue).
			Bold(true)

	// Status bar
	statusStyle = lipgloss.NewStyle().
			Foreground(colorGray).
			Padding(0, 1)

	// Filter indicator
	filterStyle = lipgloss.NewStyle().
			Foreground(colorYellow).
			Bold(true)
)

// ErrorLevelStyle returns style based on log level
func ErrorLevelStyle(level, errType string) lipgloss.Style {
	if level == "C" {
		return criticalStyle
	}
	if errType == "system" {
		return criticalStyle
	}
	if level == "E" {
		return errorStyle
	}
	return normalStyle
}
