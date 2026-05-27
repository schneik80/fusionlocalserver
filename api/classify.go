package api

import (
	"context"
	"encoding/json"
	"fmt"
)

// classifySem caps concurrent in-flight assembly-classification calls so
// the gateway isn't hammered when the contents column lands on a folder
// with dozens of designs. 8 is empirically a good balance between
// wall-clock throughput and gateway-side cost — see cmd/probe-assembly
// for the latency-vs-parallelism data.
var classifySem = make(chan struct{}, 8)

// ClassifyAssembly reports whether the design rooted at the given v3 Component
// id is an assembly (has at least one direct sub-component), read from the
// component's hasChildren flag.
//
// The call blocks on a package-level semaphore so concurrent fan-out
// from a single contents-loaded message stays bounded. Callers should
// pair the returned bool with the originating item id and a generation
// counter so late-arriving refinements after a folder change can be
// dropped on the floor.
func ClassifyAssembly(ctx context.Context, token, componentID string) (bool, error) {
	if componentID == "" {
		return false, fmt.Errorf("classify: empty componentID")
	}
	select {
	case classifySem <- struct{}{}:
	case <-ctx.Done():
		return false, ctx.Err()
	}
	defer func() { <-classifySem }()

	const q = `
		query ClassifyAssembly($cv: ID!) {
			component(componentId: $cv) {
				hasChildren
			}
		}`
	data, err := gqlQuery(ctx, token, q, map[string]any{"cv": componentID})
	if err != nil {
		return false, err
	}
	var r struct {
		Component struct {
			HasChildren bool `json:"hasChildren"`
		} `json:"component"`
	}
	if err := json.Unmarshal(data, &r); err != nil {
		return false, fmt.Errorf("classify decode: %w", err)
	}
	return r.Component.HasChildren, nil
}
