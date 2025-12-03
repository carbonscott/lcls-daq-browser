package main

import (
	"database/sql"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"
)

// Pacific timezone for LCLS (handles DST automatically)
var pacificLoc *time.Location

func init() {
	var err error
	pacificLoc, err = time.LoadLocation("America/Los_Angeles")
	if err != nil {
		// Fallback to fixed PST offset if timezone data unavailable
		pacificLoc = time.FixedZone("PST", -8*60*60)
	}
}

// utcToPacific converts a UTC time to Pacific time
func utcToPacific(t time.Time) time.Time {
	return t.In(pacificLoc)
}

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
	DateRef       string // Reference date (Pacific) for timezone conversion
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

// GetDatesWithErrors returns dates (in Pacific time) that have errors for a specific hutch, sorted descending
func GetDatesWithErrors(db *sql.DB, hutch string) ([]DateSummary, error) {
	// Fetch individual file records to convert timestamps to Pacific time
	query := `
		SELECT lf.id, lf.start_timestamp_utc, lf.error_count
		FROM log_files lf
		WHERE hutch = ? AND error_count > 0
		ORDER BY start_timestamp_utc DESC
	`
	rows, err := db.Query(query, hutch)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	// Group by Pacific date in Go for proper DST handling
	type dateAgg struct {
		fileIDs    map[int]bool
		errorCount int
	}
	dateMap := make(map[string]*dateAgg)

	for rows.Next() {
		var fileID int
		var timestampUTC string
		var errorCount int
		if err := rows.Scan(&fileID, &timestampUTC, &errorCount); err != nil {
			return nil, err
		}

		// Convert UTC timestamp to Pacific date
		pacificDate := utcTimestampToPacificDate(timestampUTC)
		if pacificDate == "" {
			continue
		}

		if agg, ok := dateMap[pacificDate]; ok {
			if !agg.fileIDs[fileID] {
				agg.fileIDs[fileID] = true
			}
			agg.errorCount += errorCount
		} else {
			dateMap[pacificDate] = &dateAgg{
				fileIDs:    map[int]bool{fileID: true},
				errorCount: errorCount,
			}
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// Convert map to slice and sort
	var dates []DateSummary
	for date, agg := range dateMap {
		dates = append(dates, DateSummary{
			Date:       date,
			FileCount:  len(agg.fileIDs),
			ErrorCount: agg.errorCount,
		})
	}

	// Sort by date descending and limit to 60
	sort.Slice(dates, func(i, j int) bool {
		return dates[i].Date > dates[j].Date
	})
	if len(dates) > 60 {
		dates = dates[:60]
	}

	return dates, nil
}

// utcTimestampToPacificDate converts a UTC timestamp string to a Pacific date string (YYYY-MM-DD)
func utcTimestampToPacificDate(timestamp string) string {
	if timestamp == "" {
		return ""
	}

	// Try common timestamp formats
	for _, layout := range []string{
		"2006-01-02 15:04:05",
		"2006-01-02T15:04:05",
		"2006-01-02T15:04:05Z",
	} {
		if t, err := time.Parse(layout, timestamp); err == nil {
			t = t.UTC()
			pacific := utcToPacific(t)
			return pacific.Format("2006-01-02")
		}
	}

	// If just a date, convert as if midnight UTC
	if len(timestamp) == 10 {
		if t, err := time.Parse("2006-01-02", timestamp); err == nil {
			t = t.UTC()
			pacific := utcToPacific(t)
			return pacific.Format("2006-01-02")
		}
	}

	return ""
}

// LoadErrors loads errors for a specific hutch and Pacific date, ordered by timestamp
func LoadErrors(db *sql.DB, hutch, pacificDate string) ([]Error, error) {
	// Calculate UTC time range for the Pacific date
	utcStart, utcEnd, err := pacificDateToUTCRange(pacificDate)
	if err != nil {
		return nil, fmt.Errorf("invalid date format: %w", err)
	}

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
		       COALESCE(le.context_after, '') as ctx_after,
		       lf.start_timestamp_utc
		FROM log_errors le
		JOIN log_files lf ON le.log_file_id = lf.id
		WHERE lf.hutch = ?
		  AND lf.start_timestamp_utc >= ?
		  AND lf.start_timestamp_utc < ?
		  AND NOT (le.error_type = 'slurm' AND le.message LIKE '%CANCELLED%')
		  AND NOT (le.error_type = 'slurm' AND le.message LIKE '%Job step aborted%')
	`
	rows, err := db.Query(query, hutch, utcStart, utcEnd)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var errors []Error
	for rows.Next() {
		var e Error
		var fileTimestamp string
		if err := rows.Scan(
			&e.ID, &e.Timestamp, &e.Component, &e.Host,
			&e.LogLevel, &e.ErrorType, &e.Message, &e.LineNumber,
			&e.FilePath, &e.ContextBefore, &e.ContextAfter, &fileTimestamp,
		); err != nil {
			return nil, err
		}

		// Set DateRef for timezone conversion (use the Pacific date we're querying)
		e.DateRef = pacificDate

		// Extract time from filepath if timestamp is empty
		if e.Timestamp == "" {
			e.Timestamp = extractTimeFromPath(e.FilePath)
		}

		// Verify this error actually falls on the target Pacific date
		// (handles edge cases near midnight)
		errPacificDate := utcTimestampToPacificDate(fileTimestamp)
		if errPacificDate == pacificDate {
			errors = append(errors, e)
		}
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	// Sort by time (chronologically) in Pacific time
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

// pacificDateToUTCRange returns the UTC time range for a Pacific date
// Returns start (inclusive) and end (exclusive) timestamps
func pacificDateToUTCRange(pacificDate string) (string, string, error) {
	// Parse the date in Pacific timezone
	dateParsed, err := time.ParseInLocation("2006-01-02", pacificDate, pacificLoc)
	if err != nil {
		return "", "", err
	}

	// Start of day in Pacific (midnight)
	startPacific := dateParsed

	// End of day in Pacific (next day midnight)
	endPacific := startPacific.AddDate(0, 0, 1)

	// Convert to UTC
	startUTC := startPacific.UTC().Format("2006-01-02 15:04:05")
	endUTC := endPacific.UTC().Format("2006-01-02 15:04:05")

	return startUTC, endUTC, nil
}

// getErrorSortTime extracts a sortable time string from error in Pacific time
// Returns "HH:MM:SS" format for proper string sorting
func getErrorSortTime(e Error) string {
	// Try parsing full datetime first (most accurate for timezone conversion)
	if e.Timestamp != "" {
		for _, layout := range []string{
			"2006-01-02 15:04:05",
			"2006-01-02T15:04:05",
		} {
			if t, err := time.Parse(layout, e.Timestamp); err == nil {
				t = t.UTC()
				pacific := utcToPacific(t)
				return pacific.Format("15:04:05")
			}
		}
	}

	// Try filepath (DD_HH:MM:SS_host:comp.log) with conversion
	timeStr := extractTimeFromPath(e.FilePath)
	if timeStr != "" {
		converted := convertTimeWithDateHHMMSS(timeStr, e.DateRef)
		if converted != "" {
			return converted
		}
		return timeStr // Fallback to original if conversion fails
	}

	// Fall back to timestamp time portion with conversion
	if e.Timestamp != "" && len(e.Timestamp) >= 8 && e.Timestamp[2] == ':' && e.Timestamp[5] == ':' {
		converted := convertTimeWithDateHHMMSS(e.Timestamp[:8], e.DateRef)
		if converted != "" {
			return converted
		}
	}

	return "99:99:99" // Sort unknown times to end
}

// convertTimeWithDateHHMMSS converts a time string (HH:MM:SS) to Pacific time, returning HH:MM:SS
func convertTimeWithDateHHMMSS(timeStr, dateRef string) string {
	if len(timeStr) < 8 {
		return ""
	}

	// Use reference date if provided, otherwise use current date
	if dateRef == "" {
		dateRef = time.Now().UTC().Format("2006-01-02")
	}

	// Parse as UTC datetime
	fullDateTime := dateRef + " " + timeStr
	t, err := time.Parse("2006-01-02 15:04:05", fullDateTime)
	if err != nil {
		return ""
	}

	t = t.UTC()
	pacific := utcToPacific(t)
	return pacific.Format("15:04:05")
}

// FindNearestErrorIndex finds the error closest to targetTime (HH:MM format in Pacific time)
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
		errTime := extractTimeHHMM(e.Timestamp, e.FilePath, e.DateRef)
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

// extractTimeHHMM gets HH:MM from timestamp or filepath, converting UTC to Pacific
// dateRef is a reference date (YYYY-MM-DD) used when timestamp doesn't contain a full date
func extractTimeHHMM(timestamp, filepath, dateRef string) string {
	if timestamp != "" {
		// Try full datetime format first (has date for proper DST handling)
		if t, err := time.Parse("2006-01-02 15:04:05", timestamp); err == nil {
			t = t.UTC() // Ensure it's treated as UTC
			pacific := utcToPacific(t)
			return pacific.Format("15:04")
		}

		// Try time-only formats - need to combine with reference date
		for _, layout := range []string{
			"15:04:05",
			"15:04",
		} {
			if t, err := time.Parse(layout, timestamp); err == nil {
				return convertTimeWithDate(t.Format("15:04:05"), dateRef)
			}
		}

		// Just take first 5 chars if looks like HH:MM and convert
		if len(timestamp) >= 5 && timestamp[2] == ':' {
			timeStr := timestamp
			if len(timeStr) < 8 {
				timeStr = timestamp[:5] + ":00"
			}
			return convertTimeWithDate(timeStr[:8], dateRef)
		}
	}

	// Fallback to filepath
	timeStr := extractTimeFromPath(filepath)
	if timeStr != "" {
		return convertTimeWithDate(timeStr, dateRef)
	}
	return ""
}

// convertTimeWithDate converts a time string (HH:MM:SS) to Pacific time using a reference date
func convertTimeWithDate(timeStr, dateRef string) string {
	if len(timeStr) < 5 {
		return ""
	}

	// Use reference date if provided, otherwise use current date
	if dateRef == "" {
		dateRef = time.Now().UTC().Format("2006-01-02")
	}

	// Ensure we have HH:MM:SS format
	if len(timeStr) == 5 {
		timeStr = timeStr + ":00"
	}

	// Parse as UTC datetime
	fullDateTime := dateRef + " " + timeStr
	t, err := time.Parse("2006-01-02 15:04:05", fullDateTime)
	if err != nil {
		// Fallback: return first 5 chars without conversion
		return timeStr[:5]
	}

	t = t.UTC()
	pacific := utcToPacific(t)
	return pacific.Format("15:04")
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
