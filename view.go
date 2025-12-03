package main

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

func (m Model) View() string {
	if m.quitting {
		return ""
	}

	if m.err != nil {
		return fmt.Sprintf("Error: %v\nPress q to quit.", m.err)
	}

	if !m.ready {
		return "Loading..."
	}

	var view string
	switch m.mode {
	case ModeHutchPicker:
		view = m.viewHutchPicker()
	case ModeDatePicker:
		view = m.viewDatePicker()
	case ModeErrorList:
		view = m.viewErrorList()
	default:
		view = ""
	}

	// Overlay input dialog if in input mode
	if m.inputMode != InputNone {
		view = m.overlayInput(view)
	}

	return view
}

func (m Model) viewHutchPicker() string {
	var sb strings.Builder

	// Title
	title := titleStyle.Render("DAQ Error Browser")
	sb.WriteString(title)
	sb.WriteString("\n\n")

	// Instructions
	sb.WriteString("Select a hutch to browse:\n\n")

	// Hutch list
	visibleHutches := m.hutches
	if len(visibleHutches) > m.height-8 {
		visibleHutches = visibleHutches[:m.height-8]
	}

	for i, h := range visibleHutches {
		cursor := "  "
		style := normalStyle
		if i == m.hutchCursor {
			cursor = cursorStyle.Render("> ")
			style = selectedStyle
		}

		line := fmt.Sprintf("%-6s  (%d files, %d errors)", strings.ToUpper(h.Hutch), h.FileCount, h.ErrorCount)
		sb.WriteString(cursor)
		sb.WriteString(style.Render(line))
		sb.WriteString("\n")
	}

	// Help
	sb.WriteString("\n")
	if m.showHelp {
		sb.WriteString(m.help.View(m.keys))
	} else {
		sb.WriteString(helpStyle.Render("Press ? for help, q to quit"))
	}

	return sb.String()
}

func (m Model) viewDatePicker() string {
	var sb strings.Builder

	// Title
	title := titleStyle.Render(fmt.Sprintf("DAQ Error Browser - %s", strings.ToUpper(m.selectedHutch)))
	sb.WriteString(title)
	sb.WriteString("\n\n")

	// Instructions
	sb.WriteString("Select a date to browse errors:\n\n")

	// Date list
	visibleDates := m.dates
	if len(visibleDates) > m.height-8 {
		visibleDates = visibleDates[:m.height-8]
	}

	for i, d := range visibleDates {
		cursor := "  "
		style := normalStyle
		if i == m.cursor {
			cursor = cursorStyle.Render("> ")
			style = selectedStyle
		}

		line := fmt.Sprintf("%s  (%d files, %d errors)", d.Date, d.FileCount, d.ErrorCount)
		sb.WriteString(cursor)
		sb.WriteString(style.Render(line))
		sb.WriteString("\n")
	}

	// Help
	sb.WriteString("\n")
	if m.showHelp {
		sb.WriteString(m.help.View(m.keys))
	} else {
		sb.WriteString(helpStyle.Render("Press ? for help, q to quit"))
	}

	return sb.String()
}

func (m Model) viewErrorList() string {
	var sb strings.Builder

	// Calculate layout - three panels
	panelWidth := (m.width - 6) / 3
	if panelWidth < 20 {
		panelWidth = 20
	}

	// Build three panels
	leftPane := m.buildGroupsPane(panelWidth)
	middlePane := m.buildErrorsPane(panelWidth)
	rightPane := m.buildContextPane(panelWidth)

	// Join horizontally
	content := lipgloss.JoinHorizontal(
		lipgloss.Top,
		leftPane,
		" ",
		middlePane,
		" ",
		rightPane,
	)

	// Title bar with filter indicators
	titleText := fmt.Sprintf("DAQ Errors - %s - %s", strings.ToUpper(m.selectedHutch), m.selectedDate)
	title := titleStyle.Render(titleText)
	sb.WriteString(title)

	// Filter indicators in title line
	if m.levelFilter != "" || m.componentFilter != "" {
		sb.WriteString("  ")
		if m.levelFilter == "C" {
			sb.WriteString(criticalStyle.Render("[Critical only]"))
		}
		if m.componentFilter != "" {
			sb.WriteString(filterStyle.Render(fmt.Sprintf(" [/%s]", m.componentFilter)))
		}
	}
	sb.WriteString("\n\n")

	sb.WriteString(content)
	sb.WriteString("\n")

	// Status bar
	if len(m.groups) > 0 {
		status := fmt.Sprintf("Group %d/%d", m.groupCursor+1, len(m.groups))
		if m.groupCursor < len(m.groups) {
			g := m.groups[m.groupCursor]
			status += fmt.Sprintf("  |  Error %d/%d in group", m.errorCursor+1, len(g.Errors))
		}
		if len(m.filteredErrors) != len(m.allErrors) {
			status += fmt.Sprintf("  |  %d of %d total", len(m.filteredErrors), len(m.allErrors))
		}
		sb.WriteString(statusStyle.Render(status))
	} else {
		sb.WriteString(statusStyle.Render("No errors match filter"))
	}
	sb.WriteString("  ")

	// Help
	if m.showHelp {
		sb.WriteString("\n")
		sb.WriteString(m.help.View(m.keys))
	} else {
		focusHint := "groups"
		switch m.focusedPanel {
		case PanelErrors:
			focusHint = "errors"
		case PanelContext:
			focusHint = "context"
		}
		sb.WriteString(helpStyle.Render(fmt.Sprintf("↑↓ nav [%s]  tab switch  t time  c crit  / filter  a all  q quit", focusHint)))
	}

	return sb.String()
}

// buildGroupsPane builds the left panel showing error groups
func (m Model) buildGroupsPane(width int) string {
	var sb strings.Builder

	// Header
	header := "Groups"
	if m.focusedPanel == PanelGroups {
		header = dateHeaderStyle.Render("▸ Groups")
	} else {
		header = normalStyle.Render("  Groups")
	}
	sb.WriteString(header)
	sb.WriteString(fmt.Sprintf(" (%d)", len(m.groups)))
	sb.WriteString("\n")
	sb.WriteString(strings.Repeat("─", min(width-2, 25)))
	sb.WriteString("\n")

	if len(m.groups) == 0 {
		sb.WriteString(helpStyle.Render("No groups"))
		return panelBorderStyle(m.focusedPanel == PanelGroups).Width(width).Render(sb.String())
	}

	// Calculate visible range
	visibleCount := m.height - 10
	if visibleCount < 5 {
		visibleCount = 5
	}

	start := m.groupOffset
	end := start + visibleCount
	if end > len(m.groups) {
		end = len(m.groups)
	}

	for i := start; i < end; i++ {
		g := m.groups[i]

		// Cursor indicator
		cursor := "  "
		if i == m.groupCursor && m.focusedPanel == PanelGroups {
			cursor = cursorStyle.Render("> ")
		} else if i == m.groupCursor {
			cursor = "▸ "
		}

		// Format: "07:50 teb0 (15)"
		comp := g.Component
		if len(comp) > 12 {
			comp = comp[:9] + "..."
		}
		line := fmt.Sprintf("%s %-12s (%d)", g.Time, comp, len(g.Errors))

		// Style based on selection
		if i == m.groupCursor {
			line = selectedStyle.Render(line)
		} else {
			line = normalStyle.Render(line)
		}

		sb.WriteString(cursor)
		sb.WriteString(line)
		sb.WriteString("\n")
	}

	return panelBorderStyle(m.focusedPanel == PanelGroups).Width(width).Render(sb.String())
}

// buildErrorsPane builds the middle panel showing errors in selected group
func (m Model) buildErrorsPane(width int) string {
	var sb strings.Builder

	// Get filtered errors for current group
	errors := m.getFilteredGroupErrors()

	// Header
	header := "Errors"
	if m.focusedPanel == PanelErrors {
		header = dateHeaderStyle.Render("▸ Errors")
	} else {
		header = normalStyle.Render("  Errors")
	}

	if m.groupCursor < len(m.groups) {
		g := m.groups[m.groupCursor]
		sb.WriteString(header)
		// Show filtered count vs total
		if m.messageFilter != "" && len(errors) != len(g.Errors) {
			sb.WriteString(fmt.Sprintf(" in %s %s (%d/%d)", g.Time, g.Component, len(errors), len(g.Errors)))
		} else {
			sb.WriteString(fmt.Sprintf(" in %s %s (%d)", g.Time, g.Component, len(errors)))
		}
	} else {
		sb.WriteString(header)
	}
	sb.WriteString("\n")
	sb.WriteString(strings.Repeat("─", min(width-2, 40)))
	sb.WriteString("\n")

	if len(errors) == 0 {
		if m.messageFilter != "" {
			sb.WriteString(helpStyle.Render("No matching errors"))
		} else {
			sb.WriteString(helpStyle.Render("No errors in group"))
		}
		return panelBorderStyle(m.focusedPanel == PanelErrors).Width(width).Render(sb.String())
	}

	// Calculate visible range
	visibleCount := m.height - 10
	if visibleCount < 5 {
		visibleCount = 5
	}

	start := m.errorOffset
	end := start + visibleCount
	if end > len(errors) {
		end = len(errors)
	}

	for i := start; i < end; i++ {
		e := errors[i]

		// Cursor indicator
		cursor := "  "
		if i == m.errorCursor && m.focusedPanel == PanelErrors {
			cursor = cursorStyle.Render("> ")
		} else if i == m.errorCursor {
			cursor = "▸ "
		}

		// Level indicator
		level := fmt.Sprintf("[%s]", e.LogLevel)
		levelStyle := ErrorLevelStyle(e.LogLevel, e.ErrorType)

		// Message preview
		msgWidth := width - 10
		if msgWidth < 10 {
			msgWidth = 10
		}
		msg := e.Message
		if len(msg) > msgWidth {
			msg = msg[:msgWidth-3] + "..."
		}

		line := fmt.Sprintf("%s %s", levelStyle.Render(level), msg)

		sb.WriteString(cursor)
		sb.WriteString(line)
		sb.WriteString("\n")
	}

	return panelBorderStyle(m.focusedPanel == PanelErrors).Width(width).Render(sb.String())
}

// buildContextPane builds the right panel showing error context
func (m Model) buildContextPane(width int) string {
	if len(m.groups) == 0 {
		return panelBorderStyle(m.focusedPanel == PanelContext).Width(width).Render("No errors")
	}

	// Use the viewport content with focus-aware border
	return panelBorderStyle(m.focusedPanel == PanelContext).Width(width).Render(m.viewport.View())
}

// panelBorderStyle returns a style for panel borders based on focus
func panelBorderStyle(focused bool) lipgloss.Style {
	if focused {
		return lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colorBlue).
			Padding(0, 1)
	}
	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(colorDimGray).
		Padding(0, 1)
}

// overlayInput renders an input dialog on top of the view
func (m Model) overlayInput(baseView string) string {
	var title, prompt string

	switch m.inputMode {
	case InputTimeJump:
		title = "Jump to Time"
		prompt = "Enter time (HH:MM): " + m.timeInput.View()
	case InputComponentFilter:
		title = "Filter by Component"
		prompt = "Component: " + m.filterInput.View()
	case InputMessageFilter:
		title = "Filter by Message"
		prompt = "Message: " + m.filterInput.View()
	default:
		return baseView
	}

	// Build the dialog box
	dialogStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(colorBlue).
		Padding(1, 2).
		Width(40)

	titleRendered := lipgloss.NewStyle().
		Bold(true).
		Foreground(colorBlue).
		Render(title)

	help := helpStyle.Render("Enter to confirm, Esc to cancel")

	dialogContent := titleRendered + "\n\n" + prompt + "\n\n" + help
	dialog := dialogStyle.Render(dialogContent)

	// Center the dialog
	lines := strings.Split(baseView, "\n")
	dialogLines := strings.Split(dialog, "\n")

	// Calculate position
	startRow := (len(lines) - len(dialogLines)) / 2
	if startRow < 0 {
		startRow = 0
	}

	// Overlay dialog onto base view
	for i, dLine := range dialogLines {
		targetRow := startRow + i
		if targetRow < len(lines) {
			// Center horizontally
			padding := (m.width - lipgloss.Width(dLine)) / 2
			if padding < 0 {
				padding = 0
			}
			lines[targetRow] = strings.Repeat(" ", padding) + dLine
		}
	}

	return strings.Join(lines, "\n")
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
