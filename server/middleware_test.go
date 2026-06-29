package server

import (
	"crypto/tls"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// okHandler is a trivial downstream handler the middleware wraps.
func okHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
}

func TestSecurityHeadersProd(t *testing.T) {
	s := &Server{logger: quietLogger()} // opts.Dev == false
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "http://example.test/", nil)

	s.securityHeaders(okHandler()).ServeHTTP(rec, req)

	want := map[string]string{
		"X-Content-Type-Options": "nosniff",
		"X-Frame-Options":        "DENY",
		"Referrer-Policy":        "no-referrer",
	}
	for k, v := range want {
		if got := rec.Header().Get(k); got != v {
			t.Errorf("%s = %q, want %q", k, got, v)
		}
	}

	csp := rec.Header().Get("Content-Security-Policy")
	if csp == "" {
		t.Fatal("Content-Security-Policy header missing")
	}
	// Spot-check the directives that carry security weight; a regression that
	// loosens any of these should fail here.
	for _, dir := range []string{"default-src 'self'", "frame-ancestors 'none'", "base-uri 'self'"} {
		if !strings.Contains(csp, dir) {
			t.Errorf("CSP missing %q; got %q", dir, csp)
		}
	}

	// HSTS must NOT be set on a plain-HTTP request (LAN mode stays unpinned).
	if got := rec.Header().Get("Strict-Transport-Security"); got != "" {
		t.Errorf("Strict-Transport-Security set on non-TLS request: %q", got)
	}
}

func TestSecurityHeadersHSTSOverTLS(t *testing.T) {
	s := &Server{logger: quietLogger()}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "https://example.test/", nil)
	req.TLS = &tls.ConnectionState{} // mark the request as TLS-terminated

	s.securityHeaders(okHandler()).ServeHTTP(rec, req)

	if got := rec.Header().Get("Strict-Transport-Security"); got == "" {
		t.Error("Strict-Transport-Security missing on TLS request")
	}
}

func TestSecurityHeadersDevNoOp(t *testing.T) {
	s := &Server{logger: quietLogger(), opts: Options{Dev: true}}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "http://localhost:5173/", nil)

	s.securityHeaders(okHandler()).ServeHTTP(rec, req)

	// Dev is a no-op so Vite HMR (inline preamble + websocket) isn't broken.
	for _, k := range []string{
		"Content-Security-Policy",
		"X-Frame-Options",
		"X-Content-Type-Options",
		"Referrer-Policy",
	} {
		if got := rec.Header().Get(k); got != "" {
			t.Errorf("dev mode set %s = %q, want empty", k, got)
		}
	}
}
