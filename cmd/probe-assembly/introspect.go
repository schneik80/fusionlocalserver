//go:build introspect

package main

// introspectComponentVersion is gated behind a build tag so the standard
// probe stays small. Run with:
//
//	go run -tags introspect -ldflags "..." ./cmd/probe-assembly
//
// Probes whether APS exposes any aggregate / boolean field on
// ComponentVersion that distinguishes assemblies from parts without
// having to fetch an actual sub-component page.

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/schneik80/fusionlocalserver/auth"
)

func init() {
	// Replace the default main behavior with introspection by os.Exiting.
	old := os.Args
	defer func() { os.Args = old }()

	td, err := auth.LoadTokens()
	if err != nil || td == nil {
		fmt.Println("introspect: no tokens; run main app first")
		os.Exit(1)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	q := `query { __type(name: "ComponentVersion") { name fields { name type { name kind ofType { name kind } } } } }`
	var resp struct {
		Type struct {
			Name   string `json:"name"`
			Fields []struct {
				Name string `json:"name"`
				Type struct {
					Name   string `json:"name"`
					Kind   string `json:"kind"`
					OfType struct {
						Name string `json:"name"`
						Kind string `json:"kind"`
					} `json:"ofType"`
				} `json:"type"`
			} `json:"fields"`
		} `json:"__type"`
	}
	if err := do(ctx, td.AccessToken, q, nil, &resp); err != nil {
		fmt.Printf("introspect: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("ComponentVersion fields:\n")
	for _, f := range resp.Type.Fields {
		t := f.Type.Name
		if t == "" {
			t = f.Type.Kind + " of " + f.Type.OfType.Name
		}
		fmt.Printf("  %s : %s\n", f.Name, t)
	}
	os.Exit(0)
}
