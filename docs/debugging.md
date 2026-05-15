# Debugging & Reporting Bugs

This page is for users who hit a problem with FusionDataCLI and want to report it. It explains how to capture enough diagnostic information that the maintainers can reproduce and fix the issue without a long back-and-forth.

If you're a contributor looking for the test-suite architecture, see [`testing.md`](testing.md). If you want internals on the GraphQL client, retry behaviour, or the in-memory log, see [`api.md`](api.md).

---

## Where logs live

Two files appear under `~/.config/fusiondatacli/` while the app is running:

| File | When written | Contents |
|---|---|---|
| `debug.log` | Whenever `APSNAV_DEBUG=1` is set. Truncated each session. Mode `0600`. | Full GraphQL request and response bodies, retry decisions, browser-open events. **No tokens, no Authorization headers.** |
| `panic.log` | Only if the app crashes unrecoverably. Appended each crash (so multiple crashes accumulate). Mode `0600`. | Crash message + full goroutine stack trace from the moment the app died. |

Both files are local to your machine; nothing is sent anywhere automatically. You decide what (if anything) to share when you file a defect.

On Linux and macOS the path is exactly `~/.config/fusiondatacli/`. On Windows it's `%USERPROFILE%\.config\fusiondatacli\` (the same shape — the app uses `os.UserHomeDir()` directly rather than the OS-specific config dir).

---

## Enabling debug mode

```sh
APSNAV_DEBUG=1 fusiondatacli
```

Three things happen:

1. **In-memory log** — press `?` from the main browser at any time to scroll the rolling log of the last 500 entries. The first line of the overlay shows the path of the on-disk log file.
2. **`debug.log` file** — written under your config dir, truncated each session start. Tail it from another terminal:
   ```sh
   tail -f ~/.config/fusiondatacli/debug.log
   ```
3. **Stderr mirror** — only when stderr is redirected. If you launch with `APSNAV_DEBUG=1 fusiondatacli 2> capture.log`, every entry is also written to `capture.log`. When stderr is your terminal, the mirror is suppressed automatically so it doesn't smear the alternate-screen render.

The debug log is silent unless `APSNAV_DEBUG=1` is set. There is no performance cost when it's off.

---

## What's safe to share

Everything written to `debug.log` is already redacted of sensitive material:

- **Authorization headers are never logged.** The bearer token never appears in any log file.
- **Token files (`tokens.json`)** are in the same directory but are not part of the debug log. Don't attach them.
- **GraphQL response bodies** are logged verbatim. They contain item names, project names, hub names, file sizes, and other metadata. If your project names themselves are confidential, redact those before sharing.

`panic.log` contains a Go stack trace and runtime details. It does not contain tokens or user data — only program flow. It's safe to attach in full.

---

## Filing a bug — checklist

When you open an issue at <https://github.com/schneik80/FusionDataCLI/issues>, please include:

1. **Version** — `fusiondatacli` shows the version on the About screen (`shift+a`). Or check `which fusiondatacli && stat $(which fusiondatacli)`.
2. **OS and terminal** — e.g. "macOS 14.5, Terminal.app" or "Linux Fedora 40, Ghostty 1.0".
3. **What you did** — three to five lines is plenty. "Logged in, picked the *IMA* hub, selected the *RC* project, drilled into *Designs*, pressed `2` to load the Uses tab on a 200-component assembly."
4. **What you expected vs. what happened.**
5. **The relevant slice of `debug.log`** if you have one — usually the last few hundred lines around the failure are enough. The very first lines of `debug.log` always show the GraphQL endpoint and your hub list, which is helpful context.
6. **`panic.log`** in full if the app crashed.

If the issue is a slow or flaky API call, we'll usually want to see at least one full `REQUEST` / `RESPONSE` pair plus any `RETRY` lines that fired.

A good bug-report skeleton:

```
Version: v4.0.0
OS: macOS 14.5 (Apple Silicon)
Terminal: iTerm2 3.5

Steps:
1. APSNAV_DEBUG=1 fusiondatacli
2. Selected hub "IMA"
3. Selected project "RC"
4. Drilled into "Mechanical Drawings"
5. Selected design "2021-02"
6. Pressed "4" to load the Drawings tab

Expected: list of drawings made from the design.
Actual: tab shows "Error: Query point value 23066 exceeds maximum...".

debug.log excerpt: <attached>
```

---

## Common log signatures and what they mean

These are the patterns that show up most often in `debug.log`. Recognising them helps you tell whether you've hit something we already know about.

### `RETRY attempt=N delay=… lastErr=…`

The retry layer kicked in. v3.1.1+ has a 3-attempt retry loop for transient APS gateway flakiness — see [`api.md`](api.md#error-handling-and-retry). Seeing one or two retries is fine; seeing the loop bottom out with `APS GraphQL flaky after 3 attempts: …` after every action is a real bug worth filing.

### `code:NOT_FOUND, errorType:UNKNOWN, service:cw`

The APS Manufacturing Data Model gateway returns this intermittently for hub URNs it just successfully enumerated. It's an upstream bug, not yours. The retry layer absorbs it for root-level paths automatically. There's a defect-report template for filing this with APS at `~/Documents/aps-mfg-graphql-flakiness.md` (kept outside the repo so you can pick it up to attach to a support case).

### `Query point value <N> exceeds maximum allowed query point value 1000`

The GraphQL gateway rejected the request as too expensive. If this comes from a query that *we* sent, that's a real bug — please file it with the `REQUEST` line so we can shrink the query. The drawings tab in particular has tight limits because of this cap; see [`api.md`](api.md#getdrawingsfordesign--drawings-tab-on-a-designitem) for the trade-off.

### `unauthorized (HTTP 401) — token may be expired or lacks scope/entitlement`

Your access token is stale, was revoked, or lacks the `data:read` / `user-profile:read` scope. Re-running the app prompts a fresh login on launch. If a fresh login still produces 401, the APS app registration may be misconfigured — see [`authentication.md`](authentication.md).

### `GraphQL partial errors (kept data): …`

A response came back with both useful data and field-level errors. The client kept the data and surfaced the errors via this log line. Typically harmless — common when one row in a list references a deactivated or deleted record. If a tab unexpectedly shows missing fields, the corresponding `GraphQL partial errors` line in the log identifies which row.

### `OPEN_BROWSER <url>`

The app handed `<url>` to your OS browser handler (for `u`-key actions). If the page errors out (e.g. Autodesk's "WEB SESSION INVALID"), this line is the URL to check or copy manually.

### `ClassifyAssembly` query bursts after a folder load

When the Contents column loads, the app dispatches up to 50 small `componentVersion(...).occurrences(pagination: { limit: 1 })` queries in parallel, capped at 8 concurrent in flight by a semaphore. In `debug.log` this shows up as a burst of similar requests right after the `itemsByFolder` / `itemsByProject` response. Each one returns a single-result list (or empty) and refines a Contents-column row's icon to `· asm` or `· part`. If those queries 401 or get rate-limited, the corresponding rows silently keep their generic design icon — failures here never block navigation. See [`api.md`](api.md#classifyassembly--async-partassembly-subtype) for the query and the concurrency cap, and [`architecture.md`](architecture.md#async-assembly-vs-part-classification) for the cancellation pattern (a `contentsGen` counter on the Model drops late responses when the user has navigated away).

---

## When the app crashes

If FusionDataCLI exits unexpectedly:

1. Re-run with `APSNAV_DEBUG=1` so the debug log captures the actions leading up to the crash.
2. Reproduce the crash.
3. Attach **both** `debug.log` and `panic.log` to your bug report.

`panic.log` is appended-to, not truncated, so multiple crashes accumulate. If you've been debugging for a while and the file is large, the most relevant entry is the *last* `=== panic at <timestamp> ===` block.

---

## Disabling debug mode

Just don't set the environment variable:

```sh
fusiondatacli           # quiet — no debug logging anywhere
```

In quiet mode no log files are written and the in-memory ring buffer stays empty. The `?` debug overlay opens but reads "Debug mode is off. Re-launch with APSNAV_DEBUG=1 to enable logging."

---

## Privacy and security

- All log files are written with mode `0600` (owner-only read/write).
- Token files (`tokens.json`) are separate from the debug log and are never read by the logger.
- The Authorization header is filtered out of every request body before it's logged.
- Stderr mirroring requires explicit redirection; the app never writes to stderr while bound to a TTY.
- Nothing is uploaded automatically. Logs stay on your machine until you choose to share them.

If you find a bug that *does* leak tokens or other sensitive data into a log, please report it privately rather than filing a public issue — see `SECURITY-TODO.md` and the security-fix history (PR #1) for how the project handles such reports.
