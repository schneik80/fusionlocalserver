# Chat security addendum

Companion to `SECURITY-REVIEW-2026-06.md`, covering the project-chat feature
(`chat/`, `server/handlers_chat*.go`, `web/src/chat/`) as shipped through
phase 5 of `docs/chat/PLAN.md`. Chat is the first feature in this codebase
that both **persists user-authored content** and **relays it between users**,
so its posture is recorded here explicitly.

## Authorization

- **No parallel permission system.** Every REST handler and the SSE stream
  front-door through `chat.Authorizer`, which resolves the caller's APS
  project role (`api.GetProjectMembers`, fetched with the caller's *own*
  token) into capabilities: Viewer/Reader → read; Editor+ → post/react/edit
  own/create channels; Manager/Administrator → moderate. A roster row that is
  present but **non-ACTIVE** (PENDING/INACTIVE), or carries an unrecognized
  role, denies by default.
- **Group-only members** (access via a project group, so absent from
  `folderLevelProjectMembers` — which enumerates only direct contributors and
  can't be group-expanded without hub-admin) are granted **contributor**
  capabilities (read/post/react/edit-own/create-channel) but never
  moderation. The signal is that the caller's *own token* successfully read
  the project roster — the same "your token sees the project or it doesn't"
  trust the Dashboard and Wiki tabs already run on; a caller with no project
  access gets an APS error on that fetch and is denied. This is a product
  call (`docs/chat/PLAN.md` open question 3): it favors a usable chat for
  group-based orgs over withholding write from members whose exact role we
  can't read. The self-only fallback lives in `Can`/`CanAccessChannel`;
  third-party membership checks (private-channel invitees) use the strict
  `IsActiveMember`, which never applies it.
- **Private channels are two-layer** (`CanAccessChannel`): project read
  access AND (public ∨ ACL member ∨ project moderator). Inaccessible private
  channels answer **404, not 403**, everywhere — their existence never leaks.
  The channel list, the unreads summary, and per-frame SSE entitlement all
  reuse the same check.
- **Revocation is TTL-bounded.** Role cache: 60 s positive / 15 s negative.
  REST access lapses at TTL; the SSE keepalive tick (25 s) re-checks
  entitlement and closes streams whose caller lost read access. A roster
  fetch *error* is not a revocation (streams ride out APS blips).
- **User-only events.** `read.updated` frames carry `Vis.UserOnly` and reach
  exactly the addressed user — verified down to the moderator case
  (`TestSSE_ReadUpdatedIsUserOnly`).

## Input handling

- Request bodies capped at 64 KiB (`http.MaxBytesReader`); message bodies
  ≤ 4000 runes, names ≤ 80, topics ≤ 500 — enforced at the HTTP boundary
  *and* in the store, so no caller can bypass them.
- Reactions on **add** must come from the fixed server-side palette
  (`chat.AllowedEmoji`); removal is unrestricted so nothing gets stuck.
- Read cursors: negative seqs 400; over-large seqs clamp to the channel's
  newest seq; moves are monotonic (no rewind by racing tabs).
- Typing pings are ephemeral: never stored, never replayed, no SSE id, and
  gated on `CapPost` + channel access.
- Project/channel ids appearing in filesystem paths pass `sanitizeID`
  (allowlist `[A-Za-z0-9_.-]`, 120-char cap) — null bytes and `../` are data,
  not paths (`TestChatNullByteParams`).
- Fuzzing: `FuzzChatMessageCreateBody`, `FuzzChatChannelCreateBody`,
  `FuzzChatMarkReadBody` run the real mux with a live session; invariant is
  no panic, no 5xx, contract statuses only. (~10⁶ execs each at last run.)

## Rate limiting (per session)

| Limiter | Rate | Burst | Covers |
|---|---|---|---|
| `chatMsgLim` | 2/s | 5 | message create |
| `chatOpLim` | 10/min | 10 | channel create/rename/archive |
| `chatSyncLim` | 2/s | 20 | read cursors + typing pings |

## Storage & output

- Files live under `~/.config/fusionlocalserver/chat/`, dirs 0700, files
  0600; meta/cursors rewritten atomically (temp + rename); JSONL logs are
  append-only with truncated-tail recovery. Data written by a newer build is
  refused (503), never rewritten.
- Message bodies are **never logged** — store errors log paths/reasons only;
  APS/authz failures go through the generic `s.fail` envelope.
- The SPA renders bodies as plain text (React escaping; no markdown, no
  `dangerouslySetInnerHTML`); CSP from `securityHeaders` backstops.
- Chat react-query caches are excluded from localStorage persistence, so
  private-channel content never lands on a shared machine's disk.

## Known bounds (accepted)

- Roles cache means a removed user can read (not write new fetches) for up
  to 60 s (15 s for group-derived entries) and their open stream up to one
  keepalive tick. Real project removal makes the user's own token fail to
  read the roster (an error the keepalive tick rides out as a transient
  blip); the tick tears a stream down on an ACTIVE→suspended transition,
  and their REST calls fail immediately either way.
- Group-only members can't have their exact role read without hub-admin, so
  they get contributor (not moderator) access and can't be *added* to a
  private channel's ACL as an invitee (they aren't individually listed, so
  `IsActiveMember` can't confirm them). See the Authorization section.
- The group-derived grant trusts APS to reject a non-member's roster query
  (as it does `GetGroupMembers` for non-hub-admins). If a future APS change
  returned an empty roster instead of an error to a non-member, the fallback
  would need a stricter access probe.
- Multi-process servers sharing one config dir are a non-goal (single
  writer assumed).
