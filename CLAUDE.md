# fusionlocalserver — Claude Code context

A local **BFF**: a Go HTTP server that signs the user into Autodesk Platform
Services (APS) and serves a React SPA for browsing **Fusion Team** data
(hubs → projects → folders → designs), with details, version history,
references (uses / where-used / drawings), thumbnails, BOM, and pins.

## Layout
- `auth/` — 3-legged PKCE OAuth against APS; per-session in-memory tokens (auto-refresh). **Never persist tokens.**
- `api/` — APS clients: Manufacturing Data Model **GraphQL** (`client.go`, `queries.go`, `details.go`, `refs.go`, …). **Design activity** is GraphQL-sourced (`activity_graphql.go` → `activity_report.go`); `activity.go` keeps the shared types + `HubSlug` (the notifications feed it once used is first-party-gated — removed).
- `server/` — Go 1.22 `net/http.ServeMux`; routes in `routes.go`; handlers `handlers_*.go`; DTOs in `dto*.go`; session/auth middleware (`fls_session` cookie).
- **Local per-project stores** — features whose data is ours, not APS's. All share one posture: one JSON/JSONL file per project under `config.Dir()`, atomic temp+rename writes, a per-project mutex, `.bak` on corruption, a future-version guard, and authorization delegated to `chat.Authorizer` (APS project role → capability) rather than a parallel permission system.
  - `chat/` — append-only channel logs + the shared `Authorizer` / `Limiter`.
  - `tasks/` — `tasks.json` per project (Kanban tasks).
  - `production/` — `production.json` per project (jobs, step DAG, version-pinned documents, batches). See `docs/production/STATUS.md`.
  - `whiteboards/` — tldraw boards: `whiteboards.json` metadata per project plus one `doc-<id>.json` per board, since a document is megabytes and is rewritten on every autosave. See `docs/whiteboards/STATUS.md`.
- `web/` — React 18 + Vite + TypeScript + MUI v6 + @tanstack/react-query (+ recharts). API wrapper `src/api/client.ts`, hooks `src/api/queries.ts`. Project apps live one folder each (`src/tasks/`, `src/wiki/`, `src/chat/`, `src/production/`) and mount as tabs in `components/ProjectPanel.tsx` under the contract `({ active }: { active?: boolean })` — every tab stays mounted and gates its fetching on `active`. A tab that also *measures* its layout (charts, canvases) must additionally skip rendering when inactive: hidden tabs have zero size, so a `ResponsiveContainer` there logs a 0x0 warning on every re-render.
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
- **IDs ride in query params, never path segments** — URNs contain `:` and `/`.
- Web: typed `request()` wrapper, react-query hooks (bump the persist `buster` in `main.tsx` when query shapes change). Realtime/per-user query keys (`chat*`, `task*`, `prod*`) are excluded from localStorage persistence — see the dehydrate filter in `main.tsx`.
- **APS calls are quota'd, so never fan out per row.** The per-minute cost quota answers a burst with 429s, and `api/client.go` deliberately does *not* retry them (a retry can't replenish a per-minute budget). Anything per-item — a classify, a thumbnail — waits for the row to near the viewport via `components/useInView.ts`; anything per-container is capped with a visible "Load all" (`ACTIVITY_CAP` / `CLASSIFY_CAP` in `Dashboards.tsx`). Never cap silently.
- **One stylesheet, one exception.** There are no CSS files except `web/src/whiteboards/whiteboard.css`, which reskins tldraw (a CSS-variable-themed component that cannot be styled through `sx`). It is scoped to `.fls-tldraw`. Everything else is MUI `sx`.
- **Visualizations are hand-drawn inline SVG** — there is no graph/chart library beyond one recharts donut, no framer-motion, and no CSS files. Motion is MUI `<Slide>` plus short (100–120 ms) `sx` transitions. `RelationGraph.tsx` (pan/zoom + bezier edges), `HistoryGraph.tsx` (lanes) and `ActivityHeatmap.tsx` (isometric) are the reference implementations.
- **Card tokens** — `fls:doc` / `fls:task` / `fls:job` / `fls:batch` are compact pseudo-URL tokens stored inline in chat/wiki/task bodies and unfurled at render time. `components/RefCard.tsx` maps every scheme to its renderer; `components/reftokens.ts` splits them out of plain text.
- Commit/push only when asked.

## Active work
**Whiteboards** on branch `feat/whiteboards` — a per-project tldraw board (fifth
project app). Draw freely and drop **live app cards** (`fls:doc` / `fls:task` /
`fls:job` / `fls:batch`) onto the canvas: the custom `fls-card` shape stores only
the token and renders it through the shared `components/RefCard.tsx`, so a card
is the real task/batch, not a screenshot. tldraw is lazy-loaded (~1.7 MB) so it
stays out of the entry bundle. **It needs a licence key** (`VITE_TLDRAW_LICENSE_KEY`
in the git-ignored `web/.env.local`; see `web/.env.example`) — without one tldraw
silently replaces the board with a hidden div 5 s after mount. The current key is
an **evaluation licence expiring 2026-10-29**; see `docs/whiteboards/STATUS.md`.

Previously: **Production** on branch `Production` — a light MES / product tracker, the fourth
project app beside Tasks, Wiki and Chat. A **Job** is a graph of **Steps** carrying
version-pinned plan documents and placeholder slots; a **Batch** is a dated run
that *freezes* the plan (steps, pinned versions, placeholders are deep-copied), so
later plan edits can never rewrite what a run recorded. Documents are supplied by
browsing the hub or uploading, and every pin resolves its version server-side
(`api/production_snapshot.go`). UI in `web/src/production/`: a pan/zoom SVG flow
canvas, a list view, batches with a prove/production timeline, plus a cross-project
screen on the nav rail. See **`docs/production/STATUS.md`**.

Previously: **Design activity** (branch `feature/activity-reports`) — a per-design
**Activity tab** (`web/src/components/ActivityHeatmap.tsx`), an isometric heat map
off the GraphQL design report (`/api/activity/report?scope=design&hubId=…&id=…`).
The hub/project/folder dashboard was **removed** (the notifications feed it relied
on is first-party-gated). See **`docs/activity-reports/STATUS.md`**.
