package main

import (
	"database/sql"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"
)

// Error represents a single error from the database
type Error struct {
	ID            int
	Timestamp     string
	Component     string
	Host          string
	LogLevel      string
	ErrorType     string
	Message       string
	LineNumber    int
	FilePath      string
	ContextBefore string
	ContextAfter  string
}

// DateSummary represents a date with error counts
type DateSummary struct {
	Date       string
	FileCount  int
	ErrorCount int
}

// HutchSummary represents a hutch with error counts
type HutchSummary struct {
	Hutch      string
	FileCount  int
	ErrorCount int
}

// GetHutchesWithErrors returns hutches that have errors, sorted alphabetically
func GetHutchesWithErrors(db *sql.DB) ([]HutchSummary, error) {
	query := `
		SELECT hutch,
		       COUNT(DISTINCT id) as files,
		       SUM(error_count) as errors
		FROM log_files
		WHERE error_count > 0
		GROUP BY hutch
		ORDER BY hutch
	`
	rows, err := db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var hutches []HutchSummary
	for rows.Next() {
		var h HutchSummary
		if err := rows.Scan(&h.Hutch, &h.FileCount, &h.ErrorCount); err != nil {
			return nil, err
		}
		hutches = append(hutches, h)
	}
	return hutches, rows.Err()
}

// GetDatesWithErrors returns dates that have errors for a specific hutch, sorted descending
func GetDatesWithErrors(db *sql.DB, hutch string) ([]DateSummary, error) {
	query := `
		SELECT DATE(start_timestamp_utc) as date,
		       COUNT(DISTINCT lf.id) as files,
		       SUM(error_count) as errors
		FROM log_files lf
		WHERE hutch = ? AND error_count > 0
		GROUP BY date
		ORDER BY date DESC
		LIMIT 60
	`
	rows, err := db.Query(query, hutch)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var dates []DateSummary
	for rows.Next() {
		var d DateSummary
		if err := rows.Scan(&d.Date, &d.FileCount, &d.ErrorCount); err != nil {
			return nil, err
		}
		dates = append(dates, d)
	}
	return dates, rows.Err()
}

// LoadErrors loads errors for a specific hutch and date, ordered by timestamp
func LoadErrors(db *sql.DB, hutch, date string) ([]Error, error) {
	query := `
		SELECT le.id,
		       COALESCE(le.timestamp_utc, '') as timestamp,
		       lf.component,
		       lf.host,
		       le.log_level,
		       le.error_type,
		       le.message,
		       le.line_number,
		       lf.file_path,
		       COALESCE(le.context_before, '') as ctx_before,
		       COALESCE(le.context_after, '') as ctx_after
		FROM log_errors le
		JOIN log_files lf ON le.log_file_id = lf.id
		WHERE lf.hutch = ?
		  AND DATE(lf.start_timestamp_utc) = ?
		  AND NOT (le.error_type = 'slurm' AND le.message LIKE '%CANCELLED%')
		  AND NOT (le.error_type = 'slurm' AND le.message LIKE '%Job step aborted%')
	`
	rows, err := db.Query(query, hutch, date)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var errors []Error
	for rows.Next() {
		var e Error
		if err := rows.Scan(
			&e.ID, &e.Timestamp, &e.Component, &e.Host,
			&e.LogLevel, &e.ErrorType, &e.Message, &e.LineNumber,
			&e.FilePath, &e.ContextBefore, &e.ContextAfter,
		); err != nil {
			return nil, err
		}
		// Extract time from filepath if timestamp is empty
		if e.Timestamp == "" {
			e.Timestamp = extractTimeFromPath(e.FilePath)
		}
		errors = append(errors, e)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	// Sort by time (chronologically) using filepath timestamp
	sort.Slice(errors, func(i, j int) bool {
		ti := getErrorSortTime(errors[i])
		tj := getErrorSortTime(errors[j])
		if ti != tj {
			return ti < tj
		}
		// Secondary sort by line number within same file
		return errors[i].LineNumber < errors[j].LineNumber
	})

	return errors, nil
}

// getErrorSortTime extracts a sortable time string from error
// Returns "HH:MM:SS" format for proper string sorting
func getErrorSortTime(e Error) string {
	// First try filepath (most reliable: DD_HH:MM:SS_host:comp.log)
	timeStr := extractTimeFromPath(e.FilePath)
	if timeStr != "" {
		return timeStr // Already HH:MM:SS format
	}

	// Fall back to timestamp field
	if e.Timestamp != "" {
		// Try to extract time portion
		if len(e.Timestamp) >= 8 && e.Timestamp[2] == ':' && e.Timestamp[5] == ':' {
			return e.Timestamp[:8]
		}
		// Try parsing full datetime
		for _, layout := range []string{
			"2006-01-02 15:04:05",
			"2006-01-02T15:04:05",
		} {
			if t, err := time.Parse(layout, e.Timestamp); err == nil {
				return t.Format("15:04:05")
			}
		}
	}

	return "99:99:99" // Sort unknown times to end
}

// FindNearestErrorIndex finds the error closest to targetTime (HH:MM format)
func FindNearestErrorIndex(errors []Error, targetTime string) int {
	if len(errors) == 0 {
		return 0
	}

	targetMinutes := parseTimeToMinutes(targetTime)
	if targetMinutes < 0 {
		return 0
	}

	bestIdx := 0
	bestDiff := math.MaxInt32

	for i, e := range errors {
		errTime := extractTimeHHMM(e.Timestamp, e.FilePath)
		errMinutes := parseTimeToMinutes(errTime)
		if errMinutes < 0 {
			continue
		}

		diff := abs(errMinutes - targetMinutes)
		if diff < bestDiff {
			bestDiff = diff
			bestIdx = i
		}
	}
	return bestIdx
}

// extractTimeFromPath extracts HH:MM:SS from path like .../DD_HH:MM:SS_host:component.log
func extractTimeFromPath(path string) string {
	// Look for pattern DD_HH:MM:SS
	parts := strings.Split(path, "/")
	if len(parts) == 0 {
		return ""
	}
	filename := parts[len(parts)-1]
	// Format: DD_HH:MM:SS_host:component.log
	if len(filename) > 11 && filename[2] == '_' && filename[5] == ':' && filename[8] == ':' {
		return filename[3:11] // HH:MM:SS
	}
	return ""
}

// extractTimeHHMM gets HH:MM from timestamp or filepath
func extractTimeHHMM(timestamp, filepath string) string {
	if timestamp != "" {
		// Try to parse as time
		for _, layout := range []string{
			"2006-01-02 15:04:05",
			"15:04:05",
			"15:04",
		} {
			if t, err := time.Parse(layout, timestamp); err == nil {
				return t.Format("15:04")
			}
		}
		// Just take first 5 chars if looks like HH:MM
		if len(timestamp) >= 5 && timestamp[2] == ':' {
			return timestamp[:5]
		}
	}

	// Fallback to filepath
	timeStr := extractTimeFromPath(filepath)
	if len(timeStr) >= 5 {
		return timeStr[:5]
	}
	return ""
}

// parseTimeToMinutes converts HH:MM to minutes since midnight
func parseTimeToMinutes(timeStr string) int {
	if len(timeStr) < 5 || timeStr[2] != ':' {
		return -1
	}
	var h, m int
	_, err := fmt.Sscanf(timeStr, "%d:%d", &h, &m)
	if err != nil {
		return -1
	}
	return h*60 + m
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}
