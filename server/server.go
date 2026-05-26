// Package server runs fusionlocalserver as an HTTP service: a JSON REST API over
// the shared api/auth/config/pins packages, plus an embedded React/MUI SPA. It
// holds one APS identity (reused from the TUI's cached tokens) and proxies
// every data call through it. The package has no TUI dependencies.
package server

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"sync"
	"syscall"
	"time"

	"github.com/schneik80/fusionlocalserver/api"
	"github.com/schneik80/fusionlocalserver/config"
	"github.com/schneik80/fusionlocalserver/pins"
)

// Options configures a server run. Config may be nil when CfgErr is set (no
// usable APS configuration); Bootstrap then fails fast with a clear message.
type Options struct {
	// Verbose raises the log level to debug and adds per-request and upstream
	// API tracing, to both the console and the log file.
	Verbose bool
	// Dev proxies the web UI to the Vite dev server for HMR and pins the listen
	// port to the default (so the proxy target is stable); the runtime
	// port-change endpoint is disabled in this mode.
	Dev     bool
	Config  *config.Config
	CfgErr  error
	Version string
}

// Server holds the runtime state shared across handlers.
type Server struct {
	opts   Options
	logger *slog.Logger
	tm     *TokenManager
	region string // resolved APS region ("" == US)

	// portConfigurable gates the runtime port-change endpoint. The server owns
	// the port (and so derives the bind address from server.json) unless it is
	// in dev mode, where the Vite proxy is pinned to the default port.
	portConfigurable bool

	// restartCh signals the bind loop to drain the current listener and rebind
	// from the updated port setting. Buffered (cap 1) so the handler never
	// blocks; a pending restart coalesces.
	restartCh chan struct{}

	// addrMu guards addr, the currently bound address, read by handleMeta.
	addrMu sync.RWMutex
	addr   string

	// pinsMu serialises the non-atomic Load->mutate->Save pin cycle.
	pinsMu sync.Mutex

	// thumbs caches thumbnail status/URLs and image bytes by component version
	// id, shared across all clients. warmSem bounds background image prefetches
	// kicked off from the classify probe.
	thumbs  *thumbCache
	warmSem chan struct{}
}

// serveReason explains why the inner serve loop returned.
type serveReason int

const (
	reasonShutdown serveReason = iota // lifecycle context cancelled (SIGINT/TERM)
	reasonRestart                     // port changed; rebind the listener
)

// Run starts the HTTP server and blocks until it shuts down (SIGINT/SIGTERM)
// or fails to start. It performs the one-time interactive auth bootstrap
// before binding the listener, so any browser login happens up front.
func Run(opts Options) error {
	level := slog.LevelInfo
	if opts.Verbose {
		level = slog.LevelDebug
	}
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: level}))

	var clientID, clientSecret, region string
	if opts.Config != nil {
		clientID = opts.Config.ClientID
		clientSecret = opts.Config.ClientSecret
		region = opts.Config.Region
	} else if opts.CfgErr != nil {
		logger.Warn("config load failed", "err", opts.CfgErr)
	}

	// Region is process-global; set once before any API call.
	api.SetRegion(region)

	// Promote any legacy single-file pins into hub-scoped files (idempotent).
	if err := pins.MigrateLegacy(); err != nil {
		logger.Warn("pins: legacy migration failed", "err", err)
	}

	s := &Server{
		opts:             opts,
		logger:           logger,
		tm:               NewTokenManager(clientID, clientSecret, logger),
		region:           region,
		portConfigurable: !opts.Dev,
		restartCh:        make(chan struct{}, 1),
		thumbs:           newThumbCache(512, 10*time.Minute),
		warmSem:          make(chan struct{}, 12),
	}

	// Authenticate before serving. A generous window covers an interactive
	// browser login on first run.
	bootCtx, cancelBoot := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancelBoot()
	if err := s.tm.Bootstrap(bootCtx); err != nil {
		return fmt.Errorf("auth bootstrap: %w", err)
	}

	// Lifecycle context cancelled on first interrupt signal.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	s.tm.StartRefresher(ctx)

	// Listener (re)bind loop. Auth + the token refresher above are set up once
	// and span restarts; only the HTTP listener is recreated. A runtime port
	// change (POST /api/settings/port) writes server.json and signals
	// restartCh, dropping us out of serveUntil with reasonRestart so we rebind
	// on the new port.
	var prevAddr string // last successfully-served address; "" until first bind
	for {
		// If shutdown was requested (possibly while a restart was also pending,
		// where select could have picked the restart), bail before rebinding.
		select {
		case <-ctx.Done():
			logger.Info("server stopped")
			return nil
		default:
		}

		addr := s.resolveAddr()
		s.setAddr(addr)

		srv := &http.Server{
			Addr:              addr,
			Handler:           s.routes(),
			ReadHeaderTimeout: 10 * time.Second,
		}

		logger.Info("server starting",
			"addr", addr,
			"version", opts.Version,
			"region", regionLabel(region),
			"dev", opts.Dev,
			"portConfigurable", s.portConfigurable,
		)
		warnOpenNetwork(logger, addr)
		for _, u := range lanURLs(addr) {
			logger.Info("reachable on the LAN", "url", u)
		}

		reason, err := s.serveUntil(ctx, srv)
		if err != nil {
			if prevAddr == "" {
				return fmt.Errorf("http server: %w", err) // initial bind failed → fatal
			}
			// A runtime rebind failed — most likely the new port was taken in
			// the TOCTOU window after the handler's bind pre-check. Revert the
			// persisted port and keep serving on the previous one rather than
			// killing the server out from under the operator. The previous port
			// is free again (its listener was drained when the restart fired).
			logger.Error("rebind failed; reverting to previous port",
				"failed_addr", addr, "prev_addr", prevAddr, "err", err)
			if p, perr := portFromAddr(prevAddr); perr == nil {
				if serr := SaveSettings(Settings{Port: p}); serr != nil {
					logger.Error("reverting persisted port failed", "err", serr)
				}
			}
			continue
		}
		if reason == reasonShutdown {
			logger.Info("server stopped")
			return nil
		}
		prevAddr = addr
		logger.Info("port changed — restarting listener", "next_addr", s.resolveAddr())
	}
}

// serveUntil runs srv until the lifecycle context is cancelled (shutdown) or a
// restart is requested, draining connections gracefully in both cases. A
// ListenAndServe failure (e.g. the port is already in use) is returned as a
// terminal error.
func (s *Server) serveUntil(ctx context.Context, srv *http.Server) (serveReason, error) {
	errCh := make(chan error, 1)
	go func() {
		err := srv.ListenAndServe()
		if errors.Is(err, http.ErrServerClosed) {
			err = nil
		}
		errCh <- err
	}()

	select {
	case <-ctx.Done():
		s.drain(srv)
		return reasonShutdown, <-errCh
	case <-s.restartCh:
		s.drain(srv)
		return reasonRestart, <-errCh
	case err := <-errCh:
		// ListenAndServe returned on its own — a bind failure or other fatal
		// error (a clean shutdown would have come via the cases above).
		return reasonShutdown, err
	}
}

// drain gracefully shuts down srv, waiting up to 10s for in-flight requests
// (including the just-sent port-change response) to complete.
func (s *Server) drain(srv *http.Server) {
	s.logger.Info("draining connections")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		s.logger.Error("graceful shutdown failed", "err", err)
	}
}

// resolveAddr computes the bind address, always on the wildcard host. In dev
// mode the port is pinned to the default (the Vite proxy targets it); otherwise
// the server owns the port and derives it from the persisted setting
// (server.json), falling back to the default.
func (s *Server) resolveAddr() string {
	if !s.portConfigurable {
		return fmt.Sprintf("0.0.0.0:%d", defaultPort)
	}
	port := defaultPort
	if set, err := LoadSettings(); err != nil {
		s.logger.Warn("settings load failed; using default port", "err", err, "port", defaultPort)
	} else if set.Port != 0 {
		port = set.Port
	}
	return fmt.Sprintf("0.0.0.0:%d", port)
}

func (s *Server) setAddr(addr string) {
	s.addrMu.Lock()
	s.addr = addr
	s.addrMu.Unlock()
}

// currentPort returns the port of the currently bound address, or 0 if it
// can't be parsed.
func (s *Server) currentPort() int {
	s.addrMu.RLock()
	addr := s.addr
	s.addrMu.RUnlock()
	p, _ := portFromAddr(addr)
	return p
}

// portFromAddr extracts the numeric port from a host:port address.
func portFromAddr(addr string) (int, error) {
	_, p, err := net.SplitHostPort(addr)
	if err != nil {
		return 0, err
	}
	return strconv.Atoi(p)
}

// requestRestart asks the bind loop to rebind. Non-blocking: a pending restart
// coalesces rather than queuing.
func (s *Server) requestRestart() {
	select {
	case s.restartCh <- struct{}{}:
	default:
	}
}

// regionLabel renders the region for display/logging; empty maps to "US".
func regionLabel(region string) string {
	if region == "" {
		return "US"
	}
	return region
}

// lanURLs returns the browser URLs the server is reachable at. When bound to a
// wildcard host it enumerates the machine's non-loopback IPv4 interface
// addresses so an operator can copy a LAN URL straight from the startup log.
func lanURLs(addr string) []string {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return []string{"http://" + addr}
	}
	if host != "" && host != "0.0.0.0" && host != "::" {
		return []string{"http://" + net.JoinHostPort(host, port)}
	}

	urls := []string{"http://" + net.JoinHostPort("localhost", port)}
	ifaceAddrs, err := net.InterfaceAddrs()
	if err != nil {
		return urls
	}
	for _, a := range ifaceAddrs {
		var ip net.IP
		switch v := a.(type) {
		case *net.IPNet:
			ip = v.IP
		case *net.IPAddr:
			ip = v.IP
		}
		if ip == nil || ip.IsLoopback() {
			continue
		}
		if ip4 := ip.To4(); ip4 != nil {
			urls = append(urls, "http://"+net.JoinHostPort(ip4.String(), port))
		}
	}
	return urls
}

// warnOpenNetwork emits a visible warning when the server is bound to a
// non-loopback address, since there is no auth gate — anyone who can reach the
// address browses as the server's APS identity.
func warnOpenNetwork(logger *slog.Logger, addr string) {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		host = addr
	}
	loopback := host == "" || host == "localhost" || host == "127.0.0.1" || host == "::1"
	if ip := net.ParseIP(host); ip != nil && ip.IsLoopback() {
		loopback = true
	}
	if !loopback {
		logger.Warn("SERVER IS OPEN ON THE NETWORK WITH NO AUTH GATE — anyone who can reach this address browses as the server's APS identity",
			"addr", addr)
	}
}
