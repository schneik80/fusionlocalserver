// Package server runs FusionDataCLI as an HTTP service: a JSON REST API over
// the shared api/auth/config/pins packages, plus an embedded React/MUI SPA. It
// holds one APS identity (reused from the TUI's cached tokens) and proxies
// every data call through it. The package has no TUI dependencies.
package server

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/schneik80/FusionDataCLI/api"
	"github.com/schneik80/FusionDataCLI/config"
	"github.com/schneik80/FusionDataCLI/pins"
)

// Options configures a server run. Config may be nil when CfgErr is set (no
// usable APS configuration); Bootstrap then fails fast with a clear message.
type Options struct {
	Addr    string
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

	// pinsMu serialises the non-atomic Load->mutate->Save pin cycle.
	pinsMu sync.Mutex
}

// Run starts the HTTP server and blocks until it shuts down (SIGINT/SIGTERM)
// or fails to start. It performs the one-time interactive auth bootstrap
// before binding the listener, so any browser login happens up front.
func Run(opts Options) error {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))

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
		opts:   opts,
		logger: logger,
		tm:     NewTokenManager(clientID, clientSecret, logger),
		region: region,
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

	srv := &http.Server{
		Addr:              opts.Addr,
		Handler:           s.routes(),
		ReadHeaderTimeout: 10 * time.Second,
	}

	logger.Info("server starting",
		"addr", opts.Addr,
		"version", opts.Version,
		"region", regionLabel(region),
		"dev", opts.Dev,
	)
	warnOpenNetwork(logger, opts.Addr)

	// Trigger graceful shutdown when the lifecycle context is cancelled.
	shutdownDone := make(chan struct{})
	go func() {
		<-ctx.Done()
		logger.Info("shutdown signal received, draining connections")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			logger.Error("graceful shutdown failed", "err", err)
		}
		close(shutdownDone)
	}()

	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("http server: %w", err)
	}
	<-shutdownDone
	logger.Info("server stopped")
	return nil
}

// regionLabel renders the region for display/logging; empty maps to "US".
func regionLabel(region string) string {
	if region == "" {
		return "US"
	}
	return region
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
