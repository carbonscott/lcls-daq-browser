# lcls-daq-browser

A terminal user interface (TUI) for browsing and exploring DAQ (Data Acquisition) log errors from LCLS beamlines. Built with Go and the [Bubbletea](https://github.com/charmbracelet/bubbletea) framework.

## Overview

LCLS DAQ systems generate massive log files (4.5GB+ for a couple months). This tool provides fast, interactive access to indexed errors stored in a SQLite database, without needing to grep through raw text files.

**Features:**
- Three-panel layout: error groups, individual errors, and context view
- Vim-style keyboard navigation
- Filter by log level (Critical/Error), component, or message content
- Jump to specific times within a day
- Mouse support (optional)

## Prerequisites

- Go 1.24 or later
- A `daq_logs.db` SQLite database (see [Database Schema](#database-schema))

### Installing Go on SLAC SDF

Go is not available via the module system on SDF. Install it manually:

```bash
# Download and extract Go (check https://go.dev/dl/ for latest version)
cd ~
wget https://go.dev/dl/go1.24.10.linux-amd64.tar.gz
tar -xzf go1.24.10.linux-amd64.tar.gz

# Add to PATH in ~/.bashrc
echo 'export PATH=$HOME/go/bin:$PATH' >> ~/.bashrc
source ~/.bashrc

# Verify installation
go version
```

## Installation

```bash
go install github.com/carbonscott/lcls-daq-browser@latest
```

Or build from source:

```bash
git clone https://github.com/carbonscott/lcls-daq-browser.git
cd lcls-daq-browser
go build -o lcls-daq-browser
```

## Usage

```bash
# Auto-discover database in common locations
lcls-daq-browser

# Specify database path
lcls-daq-browser --db path/to/daq_logs.db

# Jump directly to a hutch and date
lcls-daq-browser --db daq_logs.db --hutch tmo --date 2025-11-19

# Jump to specific time within the day
lcls-daq-browser --db daq_logs.db --hutch tmo --date 2025-11-19 --time 19:30

# Enable mouse support
lcls-daq-browser --db daq_logs.db --mouse
```

### Command-Line Options

| Flag | Description |
|------|-------------|
| `--db PATH` | Path to daq_logs.db (auto-discovered if omitted) |
| `--hutch NAME` | Start at specific hutch (tmo, mfx, cxi, rix, xcs, xpp) |
| `--date YYYY-MM-DD` | Jump to specific date |
| `--time HH:MM` | Jump to nearest error at this time |
| `--mouse` | Enable mouse support |

## Keyboard Shortcuts

### Navigation

| Key | Action |
|-----|--------|
| `j` / `↓` | Move down |
| `k` / `↑` | Move up |
| `h` / `←` / `PgUp` | Page up |
| `l` / `→` / `PgDn` | Page down |
| `g` / `Home` | Jump to first item |
| `G` / `End` | Jump to last item |
| `Tab` | Next panel |
| `Shift+Tab` | Previous panel |
| `Enter` | Select item |
| `Esc` | Go back |

### Filtering

| Key | Action |
|-----|--------|
| `c` | Toggle critical-only filter |
| `/` | Filter by component name |
| `a` | Clear all filters (show all) |
| `t` | Jump to specific time (HH:MM) |

### General

| Key | Action |
|-----|--------|
| `?` | Toggle help |
| `q` / `Ctrl+C` | Quit |

## UI Layout

The error list view has three panels:

```
┌─────────────────┬─────────────────┬─────────────────┐
│   Error Groups  │  Errors in Grp  │  Error Context  │
│                 │                 │                 │
│ 07:50 teb0 (3)  │ > Error 1       │ [10 lines       │
│ 07:51 drp1 (5)  │   Error 2       │  before/after   │
│ > 08:00 meb (2) │   Error 3       │  the selected   │
│                 │                 │  error]         │
└─────────────────┴─────────────────┴─────────────────┘
```

- **Left panel:** Error groups by (time, component)
- **Middle panel:** Individual errors in selected group
- **Right panel:** Full context (10 lines before/after) for selected error

## Database Schema

The tool expects a SQLite database with the following tables:

### `log_files`

| Column | Type | Description |
|--------|------|-------------|
| id | INTEGER | Primary key |
| filename | TEXT | Log filename |
| hutch | TEXT | Beamline (tmo, mfx, etc.) |
| log_date | TEXT | Date (YYYY-MM-DD) |
| host | TEXT | Host machine |
| component | TEXT | DAQ component name |
| ... | | Additional metadata |

### `log_errors`

| Column | Type | Description |
|--------|------|-------------|
| id | INTEGER | Primary key |
| log_file_id | INTEGER | Foreign key to log_files |
| line_number | INTEGER | Line number in original file |
| timestamp | TEXT | Error timestamp |
| log_level | TEXT | 'E' (Error) or 'C' (Critical) |
| error_type | TEXT | Error category |
| message | TEXT | Error message |
| context_before | TEXT | 10 lines before error |
| context_after | TEXT | 10 lines after error |
