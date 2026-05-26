package server

import (
	"io"
	"log/slog"
)

// quietLogger returns a logger that discards all output, for tests that
// construct a Server without exercising logging.
func quietLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}
