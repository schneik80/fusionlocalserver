package api

import (
	"context"
	"encoding/json"
)

// ProbeResult is one candidate query's outcome: either the raw data it
// returned or the error it produced (e.g. a "Cannot query field …" rejection).
type ProbeResult struct {
	Query string          `json:"query"`
	Data  json.RawMessage `json:"data,omitempty"`
	Error string          `json:"error,omitempty"`
}

// ProbeVersionMilestones runs candidate GraphQL selections against a real
// itemVersions list to show, live, how a version exposes its root component
// version (and thus the isMilestone flag the history graph needs).
//
// It originally found that path; it is RETAINED as a -v-only diagnostic so the
// same one-click probe can re-confirm the shape (or catch a schema change) on
// any document. The confirmed answer is the "designItemVersion_rootComponentVersion"
// candidate — that is what GetItemDetails now uses.
//
// APS production disables __schema introspection, but __typename is always
// allowed — so the "typename" probe reveals the version result's actual GraphQL
// type, and the others test specific field selections. Each variant is its own
// independent query, so an unknown field fails only that probe, never the
// others. This backs the -v debug endpoint; it is not on any production path.
func ProbeVersionMilestones(ctx context.Context, token, hubID, itemID string) map[string]ProbeResult {
	vars := map[string]any{"hubId": hubID, "itemId": itemID}

	// Compact single-line queries so the echoed Query reads cleanly in JSON.
	//
	// Verified live: itemVersions.results is typed `ItemVersion` (an interface,
	// like Item → DesignItem). There is NO bare `rootComponentVersion`/`isMilestone`
	// on it; the milestone path is on the concrete `DesignItemVersion` behind an
	// inline fragment — the "designItemVersion_rootComponentVersion" candidate,
	// which returns data while the others fail validation. The losing candidates
	// are kept as a regression check: if the schema shifts, the probe shows which
	// selection newly resolves.
	candidates := map[string]string{
		// Reveals each result's CONCRETE type name — always works (not __schema).
		"typename": `query($hubId:ID!,$itemId:ID!){itemVersions(hubId:$hubId,itemId:$itemId){results{versionNumber __typename}}}`,
		// Primary hypothesis: DesignItemVersion.rootComponentVersion.isMilestone.
		"designItemVersion_rootComponentVersion": `query($hubId:ID!,$itemId:ID!){itemVersions(hubId:$hubId,itemId:$itemId){results{versionNumber ... on DesignItemVersion{__typename rootComponentVersion{id isMilestone}}}}}`,
		// Fallbacks: isMilestone directly on the concrete subtype, or on ItemVersion.
		"designItemVersion_isMilestone": `query($hubId:ID!,$itemId:ID!){itemVersions(hubId:$hubId,itemId:$itemId){results{versionNumber ... on DesignItemVersion{__typename isMilestone}}}}`,
		"itemVersion_isMilestone":       `query($hubId:ID!,$itemId:ID!){itemVersions(hubId:$hubId,itemId:$itemId){results{versionNumber isMilestone}}}`,
	}

	out := make(map[string]ProbeResult, len(candidates))
	for name, q := range candidates {
		r := ProbeResult{Query: q}
		data, err := gqlQuery(ctx, token, q, vars)
		if err != nil {
			r.Error = err.Error()
		} else {
			r.Data = data
		}
		out[name] = r
	}
	return out
}
