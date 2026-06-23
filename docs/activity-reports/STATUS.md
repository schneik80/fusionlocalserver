# Activity reports — implementation status

Branch: `feature/activity-reports`. Plan: `plan.md`. Feed contract: `feed-contract.md`.

## Done (building green, unit-tested offline)

**Backend (Go)**
- `api/activity.go` — paginated Fusion Team Notifications feed client + normalization to
  `ActivityEvent` (full hub→project→folder→design hierarchy, absolute timestamps, last-actor vs owner,
  version, views/comments/likes, COMMUNITY lifecycle events). `HubSlug` derives the feed's hub slug
  from the GraphQL hub's AltID / WebURL.
- `api/activity_report.go` — `BuildReport(events, scope, id, bucket, from, to)`: scope filtering
  (hub/project/folder/design), time-bucketing (hour/day/month/year), contributor rollups, design &
  version counts, created/last-change, child breakdown, capped recent-events.
- `server/handlers_activity.go` + `server/dto_activity.go` + route — `GET /api/activity/report`
  (`hub`, `scope`, `id`, `bucket`, `from`, `to`). `ItemDTO.slug` added (hubs) so the UI gets the slug.
- Tests: `api/activity_test.go` (pagination/normalization vs real-shaped fixtures in
  `api/testdata/`), `api/activity_report_test.go` (aggregation), `server/handlers_activity_test.go`.

**Frontend (React/Vite/MUI + recharts)**
- `web/src/components/ActivityDashboard.tsx` — time-series bar chart (hour/day/month/year toggle),
  summary stat cards, contributors, drill-down breakdown (drills via feed-native child ids), recent
  activity table. Scope follows the selected hub; drills into projects→folders→designs.
- `web/src/api/{types,client,queries}.ts` — `ActivityReport` types, `api.activityReport`,
  `useActivityReport`. Nav rail gains a Browser/Activity view toggle (`NavRail`, `AppLayout`).
  Query-cache buster bumped `fls-1` → `fls-2`.

## Feed is dead for this app — pivoted to GraphQL (2026-06-23)

Live validation answered the Phase 0 open risk **NO**: the undocumented Fusion Team notifications
feed returns **HTTP 500** for our own APS app's token (nginx/text-html/empty body, no
`WWW-Authenticate`; request byte-identical to the working web-client capture except the token).
Broadening OAuth scopes made no difference — it is first-party-gated. So the feed-backed acquisition
(`api/activity.go`) is unusable here. See memory `activity-feed-token-rejected`.

**Acquisition rebuilt on the Manufacturing Data Model GraphQL** — the token that works everywhere
else in the app. `BuildReport`/DTOs/UI are unchanged (still consume `[]ActivityEvent`); only the
acquisition layer swapped.

- ✅ **Design scope** — `api/activity_graphql.go` `GetDesignActivity` (lean `item` + `itemVersions`
  query; each version → one `ActivityEvent`). `GET /api/activity/report?scope=design&hubId=<gqlHubId>&id=<lineageUrn>`.
  **Validated** 2026-06-23 against `Cylinder Body` (hub `autodesk8083`): 4 versions, 1 contributor,
  created 2026-05-18, last change 2026-05-22 — matches GraphQL source. Test: `api/activity_graphql_test.go`.
  - Note: query omits `fusionWebUrl` — MFG intermittently 500s that one leaf field, and gqlQuery's
    retry logic then discards the whole (otherwise complete) response. Lost the design deep-link for now.

## Remaining

- **Folder / project / hub scopes** — still wired to the dead feed in `server/handlers_activity.go`
  (return 502). Rebuild on GraphQL: enumerate items in scope (`GetProjectItems`/`GetItems`) → fan out
  `GetDesignActivity` → aggregate. Cap/lazy-load hub scope (could be many designs).
- **Dashboard navigation** — UI still lands on hub scope (broken) and drills via feed-native child ids.
  Rework to navigate into a design and call the GraphQL design path; then rebuild parent scopes.
- **Phase 5 spikes:** milestone count via `isMilestone` across `itemVersions`; comment text. (Comments
  and views/likes were feed-only social signals — not available from GraphQL history.)

## Run

```
make run                                  # build UI + binary, serve over HTTPS (-tls default)
# or: go build ./... && ./fusionlocalserver -tls
# or dev: (terminal 1) go run . -dev   (terminal 2) cd web && npm run dev
```
Open the app (https://<host>:8080), pick a hub, click the **Activity** rail icon.
