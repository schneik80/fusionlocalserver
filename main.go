package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"runtime/debug"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/schneik80/FusionDataCLI/config"
	"github.com/schneik80/FusionDataCLI/server"
	"github.com/schneik80/FusionDataCLI/ui"
)

// version is set at build time via -ldflags "-X main.version=x.y.z".
var version = "dev"

func main() {
	var (
		serverMode = flag.Bool("server", false, "run an HTTP server (web UI + JSON API) instead of the TUI")
		addr       = flag.String("addr", "0.0.0.0:8080", "address to bind the server to (with -server)")
		dev        = flag.Bool("dev", false, "dev mode: proxy the web UI to the Vite dev server for HMR (with -server)")
	)
	flag.Parse()

	cfg, cfgErr := config.Load()

	if *serverMode {
		if err := server.Run(server.Options{
			Addr:    *addr,
			Dev:     *dev,
			Config:  cfg,
			CfgErr:  cfgErr,
			Version: version,
		}); err != nil {
			fmt.Fprintf(os.Stderr, "server error: %v\n", err)
			os.Exit(1)
		}
		return
	}

	runTUI(cfg, cfgErr)
}

// runTUI launches the Bubble Tea program. It owns the panic-log wrapper: a
// panic out of the event loop restores the alternate screen but the dump
// scrolls past in the terminal, so we capture the cause + full stack to
// <config.Dir()>/panic.log and re-panic so the process still exits non-zero.
// The server path does not need this — it logs via slog and recovers panics
// per-request in middleware.
func runTUI(cfg *config.Config, cfgErr error) {
	defer func() {
		if r := recover(); r != nil {
			writePanicLog(r)
			panic(r)
		}
	}()

	p := tea.NewProgram(
		ui.New(cfg, cfgErr, version),
		tea.WithAltScreen(),
		tea.WithMouseCellMotion(),
	)
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func writePanicLog(r any) {
	dir, err := config.Dir()
	if err != nil {
		return
	}
	path := filepath.Join(dir, "panic.log")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return
	}
	defer f.Close()
	fmt.Fprintf(f, "=== panic at %s — version=%s ===\n", time.Now().Format(time.RFC3339), version)
	fmt.Fprintf(f, "%v\n\n%s\n", r, debug.Stack())
}
