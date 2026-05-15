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

// ClassifyAssembly reports whether the design rooted at the given
// component-version id has at least one direct sub-component. The query
// asks the occurrences relationship for a single result; an empty
// response means the design is a part, any result means an assembly.
//
// The call blocks on a package-level semaphore so concurrent fan-out
// from a single contents-loaded message stays bounded. Callers should
// pair the returned bool with the originating item id and a generation
// counter so late-arriving refinements after a folder change can be
// dropped on the floor.
func ClassifyAssembly(ctx context.Context, token, componentVersionID string) (bool, error) {
	if componentVersionID == "" {
		return false, fmt.Errorf("classify: empty componentVersionID")
	}
	select {
	case classifySem <- struct{}{}:
	case <-ctx.Done():
		return false, ctx.Err()
	}
	defer func() { <-classifySem }()

	const q = `
		query ClassifyAssembly($cv: ID!) {
			componentVersion(componentVersionId: $cv) {
				occurrences(pagination: { limit: 1 }) {
					results { id }
				}
			}
		}`
	data, err := gqlQuery(ctx, token, q, map[string]any{"cv": componentVersionID})
	if err != nil {
		return false, err
	}
	var r struct {
		ComponentVersion struct {
			Occurrences struct {
				Results []struct {
					ID string `json:"id"`
				} `json:"results"`
			} `json:"occurrences"`
		} `json:"componentVersion"`
	}
	if err := json.Unmarshal(data, &r); err != nil {
		return false, fmt.Errorf("classify decode: %w", err)
	}
	return len(r.ComponentVersion.Occurrences.Results) > 0, nil
}
