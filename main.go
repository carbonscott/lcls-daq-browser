package main

import (
	"database/sql"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	tea "github.com/charmbracelet/bubbletea"
	_ "github.com/mattn/go-sqlite3"
)

func main() {
	// Parse command line flags
	dbPath := flag.String("db", "", "Path to daq_logs.db")
	hutch := flag.String("hutch", "", "Hutch to browse (tmo, mfx, etc.)")
	date := flag.String("date", "", "Date to browse (YYYY-MM-DD)")
	time := flag.String("time", "", "Time to jump to (HH:MM)")
	mouse := flag.Bool("mouse", false, "Enable mouse support")
	flag.Parse()

	// Find database
	if *dbPath == "" {
		// First check DAQ_LOG_DIR environment variable
		if envPath := os.Getenv("DAQ_LOG_DIR"); envPath != "" {
			*dbPath = envPath
		} else {
			// Try common locations
			candidates := []string{
				"daq_logs.db",
				"../daq_logs.db",
				filepath.Join(os.Getenv("HOME"), "proj-debug-daq/daq_logs.db"),
			}
			for _, c := range candidates {
				if _, err := os.Stat(c); err == nil {
					*dbPath = c
					break
				}
			}
		}
	}

	if *dbPath == "" {
		fmt.Fprintln(os.Stderr, "Error: Could not find daq_logs.db")
		fmt.Fprintln(os.Stderr, "Usage: daq-browser --db path/to/daq_logs.db [--hutch HUTCH] [--date YYYY-MM-DD] [--time HH:MM] [--mouse]")
		os.Exit(1)
	}

	// Open database in immutable mode (read-only, no locking)
	db, err := sql.Open("sqlite3", "file:"+*dbPath+"?immutable=1")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error opening database: %v\n", err)
		os.Exit(1)
	}
	defer db.Close()

	// Verify database connection
	if err := db.Ping(); err != nil {
		fmt.Fprintf(os.Stderr, "Error connecting to database: %v\n", err)
		os.Exit(1)
	}

	// Create model
	m := NewModel(db, *hutch, *date, *time)

	// Run Bubbletea program
	opts := []tea.ProgramOption{tea.WithAltScreen()}
	if *mouse {
		opts = append(opts, tea.WithMouseCellMotion())
	}
	p := tea.NewProgram(m, opts...)
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error running program: %v\n", err)
		os.Exit(1)
	}
}
