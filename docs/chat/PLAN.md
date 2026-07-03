# Project-Scoped Threaded Chat — Recon + Phased Plan

Deliverable for the handoff's Tasks 1–2. On approval, Task 3 executes: create branch
`chat`, commit the two spec docs under `docs/chat/`, scaffold phase 1, stop for review.

## Context

`claude-code-handoff.md` + `centrifuge-chat-design.md` specify a per-project Chat tab
(channels → messages → one-level threads), realtime events, REST → store → publish, and
authorization derived from existing project permissions. The handoff's governing rule:
**"where the doc conflicts with existing repo conventions, prefer repo conventions and note
the deviation"** — and the recon shows the doc was written against an idealized repo that
differs fundamentally from this one.

## Task 1 — Recon findings

| Design doc assumed | Reality in fusionlocalserver |
|---|---|
| Postgres + migrations | **No database.** Pure-stdlib Go 1.25 module (sole dep `golang.org/x/sync`). Persistence = JSON files under `~/.config/fusionlocalserver/` ([pins/pins.go](pins/pins.go); AES-GCM session file [server/session_persist.go](server/session_persist.go)) |
| Local `projects`/`users`/`project_members` tables, BIGINT ids | Projects & users live in **APS Fusion Team** (string URN IDs), proxied live per-user. No local tables |
| Shared `authz.Can` used by Dashboard/Wiki | **None locally** — access is implicit (your APS token sees the project or it doesn't). But [api/permissions.go](api/permissions.go) `GetProjectMembers` returns each member's `{UserID, Name, Email, Role, Status}` with the `FolderRoleEnum` ladder Viewer→Reader→Editor→Manager→Administrator — a chat capability layer can be built on it |
| Content tabs (Dashboard, Wiki) | Dashboard = full Slot-B pane; **Wiki exists only on unmerged `origin/wiki`**, which adds the project tab shell `ProjectPanel.tsx` (Dashboard/Wiki tabs, both kept mounted) |
| Existing realtime | None; no `/connection/websocket` conflict. `statusRecorder` middleware already forwards `Flush()` (Vite HMR) — SSE-compatible |
| `chatStore`/`projectStore` | react-query + context reducers only ([web/src/state/nav.tsx](web/src/state/nav.tsx)); persisted query cache with buster `fls-6` |
| Token-based WS auth | Cookie-only (`fls_session`); same-origin SSE sends it automatically via `requireAuth` |
| Nested REST paths `/projects/{pid}/channels/{cid}` | Repo convention ([client.ts:3](web/src/api/client.ts:3)): URN ids "always travel as query params, never path segments" |

**Multi-user is real** (LAN BFF; each browser user signs in with their own Autodesk account
against the same instance), so same-instance realtime chat is coherent. Conventions that
transfer directly: `ServeMux` method patterns + `prot(requireAuth)` in
[server/routes.go](server/routes.go); helpers `reqParam`/`reqCtx`/`s.token`/`s.fail`/`writeJSON`;
camelCase DTOs, `fmtTime`, slices never nil; table-driven httptest tests with
`internal/testutil.GraphQLServer`; frontend typed `request()` + react-query hooks with the 2s
`refetchInterval` pattern (thumbnails) for the polling checkpoint.

Handoff typo: the Go import path would be `github.com/centrifugal/centrifuge`, not
`github.com/centrifuge/centrifuge` — moot, see Decisions.

## Decisions (user-confirmed)

1. **Storage: file store, repo-style** — per-project JSON/JSONL under
   `~/.config/fusionlocalserver/chat/`, in-memory index + mutex, pins-style; SQL invariants
   become code + tests. `isPrivate` + channel members + two-layer check ship in phase 1.
2. **Transport: stdlib SSE** — in-process hub, cookie auth, since-cursor recovery. Keeps the
   design's `{type, v, data}` envelope and event vocabulary. **Zero new dependencies** on
   either side (native `EventSource`; no centrifuge Go or npm).
3. **Roles: APS-backed** — Viewer/Reader→read; Editor/Manager/Administrator→post, react,
   edit own, create channels; Manager/Administrator→moderate. Cached per (user, project),
   short TTL.
4. **UI: project tab shell mirroring `origin/wiki`'s `ProjectPanel.tsx`** so `chat` and
   `wiki` branches merge cleanly (Dashboard/Wiki/Chat).

### Key deviations from the design doc (per handoff rule, noted)

- Postgres → versioned file store; `reply_count`/`last_reply_at` **derived at JSONL replay**,
  never dual-written.
- centrifuge namespaces (`proj:{pid}`, `…:chan:{cid}`, `:eph`, `user:{uid}`) → **one
  multiplexed SSE stream per tab** (`GET /api/chat/events?projectId=…`), filtered
  server-side per subscriber's entitlements. Privacy-aware routing (§3) is preserved more
  simply: every stream is already per-user, so private-channel meta events are just not
  written to unentitled subscribers.
- Nested REST → flat `/api/chat/*?projectId=…&channelId=…` (URN convention).
- int64 ids → string URNs; per-channel monotonic `seq` for message ids/pagination cursors.
- Multi-instance scaling (§6) → explicit non-goal (single-process LAN tool).

## Identity (folded into Phase 1)

`auth.UserProfile` has only Name/Email; the OIDC userinfo response also carries `sub`
(Autodesk user id) which is simply not decoded today.

- Add `Sub` to `UserProfile` ([auth/userinfo.go](auth/userinfo.go)) — no new network call.
- Expose `id` in `UserDTO` / `handleAuthMe` ([server/auth.go](server/auth.go)) and the
  frontend `AuthMe` type, for own-message affordances.
- `requireAuth` additionally injects the `*Session` via a new `sessionCtxKey` +
  `sessionFromCtx` helper (existing `s.token()` untouched).
- Messages store `authorId` (sub) + denormalized `authorName`. Caller's APS roster row is
  matched by `sub == Member.UserID`, **email fallback** until Open Question 1 is settled.

## New Go package `chat/` + file store

```
~/.config/fusionlocalserver/chat/<projectSlug>/     (slug via pins-style sanitizer)
  meta.json         version, eventEpoch, channels[] {id,name,topic,isRoot,isPrivate,
                    createdBy,createdAt,archivedAt,nextMsgSeq,members[] (private only)}
  msg-<cid>.jsonl   append-only ops: create|edit|delete|react|unreact, each with "v":1
  cursors.json      per-user read cursors (phase 4)
```

- meta.json written atomically (temp + `os.Rename`), 0600; JSONL appended through one
  long-lived `O_APPEND` handle per channel, all mutations under a per-project mutex.
- **Versioning = migration analog**: `version` field gates load; older → in-place upgrade
  fns; newer → refuse (503 for that project); truncated tail line (crash mid-append) →
  drop and continue; corrupt file → rename `.bak`, start clean (pins precedent).
- Invariants enforced in code, all unit-tested: one root channel per project (lazy
  `EnsureRoot` → `general` on first chat access); root never private/archivable; unique
  `(channel, clientMsgId)` → idempotent create (200 vs 201); one-level threading (root must
  be top-level, non-deleted, same channel); unique case-insensitive channel names; length caps.

**Authorizer** (`chat/authz.go`): `Can(ctx, token, userID, email, projectID, cap)` and
two-layer `CanAccessChannel` (§1: project role, then private-channel membership OR
moderator). Role fetched via `api.GetProjectMembers` with the caller's own token
(`Status == ACTIVE` required; unknown role → deny+log), cached per (user, project) — TTL
60s positive / 15s negative, `singleflight` to collapse concurrent fetches (x/sync already
the module's dep). Revocation: REST lapses at TTL; the SSE hub re-checks entitlements on
its 25s keepalive tick and closes dropped users' streams (the SSE analog of
`node.Unsubscribe`).

## Phased plan

### Phase 1 — Store + REST + authz + polling UI (build items 1 + 7-schema)

Backend create: `chat/types.go`, `chat/store.go`, `chat/jsonl.go`, `chat/authz.go`,
`chat/ratelimit.go` (per-session token buckets → 429), `server/handlers_chat.go`
(via `reqParam`/`s.reqCtx`/`s.token`/`sessionFromCtx`/`s.fail`/`writeJSON`;
`http.MaxBytesReader` 64 KiB; body ≤ 4000 runes, name ≤ 80, topic ≤ 500),
`server/dto_chat.go`.
Backend modify: `auth/userinfo.go`, `server/auth.go`, `server/server.go` (store+authorizer
fields), `server/routes.go`:

```
GET/POST      /api/chat/channels?projectId=
PATCH/DELETE  /api/chat/channels?projectId=&channelId=        (DELETE=archive; root→400)
POST/DELETE   /api/chat/channels/members?projectId=&channelId=[&userId=]
GET/POST      /api/chat/messages?projectId=&channelId=[&beforeSeq=&afterSeq=&limit=]
PATCH/DELETE  /api/chat/messages?projectId=&channelId=&seq=   (own or moderator)
GET           /api/chat/thread?projectId=&channelId=&rootSeq=
POST/DELETE   /api/chat/reactions?projectId=&channelId=&seq=
```

All `prot(...)`-wrapped; every handler goes through `Authorizer.Can`/`CanAccessChannel` —
no parallel permission system.

Frontend: `web/src/components/ProjectPanel.tsx` **mirroring the wiki branch's shell
byte-for-byte** (tabs `dashboard | chat`, panes kept mounted via `display`) so the eventual
merge is a two-line conflict; `BrowserStage.tsx` swaps `<ProjectDashboard/>` →
`<ProjectPanel/>` (same 5-line change as wiki); new `web/src/chat/` (ChatApp, ChannelSidebar,
MessageList, MessageComposer, types) mirroring `web/src/wiki/*`; `api/client.ts` +
`api/queries.ts` hooks — `useChatMessages` with `refetchInterval: 2000` (thumbnail-poller
pattern); `main.tsx`: exclude `chat*` query keys from persistence (private content must not
land in shared-machine localStorage).

XSS: bodies render as plain text via JSX escaping only — no `dangerouslySetInnerHTML`, no
markdown in phase 1; CSP backstop. Log hygiene: `s.fail` for APS/authz errors, generic 500
for store errors, message bodies never logged.

Tests (table-driven; `testutil.GraphQLServer` for roster; `t.TempDir()` stores):
- `chat/authz_test.go` — role×capability table; not-in-roster; PENDING denied; cache TTL +
  singleflight; **two-layer check** incl. private-channel deny/member-allow/admin-allow
- `chat/store_test.go` — **thread-insert invariant** (reply-to-reply, cross-channel,
  deleted root); **idempotent create**; one-root; root-never-private; derived reply counts
- `chat/jsonl_test.go` — crash/reload roundtrip (truncated tail), version-too-new refusal,
  corrupt→`.bak`
- `chat/store_race_test.go` — 20 goroutines × 50 msgs under `-race`, unique contiguous seqs
- `server/handlers_chat_test.go` — 401 no session; 403 Viewer POST; 201/200 dedupe;
  oversize 400; 429; private channel invisible to non-member

**Checkpoint:** two browsers, two Autodesk accounts, same project → `general` auto-exists,
messages + thread replies flow within 2s (polling); Viewer sees timeline, composer disabled,
POST 403; server restart loses nothing.

### Phase 2 — SSE hub + `/api/chat/events` + publish-on-write (item 2)

Create `chat/hub.go`, `server/handlers_chat_events.go`. Per-project subscriber set + ring
buffer (512 events / 10 min) with SSE ids `<epoch>-<seq>`; `eventEpoch` bumps at startup so
stale `Last-Event-ID` → `event: reset` → client refetches via `afterSeq` REST then resumes
(also the ring-overflow path — §5's "gap too big"). Headers `text/event-stream`,
`no-cache, no-transform`; flush per event; `: ping` every 25s doubling as the entitlement
re-check tick; unregister on ctx done. Publishes added to every mutating handler
(REST → store → publish; clients never publish durable events). **Wire `Hub.CloseAll()`
before `drain()` in server.go's shutdown/rebind paths** — otherwise open streams eat the
10s graceful-shutdown budget and block port rebind.

Tests: end-to-end SSE over `httptest.NewServer(s.routes())` — live delivery, Last-Event-ID
replay, stale-epoch reset, private-event visibility (non-member stream never sees it),
revocation teardown (flip fake roster, advance TTL), prompt shutdown.

**Checkpoint:** `curl -N /api/chat/events?projectId=…` shows framed events while a browser
posts; kill/restart server → curl reconnect gets `reset`.

### Phase 3 — Frontend goes live (items 3–4)

`web/src/chat/useChatEvents.ts`: one `EventSource` per open project (mounted in
ProjectPanel, any tab), events applied straight into the react-query cache
(`setQueryData` for `message.created`, channel patches, `channel.activity` sidebar bolding);
`reset`/`onerror` → `invalidateQueries` + resume, with the 2s poll re-arming as fallback
while the stream is down. Optimistic sends keyed on `crypto.randomUUID()` clientMsgId,
reconciled by echoed SSE event or REST response, whichever first (§5 verbatim, minus
centrifuge). Own-message affordances from `useAuthMe().user.id`.

**Checkpoint:** browser A → B instant delivery; exactly one `/api/chat/events` connection
per tab in DevTools; server kill → silent reconnect + backfill.

### Phase 4 — Threads UI, read cursors/unreads, reactions, typing (item 5)

`ThreadDrawer.tsx`, `TypingIndicator.tsx`; `chat/cursors.go` → `cursors.json`; routes
`PATCH /api/chat/read`, `POST /api/chat/typing` (CapPost; ephemeral fan-out, **no `id:`
field** so it never advances Last-Event-ID), `GET /api/chat/unreads`. Thread drawer live via
the same stream filtered on `threadRoot`; `read.updated` syncs unreads across the user's own
tabs. Emoji validated against a server-side allowlist.

Tests: cursor roundtrip + crash-reload; unread computation table; typing id-lessness;
reaction toggle idempotence.

**Checkpoint:** Slack-parity loop — unread bolding, thread badges/drawer, reactions,
"N is typing…", live in two browsers.

### Phase 5 — Private-channel UI, moderation, security pass (items 6–7)

Channel-create dialog + member picker (reuse the permissions member surface), member
management, rename/archive + delete-others for Manager/Administrator —
`channel.member_added/removed` events update sidebars live; everything upstream already
routes through the two-layer authorizer, so this is additive (the doc's item-7 promise,
honored because phase 1 shipped the schema + check). Security addendum under
`docs/security/` (limits, allowlists, 0600 files, no-body logging, TTL-bounded revocation)
+ chat fuzz entries mirroring the pins fuzz in `server/fuzz_security_test.go`.

**Checkpoint:** private channel invisible to a third member (403), visible to project
Administrator; removing a member drops it from their UI live.

## Verification (end-to-end)

- `go test ./...` offline at every phase (fake GraphQL roster, TempDir stores, httptest SSE).
- `cd web && npm run build` (tsc gate) then `make run`; manual matrix per phase checkpoint
  above with two Autodesk accounts (documented in `docs/chat/`).
- Restart/crash drills: kill -9 mid-append → reload trims tail; restart mid-stream →
  EventSource reset/backfill. `-race` on store tests.

## Open questions (flagged, not guessed)

1. **Is OIDC `sub` == MDM GraphQL `user.id`?** Verify with a `-v` probe before phase 1
   ships (avoids an authorId-space migration); email fallback keeps work unblocked.
2. **Exact `FolderRoleEnum` wire casing** — confirm against one live response; unknown role
   → deny+log.
3. **Group-only members**: users reachable only via a project *group* don't appear in
   `folderLevelProjectMembers` and `GetGroupMembers` needs hub-admin → chat can't resolve
   their role. Proposed: read-only + UI hint. Product call.
4. **HTTP/1.1 6-conn/origin** limit with one SSE stream per tab → document `-tls` (HTTP/2)
   as the multi-tab-safe mode; SharedWorker later if needed.
5. **Vite `:5173`-direct dev**: verify the proxy streams SSE unbuffered; `-dev` Go-proxy
   mode sidesteps it.
6. **Windows share-mode/AV interference on JSONL opens** — single long-lived handle +
   one retry-with-backoff on open; multi-process on one config dir stays a non-goal.
7. **No frontend test harness exists** — chat UI verification stays checkpoint-based unless
   the repo adopts vitest separately.

## Immediately after approval (handoff Task 3 — then stop for review)

1. `git checkout -b chat`
2. Copy the two spec docs + this plan into `docs/chat/` and commit.
3. Scaffold phase 1: `chat/` package skeleton (types + store API surface + version-gated
   loader), route stubs, empty `web/src/chat/` — compiling, tested store skeleton, no
   feature behavior. Stop for review.
