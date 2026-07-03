# Claude Code Handoff — Project-Scoped Threaded Chat

**Repo:** `C:\Users\msft\Source\fusionlocalserver`
**Branch:** `chat` (new, off the default branch)
**Companion doc:** `centrifuge-chat-design.md` (the approved design — treat it as the spec)
**Mode:** PLAN FIRST. Do not write feature code until the plan below is produced and approved.

---

## Context

We have a Go backend + TypeScript frontend organized around **projects**. Each
project has users with permissions, and content regions rendered as tabs
(Dashboard, Wiki). We are adding a **Chat** tab: per-project channels (one
auto-created root channel, users create more, optional private channels),
threaded messages (Slack-style, one level deep), realtime via an embedded
`centrifuge` node. The design doc specifies the schema, Centrifuge channel
namespace, event contract, authorization model, REST surface, and frontend
subscription scoping. Follow it; where the doc conflicts with existing repo
conventions, prefer repo conventions and note the deviation.

## Task 1 — Recon (read-only)

Explore the codebase and report back:

1. **Backend layout** — HTTP framework/router in use, how routes are registered,
   middleware chain, where project authorization currently lives (find the
   function/middleware Dashboard and Wiki use — chat MUST reuse it, per §1 of
   the design).
2. **Persistence** — migration tooling (goose/golang-migrate/atlas/other?),
   DB access pattern (sqlc, pgx raw, GORM?), naming conventions for tables and
   migrations. The design's SQL must be translated into whatever the repo uses.
3. **Existing realtime** — any current WebSocket/SSE usage that chat should
   share or must not conflict with (route paths, upgrade handlers).
4. **Frontend layout** — how content-region tabs are implemented (routing,
   lazy loading), state management library (the design's `chatStore` /
   `projectStore` should be idiomatic to it), existing API client wrapper,
   where auth tokens come from (needed for the `getToken` Centrifuge callback).
5. **Users/projects schema** — actual table and PK names for users, projects,
   and project membership/roles, so the design's FKs and the role→capability
   mapping (§1) bind to real columns and role values.

## Task 2 — Implementation plan

Produce a phased plan mapping the design doc's build order (§Build order, items
1–7) onto this repo. For each phase:

- Files to create/modify (concrete paths matching repo layout)
- Migrations in repo's migration format
- New Go packages (suggest `internal/chat` or repo-consistent equivalent;
  centrifuge node lifecycle should hook into the existing server startup)
- Frontend components/routes for the Chat tab
- Tests: what the repo's existing test patterns are and which ones each phase
  needs (at minimum: authz two-layer check §4, thread-insert invariant §2,
  idempotent create §4)
- A demoable checkpoint per phase (phase 1 must work with polling, no WS)

Flag open questions rather than guessing — especially the role→capability
mapping if the repo's roles don't match viewer/member/admin, and whether an
existing WS route conflicts with `/connection/websocket`.

## Task 3 — Branch setup only

After the plan is approved: `git checkout -b chat`, commit the two docs under
`docs/chat/`, and scaffold phase 1 (migrations + empty package layout). Stop
there for review.

## Constraints

- Dependencies to add: `github.com/centrifuge/centrifuge` (Go),
  `centrifuge` (npm). Nothing else without asking.
- All chat REST handlers and the Centrifuge subscribe handler route through the
  existing project authorization — no parallel permission system.
- Clients never publish durable events over the socket; REST → DB → publish only.
- Ship `is_private` + `channel_members` schema and the `CanAccessChannel`
  two-layer check in phase 1 even though private-channel UI comes last (§7).
