# fusionlocalserver — Claude Code context

A local **BFF**: a Go HTTP server that signs the user into Autodesk Platform
Services (APS) and serves a React SPA for browsing **Fusion Team** data
(hubs → projects → folders → designs), with details, version history,
references (uses / where-used / drawings), thumbnails, BOM, and pins.

## Layout
- `auth/` — 3-legged PKCE OAuth against APS; per-session in-memory tokens (auto-refresh). **Never persist tokens.**
- `api/` — APS clients: Manufacturing Data Model **GraphQL** (`client.go`, `queries.go`, `details.go`, `refs.go`, …) and the Fusion Team notifications **activity feed** (`activity.go`, `activity_report.go`).
- `server/` — Go 1.22 `net/http.ServeMux`; routes in `routes.go`; handlers `handlers_*.go`; DTOs in `dto*.go`; session/auth middleware (`fls_session` cookie).
- `web/` — React 18 + Vite + TypeScript + MUI v6 + @tanstack/react-query (+ recharts). API wrapper `src/api/client.ts`, hooks `src/api/queries.ts`.
- `config/` — `APS_CLIENT_ID` / `APS_CLIENT_SECRET` / `APS_REGION` (env or `~/.config/fusionlocalserver/config.json`).

## Build / test / run
```
go build ./...        # backend
go test ./...         # all unit tests (offline)
cd web && npm install && npm run build   # frontend → embedded into server/webdist via //go:embed
./fusionlocalserver -server               # run (add -tls for https)
# dev: (t1) go run . -server   (t2) cd web && npm run dev   (Vite proxies /api)
```
`server/webdist` is gitignored and embedded at compile time — **build the web before `go build`** for the UI to ship in the binary.

## Conventions
- Go: `gofmt`; handlers use `reqParam` / `s.reqCtx` / `s.token` / `s.fail` / `writeJSON`; DTOs camelCase with `fmtTime`, slices never nil.
- Web: typed `request()` wrapper, react-query hooks (bump the persist `buster` in `main.tsx` when query shapes change).
- Commit/push only when asked.

## Active work
**Activity reports** (hub/project/folder/design dashboard) on branch
`feature/activity-reports`. Start here: **`docs/activity-reports/STATUS.md`**
(what's done / what's left), plus `plan.md` and `feed-contract.md`.
Phases 0–4 done (feed acquisition, aggregation, `/api/activity/report`,
dashboard). Phases 5 (milestones/comments) and 6 (live validation) remain.
