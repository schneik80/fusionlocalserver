package api

import (
	"fmt"
	"io"
	"regexp"
	"sync"
	"time"
)

// Request/response tracing. The server enables this under -v and points it at
// the same console+file sink its slog logger uses; when disabled it is a
// no-op with no allocation. Tracing is intentionally raw (multi-line GraphQL
// bodies) rather than structured, so it is written straight to the sink rather
// than through slog.

var (
	dbgMu      sync.Mutex
	dbgEnabled bool
	dbgSink    io.Writer
)

// signedURLRe matches a "signedUrl":"…" pair in a JSON body. A signed URL is a
// bearer credential for the derivative it points at, so its value is redacted
// before any trace reaches the console or log file.
var signedURLRe = regexp.MustCompile(`(?i)("signedurl"\s*:\s*")[^"]*(")`)

// EnableDebug routes request/response tracing to w. A non-nil writer turns
// tracing on; nil turns it off. Called once at startup.
func EnableDebug(w io.Writer) {
	dbgMu.Lock()
	defer dbgMu.Unlock()
	dbgSink = w
	dbgEnabled = w != nil
}

// DebugEnabled reports whether request/response tracing is active.
func DebugEnabled() bool {
	dbgMu.Lock()
	defer dbgMu.Unlock()
	return dbgEnabled
}

func dbgLog(format string, args ...any) {
	dbgMu.Lock()
	defer dbgMu.Unlock()
	if !dbgEnabled || dbgSink == nil {
		return
	}
	msg := redactSignedURLs(fmt.Sprintf(format, args...))
	fmt.Fprintf(dbgSink, "[%s] %s\n", time.Now().Format("15:04:05"), msg)
}

// redactSignedURLs replaces signed-URL values with a placeholder so the
// credentials they carry never land in a log.
func redactSignedURLs(s string) string {
	return signedURLRe.ReplaceAllString(s, `${1}[redacted]${2}`)
}
