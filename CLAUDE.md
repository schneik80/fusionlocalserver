# fusionlocalserver — Claude Code context

A local **BFF**: a Go HTTP server that signs the user into Autodesk Platform
Services (APS) and serves a React SPA for browsing **Fusion Team** data
(hubs → projects → folders → designs), with details, version history,
references (uses / where-used / drawings), thumbnails, BOM, and pins.

## Layout
- `auth/` — 3-legged PKCE OAuth against APS; per-session in-memory tokens (auto-refresh). **Never persist tokens.**
- `api/` — APS clients: Manufacturing Data Model **GraphQL** (`client.go`, `queries.go`, `details.go`, `refs.go`, …). **Design activity** is GraphQL-sourced (`activity_graphql.go` → `activity_report.go`); `activity.go` keeps the shared types + `HubSlug` (the notifications feed it once used is first-party-gated — removed).
- `server/` — Go 1.22 `net/http.ServeMux`; routes in `routes.go`; handlers `handlers_*.go`; DTOs in `dto*.go`; session/auth middleware (`fls_session` cookie).
- `web/` — React 18 + Vite + TypeScript + MUI v6 + @tanstack/react-query (+ recharts). API wrapper `src/api/client.ts`, hooks `src/api/queries.ts`.
- `config/` — `APS_CLIENT_ID` / `APS_CLIENT_SECRET` / `APS_REGION` (env or `~/.config/fusionlocalserver/config.json`). Build-time `config.Default{ClientID,Region,PublicURL}` are injected via ldflags from `.aps-client-id` / `.aps-region` / `.aps-public-url` (git-ignored); `DefaultPublicURL` bakes in the canonical OAuth callback host so the binary needs no `-public-url` flag.

## Build / test / run
```
go build ./...        # backend
go test ./...         # all unit tests (offline)
cd web && npm install && npm run build   # frontend → embedded into server/webdist via //go:embed
make run                                 # build UI + binary, serve over HTTPS (-tls is on by default)
./fusionlocalserver -tls                 # or run the built binary directly (HTTPS; self-signed cert auto-generated)
# dev: (t1) go run . -dev   (t2) cd web && npm run dev   (Vite proxies /api)
```
`server/webdist` is gitignored and embedded at compile time — **build the web before `go build`** for the UI to ship in the binary.

## Conventions
- Go: `gofmt`; handlers use `reqParam` / `s.reqCtx` / `s.token` / `s.fail` / `writeJSON`; DTOs camelCase with `fmtTime`, slices never nil.
- Web: typed `request()` wrapper, react-query hooks (bump the persist `buster` in `main.tsx` when query shapes change).
- Commit/push only when asked.

## Active work
**Design activity** on branch `feature/activity-reports`. Shipped as a per-design
**Activity tab** (`web/src/components/ActivityHeatmap.tsx`) — an isometric heat map
with Day/Week/Month/Year windows + a prev/next stepper, off the GraphQL design
report (`/api/activity/report?scope=design&hubId=…&id=…`). The hub/project/folder
dashboard was **removed** (the notifications feed it relied on is first-party-gated).
See **`docs/activity-reports/STATUS.md`**.
