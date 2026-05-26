// probe-assembly is a one-off diagnostic that sends an extended
// itemsByProject query asking for tipRootComponentVersion.occurrences on
// every DesignItem, so we can verify whether the APS Manufacturing Data
// Model GraphQL gateway accepts the expanded selection (cost wise) and
// what the response shape looks like.
//
// Usage:
//
//	go run ./cmd/probe-assembly                     # auto: first hub, first project
//	go run ./cmd/probe-assembly -hub <hubID>        # specific hub
//	go run ./cmd/probe-assembly -project <projID>   # specific project
//
// Authenticate by supplying a token via the environment (the server no longer
// caches tokens to disk):
//
//	APS_ACCESS_TOKEN   a bearer token to use directly, or
//	APS_REFRESH_TOKEN  refreshed via APS_CLIENT_ID (+ APS_CLIENT_SECRET).
//
// This tool is intentionally outside api/ because it doesn't ship in the
// release binary; delete the directory once the schema decision is made.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/schneik80/fusionlocalserver/auth"
	"github.com/schneik80/fusionlocalserver/config"
)

const endpoint = "https://developer.api.autodesk.com/mfg/graphql"

type gqlReq struct {
	Query     string         `json:"query"`
	Variables map[string]any `json:"variables,omitempty"`
}

type gqlErr struct {
	Message    string         `json:"message"`
	Extensions map[string]any `json:"extensions,omitempty"`
	Path       []any          `json:"path,omitempty"`
}

type gqlResp struct {
	Data   json.RawMessage `json:"data"`
	Errors []gqlErr        `json:"errors,omitempty"`
}

func main() {
	hubFlag := flag.String("hub", "", "hub ID (auto-pick first if empty)")
	projFlag := flag.String("project", "", "project ID (auto-pick first if empty)")
	pageLimit := flag.Int("limit", 25, "items page limit")
	flag.Parse()

	token, err := loadOrRefreshToken()
	if err != nil {
		fail("auth", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	hubID := *hubFlag
	if hubID == "" {
		hubID, err = firstHub(ctx, token)
		if err != nil {
			fail("hubs", err)
		}
		fmt.Printf("hub: %s\n", hubID)
	}
	projID := *projFlag
	if projID == "" {
		projID, err = firstProject(ctx, token, hubID)
		if err != nil {
			fail("projects", err)
		}
		fmt.Printf("project: %s\n", projID)
	}

	probe(ctx, token, projID, *pageLimit)
}

func loadOrRefreshToken() (string, error) {
	if tok := os.Getenv("APS_ACCESS_TOKEN"); tok != "" {
		return tok, nil
	}
	rt := os.Getenv("APS_REFRESH_TOKEN")
	if rt == "" {
		return "", fmt.Errorf("set APS_ACCESS_TOKEN (or APS_REFRESH_TOKEN + APS_CLIENT_ID) to authenticate the probe")
	}
	clientID := os.Getenv("APS_CLIENT_ID")
	if clientID == "" {
		clientID = config.DefaultClientID
	}
	if clientID == "" {
		return "", fmt.Errorf("APS_REFRESH_TOKEN set but no APS_CLIENT_ID to refresh with")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	td, err := auth.Refresh(ctx, clientID, os.Getenv("APS_CLIENT_SECRET"), rt)
	if err != nil {
		return "", fmt.Errorf("refresh: %w", err)
	}
	return td.AccessToken, nil
}

func firstHub(ctx context.Context, token string) (string, error) {
	q := `query { hubs(pagination:{limit:5}) { results { id name } } }`
	var resp struct {
		Hubs struct {
			Results []struct{ ID, Name string } `json:"results"`
		} `json:"hubs"`
	}
	if err := do(ctx, token, q, nil, &resp); err != nil {
		return "", err
	}
	if len(resp.Hubs.Results) == 0 {
		return "", fmt.Errorf("no hubs")
	}
	for _, h := range resp.Hubs.Results {
		fmt.Printf("  available hub: %s  (%s)\n", h.Name, h.ID)
	}
	return resp.Hubs.Results[0].ID, nil
}

func firstProject(ctx context.Context, token, hubID string) (string, error) {
	q := `query($h: ID!) { projects(hubId:$h, pagination:{limit:5}) { results { id name } } }`
	var resp struct {
		Projects struct {
			Results []struct{ ID, Name string } `json:"results"`
		} `json:"projects"`
	}
	if err := do(ctx, token, q, map[string]any{"h": hubID}, &resp); err != nil {
		return "", err
	}
	if len(resp.Projects.Results) == 0 {
		return "", fmt.Errorf("no projects in hub %s", hubID)
	}
	for _, p := range resp.Projects.Results {
		fmt.Printf("  available project: %s  (%s)\n", p.Name, p.ID)
	}
	return resp.Projects.Results[0].ID, nil
}

func probe(ctx context.Context, token, projectID string, limit int) {
	q := `query($p: ID!, $n: Int!) {
		itemsByProject(projectId: $p, pagination: { limit: $n }) {
			pagination { cursor }
			results {
				__typename
				id
				name
				... on DesignItem {
					tipRootComponentVersion {
						id
						occurrences(pagination: { limit: 1 }) {
							pagination { cursor }
							results { id }
						}
					}
				}
			}
		}
	}`
	type occ struct {
		ID string `json:"id"`
	}
	type item struct {
		Typename string `json:"__typename"`
		ID       string `json:"id"`
		Name     string `json:"name"`
		TipRoot  *struct {
			ID          string `json:"id"`
			Occurrences struct {
				Pagination struct{ Cursor string } `json:"pagination"`
				Results    []occ                   `json:"results"`
			} `json:"occurrences"`
		} `json:"tipRootComponentVersion,omitempty"`
	}
	var resp struct {
		ItemsByProject struct {
			Pagination struct{ Cursor string } `json:"pagination"`
			Results    []item                  `json:"results"`
		} `json:"itemsByProject"`
	}
	t0 := time.Now()
	if err := do(ctx, token, q, map[string]any{"p": projectID, "n": limit}, &resp); err != nil {
		fmt.Printf("\nQUERY FAILED: %v\n", err)
		return
	}
	elapsed := time.Since(t0)

	fmt.Printf("\n--- probe results (project root, limit=%d, took %s) ---\n", limit, elapsed.Round(time.Millisecond))
	var designs, assemblies, parts, drawings, configured, other int
	for _, it := range resp.ItemsByProject.Results {
		switch it.Typename {
		case "DesignItem":
			designs++
			marker := "?"
			n := -1
			cursor := ""
			if it.TipRoot != nil {
				n = len(it.TipRoot.Occurrences.Results)
				cursor = it.TipRoot.Occurrences.Pagination.Cursor
			}
			switch {
			case n < 0:
				marker = "(no tipRoot — milestone-less?)"
			case n == 0:
				marker = "PART"
				parts++
			default:
				marker = "ASSEMBLY"
				if cursor != "" {
					marker += " (>1 sub-comp, paginated)"
				}
				assemblies++
			}
			fmt.Printf("  %-9s  %s  %s\n", marker, it.Name, it.ID)
		case "DrawingItem":
			drawings++
			fmt.Printf("  %-9s  %s\n", "drawing", it.Name)
		case "ConfiguredDesignItem":
			configured++
			fmt.Printf("  %-9s  %s\n", "configured", it.Name)
		default:
			other++
			fmt.Printf("  %-9s  %s\n", it.Typename, it.Name)
		}
	}
	fmt.Printf("\nTotal:      %d items\n", len(resp.ItemsByProject.Results))
	fmt.Printf("  designs:    %d  (assemblies %d / parts %d)\n", designs, assemblies, parts)
	fmt.Printf("  drawings:   %d\n", drawings)
	fmt.Printf("  configured: %d\n", configured)
	fmt.Printf("  other:      %d\n", other)
	if resp.ItemsByProject.Pagination.Cursor != "" {
		fmt.Printf("  (more pages available — cursor returned)\n")
	}
}

func do(ctx context.Context, token, q string, vars map[string]any, out any) error {
	body, err := json.Marshal(gqlReq{Query: q, Variables: vars})
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode/100 != 2 {
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, truncate(string(raw), 400))
	}
	var env gqlResp
	if err := json.Unmarshal(raw, &env); err != nil {
		return fmt.Errorf("decode envelope: %w (body: %s)", err, truncate(string(raw), 400))
	}
	if len(env.Errors) > 0 {
		// Print all errors with extensions so cost / validation rejections are visible.
		for _, e := range env.Errors {
			fmt.Printf("  graphql error: %s  ext=%v\n", e.Message, e.Extensions)
		}
		return fmt.Errorf("graphql returned %d error(s)", len(env.Errors))
	}
	return json.Unmarshal(env.Data, out)
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

func fail(label string, err error) {
	fmt.Fprintf(os.Stderr, "probe %s: %v\n", label, err)
	os.Exit(1)
}
