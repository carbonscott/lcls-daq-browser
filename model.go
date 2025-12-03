package main

import (
	"database/sql"
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
)

// InputMode for text input overlays
type InputMode int

const (
	InputNone InputMode = iota
	InputTimeJump
	InputComponentFilter
	InputMessageFilter
)

// Mode represents the current UI mode
type Mode int

const (
	ModeHutchPicker Mode = iota
	ModeDatePicker
	ModeErrorList
)

// Panel focus for three-panel layout
type Panel int

const (
	PanelGroups  Panel = iota // Left panel: error groups
	PanelErrors               // Middle panel: errors in group
	PanelContext              // Right panel: error context (scrollable)
)

// ErrorGroup represents errors grouped by (time, component)
type ErrorGroup struct {
	Time      string  // "07:50"
	Component string  // "teb0"
	Errors    []Error // All errors in this group
}

// keyMap defines keyboard bindings
type keyMap struct {
	Up           key.Binding
	Down         key.Binding
	PageUp       key.Binding
	PageDown     key.Binding
	Home         key.Binding
	End          key.Binding
	Enter        key.Binding
	Back         key.Binding
	Tab          key.Binding
	ShiftTab     key.Binding
	Quit         key.Binding
	Help         key.Binding
	JumpTime     key.Binding
	CriticalOnly key.Binding
	Search       key.Binding
	ClearFilter  key.Binding
	Refresh      key.Binding
}

func defaultKeyMap() keyMap {
	return keyMap{
		Up: key.NewBinding(
			key.WithKeys("up", "k"),
			key.WithHelp("↑/k", "up"),
		),
		Down: key.NewBinding(
			key.WithKeys("down", "j"),
			key.WithHelp("↓/j", "down"),
		),
		PageUp: key.NewBinding(
			key.WithKeys("left", "pgup", "b", "h"),
			key.WithHelp("←/h", "page up"),
		),
		PageDown: key.NewBinding(
			key.WithKeys("right", "pgdown", "f", "l"),
			key.WithHelp("→/l", "page down"),
		),
		Home: key.NewBinding(
			key.WithKeys("home", "g"),
			key.WithHelp("g/home", "first"),
		),
		End: key.NewBinding(
			key.WithKeys("end", "G"),
			key.WithHelp("G/end", "last"),
		),
		Enter: key.NewBinding(
			key.WithKeys("enter"),
			key.WithHelp("enter", "select"),
		),
		Back: key.NewBinding(
			key.WithKeys("esc"),
			key.WithHelp("esc", "back"),
		),
		Tab: key.NewBinding(
			key.WithKeys("tab"),
			key.WithHelp("tab", "next panel"),
		),
		ShiftTab: key.NewBinding(
			key.WithKeys("shift+tab"),
			key.WithHelp("shift+tab", "prev panel"),
		),
		Quit: key.NewBinding(
			key.WithKeys("q", "ctrl+c"),
			key.WithHelp("q", "quit"),
		),
		Help: key.NewBinding(
			key.WithKeys("?"),
			key.WithHelp("?", "help"),
		),
		JumpTime: key.NewBinding(
			key.WithKeys("t"),
			key.WithHelp("t", "jump to time"),
		),
		CriticalOnly: key.NewBinding(
			key.WithKeys("c"),
			key.WithHelp("c", "critical only"),
		),
		Search: key.NewBinding(
			key.WithKeys("/"),
			key.WithHelp("/", "filter component"),
		),
		ClearFilter: key.NewBinding(
			key.WithKeys("a"),
			key.WithHelp("a", "show all"),
		),
		Refresh: key.NewBinding(
			key.WithKeys("r"),
			key.WithHelp("r", "refresh"),
		),
	}
}

func (k keyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Up, k.Down, k.JumpTime, k.CriticalOnly, k.Search, k.ClearFilter, k.Refresh, k.Quit}
}

func (k keyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{k.Up, k.Down, k.PageUp, k.PageDown},
		{k.Home, k.End, k.Enter, k.Back, k.Quit},
	}
}

// Model is the main Bubbletea model
type Model struct {
	// Database
	db *sql.DB

	// Data
	hutches        []HutchSummary
	dates          []DateSummary
	allErrors      []Error      // Full unfiltered list
	filteredErrors []Error      // Currently visible (after filters)
	groups         []ErrorGroup // Grouped by (time, component)

	// Navigation - three panel layout
	mode         Mode
	focusedPanel Panel // Which panel has focus
	groupCursor  int   // Selected group in left panel
	errorCursor  int   // Selected error in middle panel
	groupOffset  int   // Scroll offset for groups
	errorOffset  int   // Scroll offset for errors
	pageSize     int

	// Legacy (kept for compatibility)
	cursor     int
	pageOffset int

	// Hutch selection
	hutchCursor   int
	selectedHutch string

	// Current selection
	selectedDate string

	// Filtering
	levelFilter     string // "", "C", or "E"
	componentFilter string // "" or component substring (for groups panel)
	messageFilter   string // "" or message substring (for errors panel)
	inputMode       InputMode
	timeInput       textinput.Model
	filterInput     textinput.Model

	// Viewport for context pane
	viewport viewport.Model

	// Help
	help     help.Model
	keys     keyMap
	showHelp bool

	// Terminal size
	width  int
	height int

	// State
	ready    bool
	quitting bool
	err      error
}

// NewModel creates a new model
func NewModel(db *sql.DB, initialHutch, initialDate, initialTime string) Model {
	h := help.New()
	h.ShowAll = false

	// Initialize time input
	ti := textinput.New()
	ti.Placeholder = "HH:MM"
	ti.CharLimit = 5
	ti.Width = 10

	// Initialize filter input
	fi := textinput.New()
	fi.Placeholder = "component name"
	fi.CharLimit = 30
	fi.Width = 25

	m := Model{
		db:          db,
		mode:        ModeHutchPicker,
		keys:        defaultKeyMap(),
		help:        h,
		pageSize:    15,
		timeInput:   ti,
		filterInput: fi,
		inputMode:   InputNone,
	}

	// Load hutches
	hutches, err := GetHutchesWithErrors(db)
	if err != nil {
		m.err = err
		return m
	}
	m.hutches = hutches

	// If initial hutch provided, skip to date picker
	if initialHutch != "" {
		m.selectedHutch = initialHutch
		// Find hutch index
		for i, h := range hutches {
			if h.Hutch == initialHutch {
				m.hutchCursor = i
				break
			}
		}

		// Load dates for this hutch
		dates, err := GetDatesWithErrors(db, initialHutch)
		if err != nil {
			m.err = err
			return m
		}
		m.dates = dates
		m.mode = ModeDatePicker

		// If initial date also provided, load errors directly
		if initialDate != "" {
			m.selectedDate = initialDate
			errors, err := LoadErrors(db, initialHutch, initialDate)
			if err != nil {
				m.err = err
				return m
			}
			m.allErrors = errors
			m.filteredErrors = errors
			m.mode = ModeErrorList

			// Pin to initial time if provided
			if initialTime != "" && len(errors) > 0 {
				m.cursor = FindNearestErrorIndex(errors, initialTime)
				// Adjust page offset to show cursor
				m.pageOffset = (m.cursor / m.pageSize) * m.pageSize
			}
		}
	}

	return m
}

func (m Model) Init() tea.Cmd {
	return nil
}

// updateContextPane updates the viewport with current error's context
func (m *Model) updateContextPane() {
	if m.mode != ModeErrorList || len(m.groups) == 0 {
		return
	}

	// Get filtered errors for current group
	errors := m.getFilteredGroupErrors()
	if len(errors) == 0 || m.errorCursor >= len(errors) {
		return
	}

	e := errors[m.errorCursor]
	content := formatContext(e, m.viewport.Width)
	m.viewport.SetContent(content)
	m.viewport.GotoTop()
}

// selectedError returns the currently selected error
func (m *Model) selectedError() *Error {
	errors := m.getFilteredGroupErrors()
	if len(errors) == 0 || m.errorCursor >= len(errors) {
		return nil
	}
	return &errors[m.errorCursor]
}

// formatContext formats error context for display with word wrapping
func formatContext(e Error, width int) string {
	var sb strings.Builder

	// Content width (account for padding/borders)
	contentWidth := width - 4
	if contentWidth < 20 {
		contentWidth = 20
	}

	// Header
	sb.WriteString(contextHeaderStyle.Render("Component: "))
	sb.WriteString(e.Component)
	sb.WriteString(" @ ")
	sb.WriteString(e.Host)
	sb.WriteString("\n")

	sb.WriteString(contextHeaderStyle.Render("File: "))
	sb.WriteString(wrapText(e.FilePath, contentWidth-6))
	sb.WriteString(":")
	sb.WriteString(fmt.Sprintf("%d", e.LineNumber))
	sb.WriteString("\n")

	sb.WriteString(contextHeaderStyle.Render("Type: "))
	sb.WriteString(e.ErrorType)
	sb.WriteString("  ")
	sb.WriteString(contextHeaderStyle.Render("Level: "))
	sb.WriteString(e.LogLevel)
	sb.WriteString("\n\n")

	// Context before
	if e.ContextBefore != "" {
		lines := strings.Split(e.ContextBefore, "\n")
		startLine := e.LineNumber - len(lines)
		for i, line := range lines {
			lineNum := startLine + i
			if lineNum > 0 {
				sb.WriteString(lineNumberStyle.Render(fmt.Sprintf("%4d ", lineNum)))
			} else {
				sb.WriteString("     ")
			}
			sb.WriteString(wrapText(line, contentWidth-6))
			sb.WriteString("\n")
		}
	}

	// Error line (no truncation - wrap instead)
	sb.WriteString(errorLineStyle.Render(fmt.Sprintf(">>> %4d ", e.LineNumber)))
	sb.WriteString(errorLineStyle.Render(wrapText(e.Message, contentWidth-10)))
	sb.WriteString("\n")

	// Context after
	if e.ContextAfter != "" {
		lines := strings.Split(e.ContextAfter, "\n")
		for i, line := range lines {
			lineNum := e.LineNumber + i + 1
			sb.WriteString(lineNumberStyle.Render(fmt.Sprintf("%4d ", lineNum)))
			sb.WriteString(wrapText(line, contentWidth-6))
			sb.WriteString("\n")
		}
	}

	return sb.String()
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max-3] + "..."
}

// wrapText wraps text to fit within the given width
func wrapText(text string, width int) string {
	if width <= 0 || len(text) <= width {
		return text
	}
	var result strings.Builder
	for len(text) > width {
		// Find last space before width, or break at width
		breakPoint := width
		for i := width - 1; i > 0; i-- {
			if text[i] == ' ' {
				breakPoint = i
				break
			}
		}
		result.WriteString(text[:breakPoint])
		result.WriteString("\n       ") // Indent continuation lines
		text = strings.TrimLeft(text[breakPoint:], " ")
	}
	result.WriteString(text)
	return result.String()
}
