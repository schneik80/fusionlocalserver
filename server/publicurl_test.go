package server

import (
	"crypto/tls"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

func TestCallbackURI(t *testing.T) {
	t.Run("derived from request when no public URL", func(t *testing.T) {
		s := newAuthTestServer()
		req := httptest.NewRequest(http.MethodGet, "http://10.0.4.34:8080/api/auth/login", nil)
		if got, want := s.callbackURI(req), "http://10.0.4.34:8080/api/auth/callback"; got != want {
			t.Errorf("callbackURI = %q, want %q", got, want)
		}
	})

	t.Run("derived uses https under TLS", func(t *testing.T) {
		s := newAuthTestServer()
		req := httptest.NewRequest(http.MethodGet, "https://10.0.4.34:8080/api/auth/login", nil)
		req.TLS = &tls.ConnectionState{}
		if got, want := s.callbackURI(req), "https://10.0.4.34:8080/api/auth/callback"; got != want {
			t.Errorf("callbackURI = %q, want %q", got, want)
		}
	})

	t.Run("fixed to public URL regardless of request host", func(t *testing.T) {
		s := newAuthTestServer()
		s.publicURL = "https://fusion.lan:8080"
		s.publicOrigin = "https://fusion.lan:8080"
		// Even though the client reached us via the LAN IP, the callback is the
		// canonical one — so only that single URL must be registered with APS.
		req := httptest.NewRequest(http.MethodGet, "https://10.0.4.34:8080/api/auth/login", nil)
		req.TLS = &tls.ConnectionState{}
		if got, want := s.callbackURI(req), "https://fusion.lan:8080/api/auth/callback"; got != want {
			t.Errorf("callbackURI = %q, want %q", got, want)
		}
	})
}

func TestCanonicalRedirect(t *testing.T) {
	s := newAuthTestServer()
	s.publicURL = "https://fusion.lan:8080"
	s.publicOrigin = "https://fusion.lan:8080"

	var reached bool
	h := s.canonicalRedirect(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		reached = true
	}))

	t.Run("other host is redirected to the canonical origin", func(t *testing.T) {
		reached = false
		req := httptest.NewRequest(http.MethodGet, "https://10.0.4.34:8080/projects?x=1", nil)
		req.TLS = &tls.ConnectionState{}
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if reached {
			t.Error("next handler ran; expected a redirect")
		}
		if rec.Code != http.StatusFound {
			t.Fatalf("status = %d, want 302", rec.Code)
		}
		loc := rec.Header().Get("Location")
		u, _ := url.Parse(loc)
		if u.Scheme != "https" || u.Host != "fusion.lan:8080" || u.Path != "/projects" || u.RawQuery != "x=1" {
			t.Errorf("redirected to %q, want https://fusion.lan:8080/projects?x=1", loc)
		}
	})

	t.Run("canonical host passes through", func(t *testing.T) {
		reached = false
		req := httptest.NewRequest(http.MethodGet, "https://fusion.lan:8080/", nil)
		req.TLS = &tls.ConnectionState{}
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if !reached {
			t.Error("next handler did not run for a canonical-host request")
		}
		if rec.Code == http.StatusFound {
			t.Error("canonical-host request was redirected")
		}
	})
}

func TestCanonicalRedirect_DisabledIsPassthrough(t *testing.T) {
	s := newAuthTestServer() // no public URL set
	var reached bool
	h := s.canonicalRedirect(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		reached = true
	}))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "http://anything:1234/", nil))
	if !reached {
		t.Error("with no public URL, every request should pass through")
	}
}
