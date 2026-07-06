package server

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"
)

func TestIsHandshakeNoise(t *testing.T) {
	cases := []struct {
		msg  string
		want bool
	}{
		// The port probe / browser preconnect flood — must be demoted.
		{"http: TLS handshake error from 127.0.0.1:43778: EOF", true},
		{"http: TLS handshake error from 192.168.1.20:52868: read tcp 192.168.1.5:8080->192.168.1.20:52868: read: connection reset by peer", true},
		{"http: TLS handshake error from 10.0.0.7:1234: read tcp 10.0.0.1:8080->10.0.0.7:1234: i/o timeout", true},
		// Real TLS problems — must stay loud.
		{"http: TLS handshake error from 127.0.0.1:5555: remote error: tls: bad certificate", false},
		{"http: TLS handshake error from 127.0.0.1:5555: tls: client offered only unsupported versions", false},
		// Non-handshake server errors — must stay loud.
		{"http: panic serving 127.0.0.1:9999: boom", false},
		{"http: response.WriteHeader on hijacked connection", false},
	}
	for _, c := range cases {
		if got := isHandshakeNoise(c.msg); got != c.want {
			t.Errorf("isHandshakeNoise(%q) = %v, want %v", c.msg, got, c.want)
		}
	}
}

func TestHTTPErrorLog_RoutesByNoise(t *testing.T) {
	// Info-level sink: demoted (debug) lines must NOT appear, error lines must.
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo}))
	lg := httpErrorLog(logger)

	lg.Printf("http: TLS handshake error from 127.0.0.1:43778: EOF")
	if got := buf.String(); got != "" {
		t.Fatalf("handshake-EOF noise reached the info sink: %q", got)
	}
	lg.Printf("http: panic serving 127.0.0.1:9999: boom")
	if got := buf.String(); !strings.Contains(got, "level=ERROR") || !strings.Contains(got, "panic serving") {
		t.Fatalf("real error not logged at error level: %q", got)
	}

	// Verbose (-v) sink still records the demoted lines, at debug.
	buf.Reset()
	verbose := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
	httpErrorLog(verbose).Printf("http: TLS handshake error from 127.0.0.1:43778: EOF")
	if got := buf.String(); !strings.Contains(got, "level=DEBUG") || !strings.Contains(got, "TLS handshake error") {
		t.Fatalf("demoted line missing from the debug sink: %q", got)
	}
}
