package server

import (
	"io"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/schneik80/fusionlocalserver/api"
	"github.com/schneik80/fusionlocalserver/config"
)

// logFileName is the server log written under config.Dir().
const logFileName = "server.log"

// setupLogging builds the process logger. Output always goes to the console
// (stdout) and, when the config dir is writable, is mirrored to
// ~/.config/fusionlocalserver/server.log. Default level is info — essential
// lines only (startup, warnings, errors, auth events). Verbose (-v) raises the
// level to debug, which adds the per-request line (see middleware.logRequest)
// and routes the api package's GraphQL request/response tracing to the same
// sinks (signed URLs redacted at the source).
//
// The returned closer closes the log file and is always safe to call.
func setupLogging(verbose bool) (*slog.Logger, func()) {
	level := slog.LevelInfo
	if verbose {
		level = slog.LevelDebug
	}

	sinks := []io.Writer{os.Stdout}
	closer := func() {}
	f, path, ferr := openLogFile()
	if ferr == nil {
		sinks = append(sinks, f)
		closer = func() { _ = f.Close() }
	}
	out := io.MultiWriter(sinks...)

	logger := slog.New(slog.NewTextHandler(out, &slog.HandlerOptions{Level: level}))
	if ferr != nil {
		logger.Warn("could not open log file; logging to console only", "err", ferr)
	} else {
		logger.Info("logging", "file", path, "verbose", verbose)
	}

	if verbose {
		api.EnableDebug(out)
	}
	return logger, closer
}

// openLogFile opens (creating/appending) the server log under config.Dir().
func openLogFile() (*os.File, string, error) {
	dir, err := config.Dir()
	if err != nil {
		return nil, "", err
	}
	path := filepath.Join(dir, logFileName)
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return nil, "", err
	}
	return f, path, nil
}
