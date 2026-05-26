package api

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/schneik80/fusionlocalserver/config"
)

const maxDebugLines = 500

// DebugLogFilename is the basename written under config.Dir() when
// debug logging is enabled. Each session truncates and starts fresh.
const DebugLogFilename = "debug.log"

var (
	dbgMu                sync.Mutex
	dbgEnabled           bool
	dbgLines             []string
	dbgFile              *os.File // log mirror; nil if open failed
	dbgFilePath          string   // path of dbgFile, exposed via DebugLogPath
	dbgStderrIsRedirected bool    // true when stderr is a file/pipe, not a TTY
)

// EnableDebug turns on request/response logging.
//
// Three sinks are wired up:
//   - In-memory ring buffer (always; what the in-app `?` overlay reads).
//   - File mirror at <config.Dir()>/debug.log, truncated each session, so
//     the user can copy/grep the log with standard tools while the TUI
//     is running.
//   - Stderr — but ONLY when stderr is redirected. Writing to a TTY
//     stderr would smear bubbletea's alternate-screen render.
func EnableDebug() {
	dbgMu.Lock()
	defer dbgMu.Unlock()
	dbgEnabled = true

	if fi, err := os.Stderr.Stat(); err == nil {
		dbgStderrIsRedirected = (fi.Mode() & os.ModeCharDevice) == 0
	}

	if dbgFile == nil {
		if dir, err := config.Dir(); err == nil {
			path := filepath.Join(dir, DebugLogFilename)
			if f, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0600); err == nil {
				dbgFile = f
				dbgFilePath = path
			}
		}
	}
}

// DebugLogPath returns the path of the debug log file, or "" if the
// file was never opened (debug disabled, or the config dir was
// unwritable). Surfaced in the in-app `?` overlay so the user knows
// where to look.
func DebugLogPath() string {
	dbgMu.Lock()
	defer dbgMu.Unlock()
	return dbgFilePath
}

// DebugEnabled reports whether debug logging is active.
func DebugEnabled() bool {
	dbgMu.Lock()
	defer dbgMu.Unlock()
	return dbgEnabled
}

// DebugLines returns a snapshot of the log.
func DebugLines() []string {
	dbgMu.Lock()
	defer dbgMu.Unlock()
	cp := make([]string, len(dbgLines))
	copy(cp, dbgLines)
	return cp
}

func dbgLog(format string, args ...any) {
	dbgMu.Lock()
	defer dbgMu.Unlock()
	if !dbgEnabled {
		return
	}
	line := fmt.Sprintf("[%s] ", time.Now().Format("15:04:05")) + fmt.Sprintf(format, args...)
	dbgLines = append(dbgLines, line)
	if len(dbgLines) > maxDebugLines {
		dbgLines = dbgLines[len(dbgLines)-maxDebugLines:]
	}
	if dbgFile != nil {
		fmt.Fprintln(dbgFile, line)
	}
	// Stderr mirror is only safe when stderr is redirected (file / pipe).
	// Writing to a TTY stderr would smear bubbletea's alternate-screen
	// render. The file mirror above covers the "I want to grep / copy"
	// use case without any of that risk.
	if dbgStderrIsRedirected {
		fmt.Fprintln(os.Stderr, line)
	}
}

// DebugLog appends a formatted line to the shared debug log. Intended for
// use by other packages (e.g. the ui layer) so that externally-initiated
// events like "browser opened with URL X" are captured alongside the
// request/response entries emitted by this package.
func DebugLog(format string, args ...any) {
	dbgLog(format, args...)
}
