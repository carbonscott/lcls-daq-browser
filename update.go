package main

import (
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
)

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

		// Initialize viewport if not ready
		if !m.ready {
			// Context pane gets 1/3 of screen
			vpWidth := m.width/3 - 4
			vpHeight := m.height - 8 // Leave room for header/footer
			m.viewport = viewport.New(vpWidth, vpHeight)
			m.viewport.Style = contextBorderStyle
			m.ready = true
			m.updateContextPane()
		} else {
			// Resize viewport
			m.viewport.Width = m.width/3 - 4
			m.viewport.Height = m.height - 8
		}
		return m, nil

	case tea.KeyMsg:
		// If in input mode, handle text input first
		if m.inputMode != InputNone {
			return m.updateInput(msg)
		}

		// Handle based on mode
		switch m.mode {
		case ModeHutchPicker:
			return m.updateHutchPicker(msg)
		case ModeDatePicker:
			return m.updateDatePicker(msg)
		case ModeErrorList:
			return m.updateErrorList(msg)
		}

	case tea.MouseMsg:
		// Ignore mouse in input mode
		if m.inputMode != InputNone {
			return m, nil
		}
		return m.handleMouse(msg)
	}

	// Update viewport
	var cmd tea.Cmd
	m.viewport, cmd = m.viewport.Update(msg)
	return m, cmd
}

func (m Model) updateHutchPicker(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, m.keys.Quit):
		m.quitting = true
		return m, tea.Quit

	case key.Matches(msg, m.keys.Up):
		if m.hutchCursor > 0 {
			m.hutchCursor--
		}

	case key.Matches(msg, m.keys.Down):
		if m.hutchCursor < len(m.hutches)-1 {
			m.hutchCursor++
		}

	case key.Matches(msg, m.keys.Home):
		m.hutchCursor = 0

	case key.Matches(msg, m.keys.End):
		m.hutchCursor = len(m.hutches) - 1

	case key.Matches(msg, m.keys.Enter):
		if m.hutchCursor < len(m.hutches) {
			m.selectedHutch = m.hutches[m.hutchCursor].Hutch
			dates, err := GetDatesWithErrors(m.db, m.selectedHutch)
			if err != nil {
				m.err = err
				return m, nil
			}
			m.dates = dates
			m.cursor = 0
			m.mode = ModeDatePicker
		}

	case key.Matches(msg, m.keys.Help):
		m.showHelp = !m.showHelp
	}

	return m, nil
}

func (m Model) updateDatePicker(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, m.keys.Quit):
		m.quitting = true
		return m, tea.Quit

	case key.Matches(msg, m.keys.Back):
		// Go back to hutch picker
		m.mode = ModeHutchPicker
		m.dates = nil

	case key.Matches(msg, m.keys.Up):
		if m.cursor > 0 {
			m.cursor--
		}

	case key.Matches(msg, m.keys.Down):
		if m.cursor < len(m.dates)-1 {
			m.cursor++
		}

	case key.Matches(msg, m.keys.Home):
		m.cursor = 0

	case key.Matches(msg, m.keys.End):
		m.cursor = len(m.dates) - 1

	case key.Matches(msg, m.keys.Enter):
		if m.cursor < len(m.dates) {
			m.selectedDate = m.dates[m.cursor].Date
			errors, err := LoadErrors(m.db, m.selectedHutch, m.selectedDate)
			if err != nil {
				m.err = err
				return m, nil
			}
			m.allErrors = errors
			m.filteredErrors = errors
			m.levelFilter = ""
			m.componentFilter = ""
			m.buildGroups()
			m.mode = ModeErrorList
			m.focusedPanel = PanelGroups
			m.groupCursor = 0
			m.errorCursor = 0
			m.groupOffset = 0
			m.errorOffset = 0
			m.updateContextPane()
		}

	case key.Matches(msg, m.keys.Help):
		m.showHelp = !m.showHelp
	}

	return m, nil
}

func (m Model) updateErrorList(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, m.keys.Quit):
		m.quitting = true
		return m, tea.Quit

	case key.Matches(msg, m.keys.Back):
		switch m.focusedPanel {
		case PanelContext:
			// Go back to errors panel
			m.focusedPanel = PanelErrors
		case PanelErrors:
			// Go back to groups panel
			m.focusedPanel = PanelGroups
		default:
			// Go back to date picker
			m.mode = ModeDatePicker
			m.cursor = 0
			for i, d := range m.dates {
				if d.Date == m.selectedDate {
					m.cursor = i
					break
				}
			}
			m.allErrors = nil
			m.filteredErrors = nil
			m.groups = nil
		}

	case key.Matches(msg, m.keys.Tab):
		// Cycle forward: Groups → Errors → Context → Groups
		switch m.focusedPanel {
		case PanelGroups:
			m.focusedPanel = PanelErrors
		case PanelErrors:
			m.focusedPanel = PanelContext
		case PanelContext:
			m.focusedPanel = PanelGroups
		}

	case key.Matches(msg, m.keys.ShiftTab):
		// Cycle backward: Groups → Context → Errors → Groups
		switch m.focusedPanel {
		case PanelGroups:
			m.focusedPanel = PanelContext
		case PanelErrors:
			m.focusedPanel = PanelGroups
		case PanelContext:
			m.focusedPanel = PanelErrors
		}

	case key.Matches(msg, m.keys.Enter):
		// Enter focuses the errors panel (drill into group)
		if m.focusedPanel == PanelGroups {
			m.focusedPanel = PanelErrors
			m.errorCursor = 0
			m.errorOffset = 0
		}

	case key.Matches(msg, m.keys.Up):
		if m.focusedPanel == PanelContext {
			m.viewport.LineUp(1)
		} else {
			m.navigateUp()
		}

	case key.Matches(msg, m.keys.Down):
		if m.focusedPanel == PanelContext {
			m.viewport.LineDown(1)
		} else {
			m.navigateDown()
		}

	case key.Matches(msg, m.keys.PageUp):
		if m.focusedPanel == PanelContext {
			m.viewport.HalfViewUp()
		} else {
			m.navigatePageUp()
		}

	case key.Matches(msg, m.keys.PageDown):
		if m.focusedPanel == PanelContext {
			m.viewport.HalfViewDown()
		} else {
			m.navigatePageDown()
		}

	case key.Matches(msg, m.keys.Home):
		m.navigateHome()

	case key.Matches(msg, m.keys.End):
		m.navigateEnd()

	case key.Matches(msg, m.keys.Help):
		m.showHelp = !m.showHelp

	// Jump to time
	case key.Matches(msg, m.keys.JumpTime):
		m.inputMode = InputTimeJump
		m.timeInput.SetValue("")
		m.timeInput.Focus()
		return m, textinput.Blink

	// Toggle critical-only filter
	case key.Matches(msg, m.keys.CriticalOnly):
		if m.levelFilter == "C" {
			m.levelFilter = ""
		} else {
			m.levelFilter = "C"
		}
		m.applyFilters()

	// Component filter
	case key.Matches(msg, m.keys.Search):
		// Panel-aware search: component filter for groups, message filter for errors
		switch m.focusedPanel {
		case PanelGroups:
			m.inputMode = InputComponentFilter
			m.filterInput.SetValue(m.componentFilter)
			m.filterInput.Focus()
			return m, textinput.Blink
		case PanelErrors:
			m.inputMode = InputMessageFilter
			m.filterInput.SetValue(m.messageFilter)
			m.filterInput.Focus()
			return m, textinput.Blink
		case PanelContext:
			// No-op for context panel
			return m, nil
		}

	// Clear all filters
	case key.Matches(msg, m.keys.ClearFilter):
		m.clearFilters()
	}

	return m, nil
}

// Navigation helpers for three-panel layout

func (m *Model) navigateUp() {
	if m.focusedPanel == PanelGroups {
		if m.groupCursor > 0 {
			m.groupCursor--
			if m.groupCursor < m.groupOffset {
				m.groupOffset = m.groupCursor
			}
			// Reset error cursor and message filter when changing groups
			m.errorCursor = 0
			m.errorOffset = 0
			m.messageFilter = ""
			m.updateContextPane()
		}
	} else {
		// PanelErrors - use filtered errors
		errors := m.getFilteredGroupErrors()
		if m.errorCursor > 0 {
			m.errorCursor--
			if m.errorCursor < m.errorOffset {
				m.errorOffset = m.errorCursor
			}
			m.updateContextPane()
		}
		_ = errors // silence unused warning
	}
}

func (m *Model) navigateDown() {
	visibleCount := m.height - 10
	if visibleCount < 5 {
		visibleCount = 5
	}

	if m.focusedPanel == PanelGroups {
		if m.groupCursor < len(m.groups)-1 {
			m.groupCursor++
			if m.groupCursor >= m.groupOffset+visibleCount {
				m.groupOffset = m.groupCursor - visibleCount + 1
			}
			// Reset error cursor and message filter when changing groups
			m.errorCursor = 0
			m.errorOffset = 0
			m.messageFilter = ""
			m.updateContextPane()
		}
	} else {
		// PanelErrors - use filtered errors
		errors := m.getFilteredGroupErrors()
		if m.errorCursor < len(errors)-1 {
			m.errorCursor++
			if m.errorCursor >= m.errorOffset+visibleCount {
				m.errorOffset = m.errorCursor - visibleCount + 1
			}
			m.updateContextPane()
		}
	}
}

func (m *Model) navigatePageUp() {
	pageSize := m.height - 10
	if pageSize < 5 {
		pageSize = 5
	}

	if m.focusedPanel == PanelGroups {
		m.groupCursor -= pageSize
		if m.groupCursor < 0 {
			m.groupCursor = 0
		}
		m.groupOffset = (m.groupCursor / pageSize) * pageSize
		m.errorCursor = 0
		m.errorOffset = 0
		m.messageFilter = ""
		m.updateContextPane()
	} else {
		m.errorCursor -= pageSize
		if m.errorCursor < 0 {
			m.errorCursor = 0
		}
		m.errorOffset = (m.errorCursor / pageSize) * pageSize
		m.updateContextPane()
	}
}

func (m *Model) navigatePageDown() {
	pageSize := m.height - 10
	if pageSize < 5 {
		pageSize = 5
	}

	if m.focusedPanel == PanelGroups {
		m.groupCursor += pageSize
		if m.groupCursor >= len(m.groups) {
			m.groupCursor = len(m.groups) - 1
		}
		if m.groupCursor < 0 {
			m.groupCursor = 0
		}
		m.groupOffset = (m.groupCursor / pageSize) * pageSize
		m.errorCursor = 0
		m.errorOffset = 0
		m.messageFilter = ""
		m.updateContextPane()
	} else {
		errors := m.getFilteredGroupErrors()
		m.errorCursor += pageSize
		if m.errorCursor >= len(errors) {
			m.errorCursor = len(errors) - 1
		}
		if m.errorCursor < 0 {
			m.errorCursor = 0
		}
		m.errorOffset = (m.errorCursor / pageSize) * pageSize
		m.updateContextPane()
	}
}

func (m *Model) navigateHome() {
	if m.focusedPanel == PanelGroups {
		m.groupCursor = 0
		m.groupOffset = 0
		m.errorCursor = 0
		m.errorOffset = 0
		m.messageFilter = ""
		m.updateContextPane()
	} else {
		m.errorCursor = 0
		m.errorOffset = 0
		m.updateContextPane()
	}
}

func (m *Model) navigateEnd() {
	pageSize := m.height - 10
	if pageSize < 5 {
		pageSize = 5
	}

	if m.focusedPanel == PanelGroups {
		m.groupCursor = len(m.groups) - 1
		if m.groupCursor < 0 {
			m.groupCursor = 0
		}
		m.groupOffset = len(m.groups) - pageSize
		if m.groupOffset < 0 {
			m.groupOffset = 0
		}
		m.errorCursor = 0
		m.errorOffset = 0
		m.messageFilter = ""
		m.updateContextPane()
	} else {
		errors := m.getFilteredGroupErrors()
		m.errorCursor = len(errors) - 1
		if m.errorCursor < 0 {
			m.errorCursor = 0
		}
		m.errorOffset = len(errors) - pageSize
		if m.errorOffset < 0 {
			m.errorOffset = 0
		}
		m.updateContextPane()
	}
}

// updateInput handles text input mode
func (m Model) updateInput(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEsc:
		// Cancel input
		m.inputMode = InputNone
		m.timeInput.Blur()
		m.filterInput.Blur()
		return m, nil

	case tea.KeyEnter:
		// Apply input
		switch m.inputMode {
		case InputTimeJump:
			timeStr := m.timeInput.Value()
			m.jumpToTime(timeStr)
		case InputComponentFilter:
			m.componentFilter = m.filterInput.Value()
			m.applyFilters()
		case InputMessageFilter:
			m.messageFilter = m.filterInput.Value()
			m.applyMessageFilter()
		}
		m.inputMode = InputNone
		m.timeInput.Blur()
		m.filterInput.Blur()
		return m, nil
	}

	// Update the active text input
	var cmd tea.Cmd
	switch m.inputMode {
	case InputTimeJump:
		m.timeInput, cmd = m.timeInput.Update(msg)
	case InputComponentFilter, InputMessageFilter:
		m.filterInput, cmd = m.filterInput.Update(msg)
	}
	return m, cmd
}

// handleMouse processes mouse events for all modes
func (m Model) handleMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	switch m.mode {
	case ModeHutchPicker:
		return m.handleMouseHutchPicker(msg)
	case ModeDatePicker:
		return m.handleMouseDatePicker(msg)
	case ModeErrorList:
		return m.handleMouseErrorList(msg)
	}
	return m, nil
}

// handleMouseHutchPicker handles mouse in hutch selection screen
func (m Model) handleMouseHutchPicker(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	listStartY := 2 // List starts at row 2
	visibleRows := m.height - 8
	if visibleRows < 1 {
		visibleRows = 1
	}

	switch msg.Button {
	case tea.MouseButtonLeft:
		if msg.Action != tea.MouseActionPress {
			return m, nil
		}
		clickedRow := msg.Y - listStartY
		if clickedRow >= 0 && clickedRow < visibleRows && clickedRow < len(m.hutches) {
			m.hutchCursor = clickedRow
		}

	case tea.MouseButtonWheelUp:
		if m.hutchCursor > 0 {
			m.hutchCursor--
		}

	case tea.MouseButtonWheelDown:
		if m.hutchCursor < len(m.hutches)-1 {
			m.hutchCursor++
		}
	}

	return m, nil
}

// handleMouseDatePicker handles mouse in date selection screen
func (m Model) handleMouseDatePicker(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	listStartY := 2 // List starts at row 2
	visibleRows := m.height - 8
	if visibleRows < 1 {
		visibleRows = 1
	}

	switch msg.Button {
	case tea.MouseButtonLeft:
		if msg.Action != tea.MouseActionPress {
			return m, nil
		}
		clickedRow := msg.Y - listStartY
		if clickedRow >= 0 && clickedRow < visibleRows && clickedRow < len(m.dates) {
			m.cursor = clickedRow
		}

	case tea.MouseButtonWheelUp:
		if m.cursor > 0 {
			m.cursor--
		}

	case tea.MouseButtonWheelDown:
		if m.cursor < len(m.dates)-1 {
			m.cursor++
		}
	}

	return m, nil
}

// handleMouseErrorList handles mouse in three-panel error list screen
func (m Model) handleMouseErrorList(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	// Calculate panel boundaries
	panelWidth := (m.width - 6) / 3
	listStartY := 3 // Content starts at row 3 (after header + separator)
	visibleRows := m.height - 10
	if visibleRows < 1 {
		visibleRows = 1
	}

	// Determine which panel was clicked based on X coordinate
	var clickedPanel Panel
	if msg.X < panelWidth+2 {
		clickedPanel = PanelGroups
	} else if msg.X < panelWidth*2+4 {
		clickedPanel = PanelErrors
	} else {
		clickedPanel = PanelContext
	}

	switch msg.Button {
	case tea.MouseButtonLeft:
		if msg.Action != tea.MouseActionPress {
			return m, nil
		}

		// Focus the clicked panel
		m.focusedPanel = clickedPanel
		clickedRow := msg.Y - listStartY

		if clickedRow >= 0 && clickedRow < visibleRows {
			switch clickedPanel {
			case PanelGroups:
				newCursor := m.groupOffset + clickedRow
				if newCursor >= 0 && newCursor < len(m.groups) {
					m.groupCursor = newCursor
					m.errorCursor = 0
					m.errorOffset = 0
					m.messageFilter = ""
					m.updateContextPane()
				}

			case PanelErrors:
				errors := m.getFilteredGroupErrors()
				newCursor := m.errorOffset + clickedRow
				if newCursor >= 0 && newCursor < len(errors) {
					m.errorCursor = newCursor
					m.updateContextPane()
				}
			}
		}

	case tea.MouseButtonWheelUp:
		switch m.focusedPanel {
		case PanelGroups:
			m.navigateUp()
		case PanelErrors:
			m.navigateUp()
		case PanelContext:
			m.viewport.LineUp(3)
		}

	case tea.MouseButtonWheelDown:
		switch m.focusedPanel {
		case PanelGroups:
			m.navigateDown()
		case PanelErrors:
			m.navigateDown()
		case PanelContext:
			m.viewport.LineDown(3)
		}
	}

	return m, nil
}
