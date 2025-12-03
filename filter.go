package main

import (
	"fmt"
	"sort"
	"strings"
)

// applyFilters filters allErrors based on levelFilter and componentFilter
func (m *Model) applyFilters() {
	m.filteredErrors = nil

	for _, e := range m.allErrors {
		// Level filter
		if m.levelFilter != "" && e.LogLevel != m.levelFilter {
			continue
		}

		// Component filter (case-insensitive substring match)
		if m.componentFilter != "" {
			if !strings.Contains(
				strings.ToLower(e.Component),
				strings.ToLower(m.componentFilter),
			) {
				continue
			}
		}

		m.filteredErrors = append(m.filteredErrors, e)
	}

	// Build groups from filtered errors
	m.buildGroups()

	// Reset cursors
	m.groupCursor = 0
	m.errorCursor = 0
	m.groupOffset = 0
	m.errorOffset = 0
	m.updateContextPane()
}

// clearFilters removes all filters but stays on the same error
func (m *Model) clearFilters() {
	// 1. Remember current error's ID before clearing
	var currentErrorID int
	errors := m.getFilteredGroupErrors()
	if m.errorCursor < len(errors) {
		currentErrorID = errors[m.errorCursor].ID
	}

	// 2. Clear filters and rebuild
	m.levelFilter = ""
	m.componentFilter = ""
	m.messageFilter = ""
	m.filterInput.SetValue("")
	m.filteredErrors = m.allErrors
	m.buildGroups()

	// 3. Find that error in the new list
	if currentErrorID > 0 {
		m.findAndSelectError(currentErrorID)
	}

	m.updateContextPane()
}

// findAndSelectError searches all groups for error with given ID
func (m *Model) findAndSelectError(errorID int) {
	for gi, group := range m.groups {
		for ei, e := range group.Errors {
			if e.ID == errorID {
				m.groupCursor = gi
				m.errorCursor = ei
				// Adjust offsets to show cursor
				pageSize := m.height - 10
				if pageSize < 5 {
					pageSize = 5
				}
				m.groupOffset = (m.groupCursor / pageSize) * pageSize
				m.errorOffset = (m.errorCursor / pageSize) * pageSize
				return
			}
		}
	}
}

// applyMessageFilter filters errors in the current group by message text
func (m *Model) applyMessageFilter() {
	// Reset cursor
	m.errorCursor = 0
	m.errorOffset = 0
	m.updateContextPane()
}

// getFilteredGroupErrors returns errors for current group, filtered by messageFilter
func (m Model) getFilteredGroupErrors() []Error {
	if m.groupCursor >= len(m.groups) {
		return nil
	}

	group := m.groups[m.groupCursor]

	// No filter? Return all errors
	if m.messageFilter == "" {
		return group.Errors
	}

	// Filter by message text
	var filtered []Error
	filterLower := strings.ToLower(m.messageFilter)
	for _, e := range group.Errors {
		if strings.Contains(strings.ToLower(e.Message), filterLower) {
			filtered = append(filtered, e)
		}
	}
	return filtered
}

// buildGroups creates error groups from filteredErrors
// Groups by (HH:MM, component) and sorts chronologically
func (m *Model) buildGroups() {
	m.groups = nil

	if len(m.filteredErrors) == 0 {
		return
	}

	// Group by (time, component)
	groupMap := make(map[string]*ErrorGroup)
	var groupOrder []string // Track insertion order for later sorting

	for _, e := range m.filteredErrors {
		timeStr := extractTimeHHMM(e.Timestamp, e.FilePath)
		if timeStr == "" {
			timeStr = "??:??"
		}
		key := timeStr + "|" + e.Component

		if g, ok := groupMap[key]; ok {
			g.Errors = append(g.Errors, e)
		} else {
			groupMap[key] = &ErrorGroup{
				Time:      timeStr,
				Component: e.Component,
				Errors:    []Error{e},
			}
			groupOrder = append(groupOrder, key)
		}
	}

	// Convert map to slice
	for _, key := range groupOrder {
		m.groups = append(m.groups, *groupMap[key])
	}

	// Sort groups chronologically by time, then by component
	sort.Slice(m.groups, func(i, j int) bool {
		if m.groups[i].Time != m.groups[j].Time {
			return m.groups[i].Time < m.groups[j].Time
		}
		return m.groups[i].Component < m.groups[j].Component
	})
}

// jumpToTime finds the group closest to the given time and moves cursor there
func (m *Model) jumpToTime(timeStr string) {
	if len(m.groups) == 0 {
		return
	}

	// Find the group with time closest to target
	targetMinutes := parseTimeToMinutes(timeStr)
	if targetMinutes < 0 {
		return
	}

	bestIdx := 0
	bestDiff := 24 * 60 // Max possible diff

	for i, g := range m.groups {
		groupMinutes := parseTimeToMinutes(g.Time)
		if groupMinutes < 0 {
			continue
		}
		diff := abs(groupMinutes - targetMinutes)
		if diff < bestDiff {
			bestDiff = diff
			bestIdx = i
		}
	}

	m.groupCursor = bestIdx
	m.errorCursor = 0
	m.groupOffset = (m.groupCursor / m.pageSize) * m.pageSize
	m.errorOffset = 0
	m.updateContextPane()
}

// filterCount returns a string like "(23/647)" showing filtered vs total
func (m *Model) filterCount() string {
	if len(m.allErrors) == 0 {
		return ""
	}
	if len(m.filteredErrors) == len(m.allErrors) {
		return ""
	}
	return fmt.Sprintf("(%d/%d)", len(m.filteredErrors), len(m.allErrors))
}

// totalErrorsInGroups counts total errors across all groups
func (m *Model) totalErrorsInGroups() int {
	total := 0
	for _, g := range m.groups {
		total += len(g.Errors)
	}
	return total
}
