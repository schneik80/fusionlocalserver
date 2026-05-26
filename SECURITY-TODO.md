# Security Follow-ups

Findings from the security review of 2026-05-02. The high-severity items
(H1, H2) and the Python-injection mitigation (M2) shipped together; the
items below remain.

## Medium

- [ ] **M1. Add `state` parameter to OAuth flow** — `auth/oauth.go`
  - PKCE covers code-injection because the verifier is local, but `state`
    is the standard CSRF defense for the redirect step (RFC 6749 §10.12,
    RFC 9700) and costs nothing.
  - Generate a random `state` alongside the verifier in `Login`, include
    it in `buildAuthURL`, and reject the callback in `WaitForCallback`
    if it doesn't match. Plumb the expected value through to the
    callback handler.

- [ ] **M3. Redact signed URLs from debug logs** — `api/client.go`, `api/debug.go`
  - When `FUSIONLOCALSERVER_DEBUG=1`, the GraphQL response bodies logged via
    `dbgLog` include `signedUrl` fields for STEP derivatives. Those
    URLs are credentials.
  - Either: (a) regex-redact `"signedUrl":"…"` substrings before
    appending to the buffer, or (b) document on the debug overlay that
    logs may contain credentials and should not be shared.

## Low

- [ ] **L1. Move tokens to OS keychain** — `auth/tokens.go`
  - Currently plaintext at `~/.config/fusionlocalserver/tokens.json` (mode
    0600). Conventional for a CLI but the macOS Keychain / Windows
    DPAPI / Linux secret-service would be stronger.
  - At minimum, mention storage location + plaintext in the About
    overlay so users know.

- [ ] **L2. Validate URL scheme before opening browser** — `auth/oauth.go`,
  `ui/app.go`
  - `OpenBrowser` is called with `details.FusionWebURL` from the API
    response. `exec.Command` is shell-free, so OS injection isn't the
    risk — but a malicious server could return `javascript:` or
    `data:` URLs.
  - Parse with `net/url` and refuse anything that isn't `http`/`https`.

- [ ] **L3. Refresh `golang.org/x/text` and other indirect deps**
  - `go.sum` pins `golang.org/x/text v0.3.8` (the version that fixed
    CVE-2022-32149, so we're safe — but well behind current). Bump on
    next dep refresh; reduces scanner noise and pulls in subsequent
    fixes for the rest of the `x/*` set.

- [ ] **L4. Bump `go` directive in `go.mod`**
  - Currently `go 1.22`. Move to `go 1.23` (or current stable) to pick
    up `net/http` and `crypto/*` fixes accumulated since 1.22's
    release.

- [ ] **L5. Surface `OpenBrowser` errors in the status bar**
  - Not a security bug — but if the user's `xdg-open` handler is
    hijacked or misconfigured, the silent `_ = auth.OpenBrowser(u)`
    in `ui/app.go` gives them no signal. Show the error in the status
    bar.
