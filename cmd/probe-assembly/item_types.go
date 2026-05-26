//go:build itemtypes

package main

// introspect possible Item-typename values via the GraphQL schema:
//
//	go run -tags itemtypes -ldflags "..." ./cmd/probe-assembly
//
// Lists every type that implements the Item interface (or, failing that,
// every type whose name ends with "Item"). Lets us match Fusion drawings
// (.f2d/.f2t) vs. electronics (PCB / Schematic / ECAD project) to the
// real APS __typename values rather than guessing.

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/schneik80/fusionlocalserver/auth"
)

var _ = strings.HasSuffix // keep import even if early return uses it

func init() {
	td, err := auth.LoadTokens()
	if err != nil || td == nil {
		fmt.Println("itemtypes: no tokens; run main app first")
		os.Exit(1)
	}
	rootCtx := context.Background()
	ctx, cancel := context.WithTimeout(rootCtx, 20*time.Second)
	defer cancel()

	// 1) Try to find the Item interface and list its possibleTypes.
	q1 := `query { __type(name: "Item") { name kind possibleTypes { name } } }`
	var r1 struct {
		Type struct {
			Name          string `json:"name"`
			Kind          string `json:"kind"`
			PossibleTypes []struct {
				Name string `json:"name"`
			} `json:"possibleTypes"`
		} `json:"__type"`
	}
	_ = do(ctx, td.AccessToken, q1, nil, &r1)

	if r1.Type.Name == "Item" && len(r1.Type.PossibleTypes) > 0 {
		fmt.Printf("Item is %s with %d implementing types:\n", r1.Type.Kind, len(r1.Type.PossibleTypes))
		names := make([]string, 0, len(r1.Type.PossibleTypes))
		for _, t := range r1.Type.PossibleTypes {
			names = append(names, t.Name)
		}
		sort.Strings(names)
		for _, n := range names {
			fmt.Printf("  %s\n", n)
		}
		os.Exit(0)
	}

	// 2) Fallback: probe specific candidate type names directly. APS
	// rejects full __schema introspection in production, but per-type
	// __type lookups still work and are cheap.
	fmt.Println("Item interface lookup empty — probing candidate type names directly...")
	candidates := []string{
		"DesignItem",
		"DrawingItem",
		"DrawingTemplateItem",
		"ConfiguredDesignItem",
		"PCBItem",
		"SchematicItem",
		"ElectronicsItem",
		"ElectronicsDesignItem",
		"ElectronicsProjectItem",
		"PCBDesignItem",
		"FootprintItem",
		"SymbolItem",
		"LibraryItem",
		"BoardItem",
		"BasicItem",
	}
	fmt.Printf("%-32s  %s\n", "candidate", "kind / status")
	for _, name := range candidates {
		q := `query($n: String!) { __type(name: $n) { name kind interfaces { name } } }`
		var r struct {
			Type *struct {
				Name       string `json:"name"`
				Kind       string `json:"kind"`
				Interfaces []struct {
					Name string `json:"name"`
				} `json:"interfaces"`
			} `json:"__type"`
		}
		callCtx, callCancel := context.WithTimeout(rootCtx, 8*time.Second)
		err := do(callCtx, td.AccessToken, q, map[string]any{"n": name}, &r)
		callCancel()
		if err != nil {
			fmt.Printf("%-32s  ERROR: %v\n", name, err)
			continue
		}
		if r.Type == nil {
			fmt.Printf("%-32s  (not in schema)\n", name)
			continue
		}
		ifaces := make([]string, 0, len(r.Type.Interfaces))
		for _, i := range r.Type.Interfaces {
			ifaces = append(ifaces, i.Name)
		}
		fmt.Printf("%-32s  %s  [%s]\n", name, r.Type.Kind, strings.Join(ifaces, ","))
	}
	os.Exit(0)
}
