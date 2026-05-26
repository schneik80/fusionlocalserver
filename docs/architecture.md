# Architecture

fusionlocalserver is a single-binary Go application that authenticates with Autodesk Platform Services (APS) and browses the Manufacturing Data Model hierarchy. It ships **two front ends over one shared core**:

- **TUI (default)** — a reactive Bubble Tea three-column terminal browser (package `ui/`).
- **Server (`-server`)** — an HTTP service (package `server/`) that exposes a JSON REST API under `/api/*` and serves an embedded React/MUI single-page web UI built from `web/`.

Both front ends sit on the same UI-agnostic layers — `api/` (APS Manufacturing Data Model GraphQL client), `auth/` (OAuth PKCE + cached tokens), `config/`, and `pins/`.

---

## System Context

```mermaid
C4Context
    title fusionlocalserver — System Context

    Person(user, "Designer / Engineer", "Autodesk account holder with access to at least one Fusion Team hub")
    Person(lan_user, "LAN web user", "Browses the web UI over the LAN. There is no auth gate — they act as the server's APS identity.")

    System(app, "fusionlocalserver", "Single Go binary with two front ends: a terminal UI (default) and an HTTP server (-server) exposing a JSON API + embedded React/MUI web UI. Both reuse one cached APS identity.")

    System_Ext(aps_auth, "APS Authentication v2", "OAuth 2.0 authorization server. Issues access and refresh tokens via PKCE 3-legged flow.")

    System_Ext(aps_mfg, "APS Manufacturing Data Model", "GraphQL API (v2). Exposes hubs, projects, folders, items, version history, thumbnails, and physical properties for Fusion designs.")

    System_Ext(browser, "System Default Browser", "Used once during first login (on the host) to complete the OAuth consent page. Not required after the token is cached.")

    System_Ext(fusion, "Fusion Desktop", "Optional. Provides a local MCP server (http://127.0.0.1:27182/mcp) used by the TUI to open and insert documents in the running app.")

    SystemDb_Ext(fs, "Local Filesystem", "~/.config/fusionlocalserver/ — config.json (client ID), tokens.json (access + refresh tokens), pins-<hub>.json, and server.json (web-server port).")

    Rel(user, app, "Navigates the TUI with keyboard")
    Rel(lan_user, app, "Browses the web UI over the LAN", "HTTP")
    Rel(app, aps_auth, "PKCE OAuth login + token refresh", "HTTPS POST")
    Rel(app, aps_mfg, "GraphQL queries", "HTTPS POST")
    Rel(app, fs, "Reads config, reads/writes tokens, pins, server.json")
    Rel(app, browser, "Opens auth URL on first login", "OS exec")
    Rel(app, fusion, "JSON-RPC tool calls (open / insert document) — TUI only", "HTTP")
    Rel(browser, aps_auth, "Redirects to 127.0.0.1:7879/callback (loopback only)")
```

---

## Container Diagram

`main.go` parses flags and dispatches to one of two front ends — `runTUI` (default) or `server.Run` (`-server`) — both wired over the same shared `config` / `auth` / `api` / `pins` packages.

```mermaid
C4Container
    title fusionlocalserver — Containers

    Person(user, "TUI user")
    Person(web_user, "Web user (LAN)")

    Container_Boundary(app, "fusionlocalserver (single binary)") {
        Component(main, "main", "Go — main.go", "Entry point. Parses flags; runs the TUI by default or the HTTP server with -server. Loads config; owns the TUI panic-log wrapper.")

        Component(ui, "ui", "Go package", "Default mode. BubbleTea Model/Update/View. Three-column ranger-style browser with optional fourth details column, pins overlay, themes, About/debug overlays.")

        Component(server, "server", "Go package", "-server mode. HTTP service: JSON REST API under /api/*, plus an embedded React/MUI SPA. TokenManager holds one APS identity; thumbnail cache + image proxy; runtime port rebind. No TUI dependencies.")
        Component(webui, "web (React/MUI SPA)", "TypeScript / Vite", "Single-page app built from web/ into server/webdist and embedded via -tags embed_ui. Browser columns, details panel (metadata + thumbnail + tabs), pins + settings dialogs.")

        Component(config, "config", "Go package", "Three-layer config loader: env vars → config.json → build-time linker default. Resolves client ID and APS region.")
        Component(auth, "auth", "Go package", "OAuth 2.0 PKCE flow. Generates verifier/challenge, opens browser, runs local callback server, exchanges/refreshes tokens.")
        Component(api, "api", "Go package", "Typed GraphQL client. Cursor-paginated hierarchy queries, item details, refs, async assembly classification, thumbnails, physical properties.")
        Component(pins, "pins", "Go package", "Per-hub bookmark storage. Load/Save scoped by sanitized hub ID; MigrateLegacy promotes the pre-hub-scoping single-file pins.json on first run.")
        Component(fusion, "fusion", "Go package", "JSON-RPC client for the running Fusion desktop's local MCP server (open and insert document tools). Used by the TUI only.")
    }

    System_Ext(aps_auth, "APS Auth v2", "https://developer.api.autodesk.com/authentication/v2")
    System_Ext(aps_gql, "APS MFG GraphQL v2", "https://developer.api.autodesk.com/mfg/graphql")
    System_Ext(fusion_mcp, "Fusion MCP", "http://127.0.0.1:27182/mcp")
    SystemDb_Ext(fs, "~/.config/fusionlocalserver/")

    Rel(main, config, "Loads config")
    Rel(main, ui, "runTUI: creates Model, runs program")
    Rel(main, server, "server.Run when -server")

    Rel(user, ui, "Keyboard")
    Rel(web_user, webui, "Browser over LAN", "HTTP")
    Rel(webui, server, "fetch /api/* (same origin)", "HTTP/JSON")

    Rel(ui, auth, "Triggers login / token check")
    Rel(ui, api, "Issues data queries")
    Rel(ui, pins, "Load(hubID) / Save(hubID, pins)")
    Rel(ui, fusion, "Open / insert document via MCP")

    Rel(server, auth, "TokenManager bootstrap + refresh")
    Rel(server, api, "Issues data queries on behalf of web clients")
    Rel(server, pins, "Load/Save")

    Rel(auth, aps_auth, "PKCE token exchange + refresh", "HTTPS")
    Rel(auth, fs, "Persists tokens.json", "os.WriteFile 0600")
    Rel(config, fs, "Reads config.json", "os.ReadFile")
    Rel(pins, fs, "Reads/writes pins-<hubID>.json", "os.WriteFile 0600")
    Rel(server, fs, "Reads/writes server.json (port)", "os.WriteFile 0600")
    Rel(api, aps_gql, "GraphQL POST", "HTTPS")
    Rel(fusion, fusion_mcp, "JSON-RPC", "HTTP")
```

### Server mode specifics

- **Bind address.** Binds `0.0.0.0:8080` by default, so the web UI is reachable from other machines on the LAN. **There is no auth gate** — anyone who can reach the address browses as the server's single cached APS identity. Startup logs a warning and the reachable `http://<lan-ip>:8080` URLs.
- **Runtime-configurable port.** When `-addr` was not given explicitly (and not in `-dev` mode), the listen port is owned by the server and persisted in `~/.config/fusionlocalserver/server.json`. `POST /api/settings/port` validates and saves a new port, then an in-process listener rebind drops the old listener and binds the new one without restarting the process.
- **Embedded SPA vs. stub.** The React/MUI app is embedded only when built with `-tags embed_ui` (`server/static_embed.go`); a plain `go build` compiles `server/static_stub.go`, which serves a small "not built yet" shell. In `-dev` mode the static handler instead reverse-proxies non-`/api` requests to the Vite dev server for HMR.
- **Thumbnail cache + image proxy.** A bounded, shared in-memory cache (`thumbCache`) holds thumbnail status/URLs and PNG bytes keyed by component-version id. It is warmed in the background off the per-row classify probe, and `/api/items/thumbnail/image` streams the bytes same-origin so browsers never fetch the cross-origin APS signed URL directly.
- **Physical properties.** `/api/items/properties` returns a design's mass/geometry properties (v2 API); generation is async, so the web UI polls until COMPLETED.

---

## Component Diagram — `ui` package

```mermaid
C4Component
    title ui package (TUI mode) — Internal Components

    Component(app, "app.go", "BubbleTea Model", "Root state machine. Owns the Model struct, Init/Update/View lifecycle, all message handlers, navigation logic, and renderer orchestration.")
    Component(keys, "keys.go", "keyMap struct", "Declares all key bindings using charmbracelet/bubbles key package. Single keyMap var consumed by app.go Update loop.")
    Component(styles, "styles.go", "Theme + Lipgloss styles", "Defines colorTheme struct, three theme palettes (Rust, Mono, System), applyTheme() that rebuilds every Lipgloss style var, and cycleTheme() called on [t] keypress.")

    Rel(app, keys, "Reads key bindings")
    Rel(app, styles, "Calls cycleTheme(), reads style vars")
```

---

## Package Dependency Graph

`main` dispatches to either `ui` (TUI) or `server` (HTTP). Both depend on the same shared `api` / `auth` / `config` / `pins` layers; only the TUI depends on `fusion`.

```mermaid
graph TD
    main --> config
    main --> ui
    main --> server

    ui --> api
    ui --> auth
    ui --> pins
    ui --> fusion

    server --> api
    server --> auth
    server --> pins
    server --> config

    auth --> config
    api --> config
    pins --> config

    subgraph stdlib
        net/http
        crypto/sha256
        encoding/base64
        crypto/rand
        os
    end

    auth --> stdlib
    api --> stdlib
    pins --> stdlib
    server --> stdlib

    subgraph charm["Charm.sh (external)"]
        bubbletea["charmbracelet/bubbletea"]
        bubbles["charmbracelet/bubbles"]
        lipgloss["charmbracelet/lipgloss"]
    end

    ui --> bubbletea
    ui --> bubbles
    ui --> lipgloss

    subgraph web["web/ (React/MUI SPA, embedded into server)"]
        spa["React + MUI + Vite\n→ server/webdist (embed_ui)"]
    end

    server --> spa
    spa -. "fetch /api/*" .-> server
```

The Go server depends on the standard library only (`net/http`, `log/slog`, `embed`, …) — no third-party Go web framework. The `web/` SPA's dependencies (React, MUI, Vite) are managed by npm and bundled into `server/webdist`, which is embedded into the binary at build time.

---

## Data Flow

The two front ends drive the same `api` package but with different glue: the TUI dispatches `tea.Cmd` goroutines from `Update`, while the server runs each `/api/*` request through a handler.

### Server mode — browser request to JSON

```mermaid
sequenceDiagram
    participant B as Browser (React/MUI SPA)
    participant MW as Middleware (recover → log → dev CORS)
    participant H as Handler (server/handlers*.go)
    participant TM as TokenManager
    participant API as api package
    participant APS as APS GraphQL

    B->>MW: GET /api/folders/contents?... (same origin)
    MW->>H: ServeHTTP
    H->>TM: Token(ctx)
    Note over TM: cached token still Valid?<br/>refresh transparently if not<br/>(401 if no refresh token)
    TM-->>H: access token
    H->>API: GetFolders / GetItems / GetItemDetails / …
    API->>APS: POST /mfg/graphql
    APS-->>API: JSON response
    API-->>H: typed result
    H-->>B: writeJSON(DTO)
    Note over B: TanStack Query caches the<br/>result and renders the column
```

The SPA is served same-origin from the embedded build (or, with `-dev`, reverse-proxied to Vite), so no CORS is needed in production. Unmatched `/api/*` paths return a JSON 404; all other paths fall through to the SPA shell (`index.html`) so client-side deep links resolve.

### TUI mode — From Keypress to Screen

#### Hierarchy navigation (arrow keys, Enter on a folder)

```mermaid
sequenceDiagram
    participant OS as Terminal / OS
    participant BT as BubbleTea runtime
    participant Update as Model.Update
    participant Cmd as tea.Cmd (goroutine)
    participant API as api package
    participant APS as APS GraphQL

    OS->>BT: KeyMsg (e.g. →)
    BT->>Update: Update(KeyMsg{→})
    Update->>Update: navigateRight()
    Update->>Cmd: loadItemsCmd(token, hubID, folderID)
    Cmd->>API: GetItems(ctx, token, hubID, folderID)
    Note over API: gqlQuery: up to 3 attempts<br/>retry on root-level errorType:UNKNOWN<br/>or HTTP 408/429/5xx
    API->>APS: POST /mfg/graphql
    APS-->>API: JSON response
    API-->>Cmd: []NavItem
    Cmd-->>BT: contentsLoadedMsg{items}
    BT->>Update: Update(contentsLoadedMsg)
    Update->>Update: populate cols[1], maybeLoadDetails()
    BT->>BT: View() → render to terminal
```

#### Tab activation (1-4)

```mermaid
sequenceDiagram
    participant U as User
    participant Update as Model.Update
    participant Cmd as tea.Cmd
    participant API as api package

    U->>Update: KeyMsg{2}
    Update->>Update: selectTab(tabUses)
    Update->>Update: maybeLoadActiveTab()
    alt cache hit (usesCache[key])
        Update-->>U: render cached rows
    else cache miss
        Update->>Cmd: loadUsesCmd / loadDrawingUsesCmd
        Cmd->>API: GetOccurrences | GetDrawingSource
        API-->>Cmd: []ComponentRef
        Cmd-->>Update: usesLoadedMsg{key, items, err}
        Update->>Update: usesCache[key] = items
        Update-->>U: render rows
    end
```

#### Show in Location (Enter / double-click on a tab row)

See [`docs/navigation.md`](navigation.md#tab-cursor-and-show-in-location) for the user-visible flow; the API-side detail is in [`docs/api.md`](api.md#getitemlocation--show-in-location).

#### Async assembly-vs-part classification

After the Contents column loads, each DesignItem is enriched with an "assembly" / "part" subtype derived from whether its tipRootComponentVersion has any sub-component occurrences. The probe is dispatched in parallel under a `tea.Batch` and capped at 8 concurrent calls by a package-level semaphore in `api/classify.go`; each result flows back as an `itemClassifiedMsg` that mutates the matching row's `Subtype` in place. A `contentsGen` counter on the Model is incremented every time the Contents slice is replaced (folder drill, hub switch, project switch, refresh, recovery), and stale `itemClassifiedMsg`s whose gen no longer matches are dropped — late refinements can never stamp state onto a folder the user has already left.

```mermaid
sequenceDiagram
    participant Update as Model.Update
    participant Batch as tea.Batch (N cmds)
    participant Sem as classifySem (cap 8)
    participant APS as APS GraphQL

    Note over Update: contentsLoadedMsg lands<br/>m.contentsGen++<br/>gen = m.contentsGen
    Update->>Batch: classifyContentsCmd(items, gen)<br/>one cmd per DesignItem
    par 8 concurrent
        Batch->>Sem: classifySem <- {}
        Sem-->>Batch: slot acquired
        Batch->>APS: componentVersion(cvid).occurrences(limit:1)
        APS-->>Batch: { results: [] | [{...}] }
        Batch-->>Update: itemClassifiedMsg{gen, itemID, isAssembly}
    end
    Update->>Update: if msg.gen != m.contentsGen: drop
    Update->>Update: else: m.cols[colContents][i].Subtype = "assembly"|"part"
    Note over Update: View() re-renders that row<br/>with the new suffix
```

---

## Performance Optimisations

The browser View() runs at spinner rate (~10 Hz) and re-renders every visible row each frame, so a few targeted caches keep navigation snappy on large hubs:

- **`detailsCache map[string]*api.ItemDetails`** — `GetItemDetails` results are memoised by item ID for the lifetime of the session. Item details are immutable for a given ID (a save creates a new version with a new tip-version number, but the item ID is stable), so arrowing back over a previously-visited item is served synchronously without an API call. Refresh (`r`) and hub re-selection clear the map to force re-fetch.
- **Per-tab caches** — `usesCache`, `whereUsedCache`, and `drawingsCache` each memoise their respective queries. Cache keys differ per relationship: Uses/WhereUsed for designs key on the tip root component-version id; Drawings keys on the design's lineage URN; Uses for drawings keys on the drawing's lineage URN. Hub change and refresh clear all of them; arrowing between items preserves them so a "scan Where Used across these designs" workflow doesn't refetch unchanged data.
- **`styleCache`** — Lipgloss styles are value types but their rules clone on each chained `.Width(...).Foreground(...)` call. The width-applied variants used in `renderColumn` / `viewDetailsColumn` are precomputed and rebuilt only when terminal size or theme changes. The cache is shared by pointer because Bubble Tea passes the Model by value to View(); a local mutation on a copy would not persist. The rendered detail-panel lines are also cached and keyed on `m.details`'s pointer + width + theme version.
- **Parallel project-contents fetch** — `loadProjectContentsCmd` issues `foldersByProject` and `itemsByProject` concurrently via `sync.WaitGroup` rather than sequentially. Wall-clock latency drops to roughly the slower of the two queries.
- **Bounded-parallelism assembly classifier** — `api.ClassifyAssembly` calls run under a package-level `classifySem` buffered channel (size 8). A `tea.Batch` of 50 cmds dispatched from `contentsLoadedMsg` translates into at most 8 in-flight HTTPS round-trips against the gateway at a time; the rest queue on the semaphore. Wall-clock for a 50-item folder is roughly `ceil(N/8) × ~150 ms ≈ 1 s` vs the ~5 s a serial extended `itemsByFolder` query would cost.
- **Contents generation guard** — `m.contentsGen` is incremented whenever the Contents slice is replaced. Async `itemClassifiedMsg`s carry the gen they were dispatched under; mismatches are dropped instead of mutating the new selection. This is the cancellation primitive that makes the classifier safe to fire-and-forget without per-cmd context cancellation plumbing.

---

## Resilience — APS gateway flakiness

The APS Manufacturing Data Model GraphQL gateway (`/mfg/graphql`) intermittently returns `code:NOT_FOUND, errorType:UNKNOWN` for hub URNs it just successfully enumerated via the `hubs` query — the same access token, same hub ID, and same query body succeed and fail within seconds. The failure can occur on the very first paginated request (no cursor involved), so it is not a cursor-encoding issue, and reproduces against both the shared `*http.Client` and `http.DefaultClient`, so it is not a connection-state issue.

`gqlQuery` (in `api/client.go`) wraps a single-shot `gqlQueryOnce` in a 3-attempt retry loop with backoffs `0 → 500 ms → 1.5 s`. Retry triggers are narrow:

- Transport errors and HTTP `408` / `429` / `5xx` (server / network).
- GraphQL `errors[]` carrying `extensions.errorType: "UNKNOWN"` (gateway's marker for intermittent upstream faults).

HTTP `401` and concrete-typed GraphQL errors (`VALIDATION`, `BAD_USER_INPUT`, etc.) are surfaced immediately without retry. Total worst-case added latency is ~2 s, well inside the 30 s context that wraps every nav `tea.Cmd`. See [`docs/api.md`](api.md#error-handling-and-retry) for the decision-tree diagram. The full repro trace and defect-report template live outside the repo at `~/Documents/aps-mfg-graphql-flakiness.md` for filing with APS.

---

## Test Strategy

A three-layer test pyramid lives alongside the code it exercises. The full strategy, layer-by-layer details, naming conventions, and instructions for adding new tests live in [`docs/testing.md`](testing.md).

| Layer | What it covers |
|-------|----------------|
| **L1 — Pure unit** | Config parsing, OAuth helpers, GraphQL response decode, UI helpers (no I/O) |
| **L2 — HTTP integration** | OAuth, `gqlQuery`, MCP JSON-RPC against `httptest.Server` fakes in `internal/testutil` |
| **L3 — TUI flow** | Bubble Tea `Update(msg)` / `View()` end-to-end through `tea.Cmd` → mocked APS |

The full `go test -race ./...` suite finishes in under five seconds. CI (`.github/workflows/test.yml`) runs `go vet` + `go test -race -count=1 -coverprofile` on every pull request and push to `main`; locally `make check` does the same.

---

## File Layout

```
fusionlocalserver/
├── main.go                  Entry point. -server runs the HTTP server; default runs the TUI.
│                            TUI path adds a deferred recover() → ~/.config/fusionlocalserver/panic.log
│
├── config/
│   └── config.go            Config struct, Load(), Dir(), Path(), DefaultClientID
│
├── auth/
│   ├── oauth.go             Login(), Refresh(), OpenBrowser(), PKCE helpers
│   ├── callback.go          WaitForCallback() — local HTTP server bound to 127.0.0.1:7879
│   └── tokens.go            LoadTokens(), SaveTokens(), TokenData.Valid()
│
├── api/
│   ├── client.go            gqlQuery() retry loop + gqlQueryOnce(), NavItem (incl. ComponentVersionID
│   │                        + Subtype), SetRegion(), SetGraphqlEndpointForTesting()
│   ├── queries.go           Hierarchy queries — GetHubs/Projects/Folders/Items; allPages() pagination;
│   │                        items queries pull tipRootComponentVersion.id inline for designs so the
│   │                        async classifier can probe occurrences without a second round-trip
│   ├── classify.go          ClassifyAssembly(cvid) + classifySem semaphore (cap 8 concurrent)
│   ├── details.go           GetItemDetails(), ItemDetails, VersionSummary, parseTime()
│   ├── refs.go              Cross-reference queries: GetOccurrences, GetWhereUsed,
│   │                        GetDrawingsForDesign, GetDrawingSource (Uses/WhereUsed/Drawings tabs)
│   ├── locate.go            GetItemLocation — project + folder ancestry walk for Show in Location
│   ├── download.go          RequestSTEPDerivative(), DownloadFile(), StepDownloadPath()
│   └── debug.go             dbgLog (in-memory ring + debug.log file + stderr if redirected),
│                            DebugLines(), DebugEnabled(), DebugLogPath()
│
├── pins/
│   └── pins.go              Hub-scoped bookmark storage (~/.config/fusionlocalserver/pins-<hubID>.json);
│                            Load(hubID), Save(hubID, pins), MigrateLegacy() (one-shot pins.json split
│                            into per-hub files), sanitizeHubID() for cross-platform filenames
│
├── fusion/
│   └── mcp.go               Fusion desktop MCP client (open / insert document) — TUI only
│
├── server/                  -server mode: HTTP JSON API + embedded React/MUI SPA
│   ├── server.go            Run(), listener (re)bind loop, LAN-URL/open-network warning, resolveAddr
│   ├── routes.go            ServeMux: /api/* JSON routes + SPA catch-all + middleware chain
│   ├── handlers*.go         Per-endpoint handlers (nav, refs, props, pins, settings, thumbnail, stub)
│   ├── dto.go               JSON DTOs returned to the web client
│   ├── token.go             TokenManager — one cached APS identity, transparent refresh
│   ├── settings.go          server.json (runtime listen port) load/save
│   ├── thumbcache.go        Bounded in-memory thumbnail status/URL/bytes cache (shared, LRU)
│   ├── middleware.go        recoverPanic / logRequest / dev-only CORS
│   ├── static.go            SPA handler: embedded build (prod) or Vite reverse-proxy (-dev)
│   ├── static_embed.go      //go:build embed_ui — go:embed server/webdist
│   └── static_stub.go       //go:build !embed_ui — "not built yet" shell
│
├── web/                     React/MUI single-page UI (Vite, TypeScript)
│   ├── src/                 App, BrowserColumns, DetailsPanel, Pins/Settings dialogs, api client
│   ├── vite.config.ts       Builds into ../server/webdist (gitignored build output)
│   └── package.json
│
├── ui/
│   ├── app.go               Model, Init, Update, View; nav + tab + Show-in-Location orchestration;
│   │                        pins overlay (statePins); async classify dispatch + contentsGen guard
│   ├── keys.go              keyMap, keys var (WASD/arrows nav, 1-4 tab select, Enter activate,
│   │                        Shift+P pin toggle, P pins overlay, Delete remove pin)
│   └── styles.go            colorTheme, themes[], applyTheme(), cycleTheme(), tab strip styles
│
├── cmd/
│   ├── probe-assembly/      One-shot diagnostic — runs the extended itemsByProject query against a
│   │                        live hub and prints assembly/part distribution. Used to validate the
│   │                        classifier schema and per-row cost. Safe to delete after decisions land.
│   └── screenshot/          Generates the README screenshot from a scripted Model state
│
├── internal/testutil/       Shared test fakes — GraphQLServer, NewMCPServer
│   ├── graphql.go           In-process APS GraphQL fake (httptest.Server)
│   └── mcp.go               In-process Fusion MCP JSON-RPC fake
│
├── docs/                    User + developer documentation
│   ├── api.md               GraphQL queries, retry behaviour, classifier, debug logging
│   ├── architecture.md      This file — C4 diagrams, packages, data flow
│   ├── authentication.md    OAuth PKCE flow
│   ├── debugging.md         End-user defect-submission guide
│   ├── development.md       Build, release, dependencies
│   ├── navigation.md        Browser, tabs, Show-in-Location, pins, mouse, themes
│   ├── server-webui-plan.md -server mode + React/MUI web UI design record
│   └── testing.md           Three-layer test strategy + how to extend
│
├── SECURITY-TODO.md         Pending security follow-ups (M1, M3, L1–L5)
├── .goreleaser.yaml         Build + release pipeline (goreleaser v2)
└── .github/workflows/
    ├── release.yml          GoReleaser + signed/notarized macOS .pkg on tag push
    └── test.yml             go vet + go test -race on every PR and push to main
```
