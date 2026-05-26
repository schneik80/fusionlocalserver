package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/schneik80/fusionlocalserver/config"
	"github.com/schneik80/fusionlocalserver/server"
)

// version is set at build time via -ldflags "-X main.version=x.y.z".
var version = "dev"

func main() {
	var (
		verbose = flag.Bool("v", false, "verbose logging (debug level, to console and the log file)")
		dev     = flag.Bool("dev", false, "developer mode: proxy the web UI to the Vite dev server for HMR")
	)
	flag.Parse()

	cfg, cfgErr := config.Load()

	if err := server.Run(server.Options{
		Verbose: *verbose,
		Dev:     *dev,
		Config:  cfg,
		CfgErr:  cfgErr,
		Version: version,
	}); err != nil {
		fmt.Fprintf(os.Stderr, "server error: %v\n", err)
		os.Exit(1)
	}
}
