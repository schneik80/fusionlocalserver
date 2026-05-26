# Security Follow-ups

Originally findings from the security review of 2026-05-02. The high-severity
items (H1, H2) and the Python-injection mitigation (M2) shipped first. The
dedicated-server refactor (per-user login, TUI removal) then resolved or
obviated most of the rest. This file tracks what remains.

## Resolved

- [x] **M1. OAuth `state` parameter (CSRF).** The per-user login flow generates
  a random `state`, stores it server-side keyed to the in-flight login, and
  rejects the callback unless the `state` query param matches both that entry
  and the `fls_pending` cookie. See `server/auth.go` and `docs/authentication.md`.
- [x] **M3. Redact signed URLs from debug logs.** `api/debug.go` runs every
  trace line through `redactSignedURLs` before it reaches the console or log
  file, so `"signedUrl":"…"` values never appear in `-v` output.
- [x] **L3. Refresh `golang.org/x/text` / indirect deps.** Moot — removing the
  TUI dropped the last third-party dependency. The module is now pure Go
  standard library (no `go.sum`).
- [x] **L4. Bump the `go` directive.** Now `go 1.23`.

## Obviated by the architecture change

- **L1. Tokens at rest.** There is no longer an on-disk token file. Each user's
  tokens live only in the server's in-memory session store, so the
  keychain-vs-plaintext question no longer applies. (See the new "session
  persistence" item below.)
- **L2 / L5. `OpenBrowser` URL-scheme validation / error surfacing.** The server
  no longer opens a browser on the host (`OpenBrowser` was deleted with the
  loopback login). Each user authenticates in their own browser.

## New — deferred

- [ ] **APS app callback registration.** The login flow derives `redirect_uri`
  from the request origin (`<scheme>://<host>/api/auth/callback`). APS requires
  each such origin to be registered as an exact-match Callback URL (no
  wildcards), and the runtime port-change feature multiplies the set. Tracked
  with the broader "APS client_id/secret provisioning" work, which is out of
  scope for the refactor.
- [ ] **TLS / `Secure` cookie.** The LAN listener is plain HTTP, so the session
  cookie cannot be `Secure` (browsers drop `Secure` cookies over `http://`) and
  a wire sniffer could hijack a session. The cookie already sets `Secure` from
  `r.TLS`, so fronting the server with TLS (or adding a TLS listener) closes
  this with no code change to the cookie logic.
- [ ] **Session persistence across restarts.** Sessions are in-memory, so a full
  process restart logs everyone out (a runtime port rebind does not). Encrypted
  at-rest session persistence would survive restarts; weigh against the added
  complexity of protecting tokens on disk.
