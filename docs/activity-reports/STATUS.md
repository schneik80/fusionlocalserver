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

## Remaining (need a live OAuth session — couldn't run offline)

- **Phase 5 spikes:** milestone count via the Manufacturing Data Model GraphQL (confirm the
  `isMilestone` field is queryable across `itemVersions`); comment text via each feed object's
  `links rel=comment` (when `postCount>0`).
- **Phase 6 validation:** run `./fusionlocalserver -server`, sign in, open the **Activity** view; then
  cross-check a known design (e.g. `_CEBIT_LASTPENDEL_FINAL`, feed `version:10`) against the Fusion
  Team web UI. Confirm our app's own 3-legged token is accepted by the (undocumented) feed endpoint.

## Run

```
go build ./... && ./fusionlocalserver -server        # add -tls for https
# or dev: (terminal 1) go run . -server   (terminal 2) cd web && npm run dev
```
Open the app, pick a hub, click the **Activity** rail icon.
