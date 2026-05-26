package server

import (
	"embed"
	"io/fs"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"path"
	"strings"
)

// webdistFS holds the built React/MUI SPA. The `all:` prefix embeds files whose
// names start with `_` or `.` (Vite emits hashed asset chunks under assets/ and
// the bundler may produce such names). A committed webdist/index.html
// placeholder ensures a fresh checkout compiles before Vite has ever run.
//
//go:embed all:webdist
var webdistFS embed.FS

// defaultViteDevServer is the origin the dev reverse-proxy targets when
// VITE_DEV_SERVER is unset.
const defaultViteDevServer = "http://localhost:5173"

// staticHandler returns the handler that serves the SPA for all non-/api
// routes. In production it serves the embedded build with SPA fallback; with
// -dev it reverse-proxies to the Vite dev server for hot module reload.
func (s *Server) staticHandler() http.Handler {
	if s.opts.Dev {
		return s.devProxyHandler()
	}
	return s.embeddedHandler()
}

// embeddedHandler serves files from the embedded webdist FS. Requests that
// don't map to a real file fall back to index.html so client-side routes
// (deep links) resolve to the SPA shell. /api routes never reach here — they
// match more specific patterns on the mux.
func (s *Server) embeddedHandler() http.Handler {
	dist, err := fs.Sub(webdistFS, "webdist")
	if err != nil {
		s.logger.Error("static: cannot open embedded webdist", "err", err)
		return http.NotFoundHandler()
	}
	fileServer := http.FileServer(http.FS(dist))

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		name := path.Clean(strings.TrimPrefix(r.URL.Path, "/"))
		if name == "." || name == "/" {
			name = "index.html"
		}
		// Serve the real file when it exists.
		if f, err := dist.Open(name); err == nil {
			_ = f.Close()
			fileServer.ServeHTTP(w, r)
			return
		}
		// SPA fallback: hand back index.html for unknown client routes.
		data, err := fs.ReadFile(dist, "index.html")
		if err != nil {
			http.Error(w, "web UI not built (run `make web`)", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write(data)
	})
}

// devProxyHandler reverse-proxies non-/api requests to the Vite dev server so
// the React app is served (with HMR) through the same origin as /api. Target
// is VITE_DEV_SERVER or http://localhost:5173. httputil's proxy forwards the
// websocket Upgrade Vite's HMR client needs.
func (s *Server) devProxyHandler() http.Handler {
	target := os.Getenv("VITE_DEV_SERVER")
	if target == "" {
		target = defaultViteDevServer
	}
	u, err := url.Parse(target)
	if err != nil {
		s.logger.Error("static: invalid VITE_DEV_SERVER, falling back to embedded", "target", target, "err", err)
		return s.embeddedHandler()
	}
	s.logger.Info("static: dev mode — proxying UI to Vite", "target", target)
	return httputil.NewSingleHostReverseProxy(u)
}
