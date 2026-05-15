# Architecture

FusionDataCLI is a single-binary terminal application written in Go. It authenticates with Autodesk Platform Services (APS), then renders a live three-column browser over the Manufacturing Data Model hierarchy using a reactive TUI loop.

---

## System Context

```mermaid
C4Context
    title FusionDataCLI — System Context

    Person(user, "Designer / Engineer", "Autodesk account holder with access to at least one Fusion Team hub")

    System(app, "FusionDataCLI", "Cross-platform terminal browser for APS Manufacturing Data Model. Runs entirely in the terminal — no GUI, no browser dependency after first login.")

    System_Ext(aps_auth, "APS Authentication v2", "OAuth 2.0 authorization server. Issues access and refresh tokens via PKCE 3-legged flow.")

    System_Ext(aps_mfg, "APS Manufacturing Data Model", "GraphQL API (v2). Exposes hubs, projects, folders, items, and version history for Fusion designs.")

    System_Ext(browser, "System Default Browser", "Used once during first login to complete the OAuth consent page. Not required after token is cached.")

    System_Ext(fusion, "Fusion Desktop", "Optional. Provides a local MCP server (http://127.0.0.1:27182/mcp) used to open and insert documents in the running app.")

    SystemDb_Ext(fs, "Local Filesystem", "~/.config/fusiondatacli/ — stores config.json (client ID) and tokens.json (access + refresh tokens).")

    Rel(user, app, "Navigates with keyboard")
    Rel(app, aps_auth, "PKCE OAuth login + token refresh", "HTTPS POST")
    Rel(app, aps_mfg, "GraphQL queries", "HTTPS POST")
    Rel(app, fs, "Reads config, reads/writes tokens")
    Rel(app, browser, "Opens auth URL on first login", "OS exec")
    Rel(app, fusion, "JSON-RPC tool calls (open / insert document)", "HTTP")
    Rel(browser, aps_auth, "Redirects to 127.0.0.1:7879/callback (loopback only)")
```

---

## Container Diagram

```mermaid
C4Container
    title FusionDataCLI — Containers

    Person(user, "User")

    Container_Boundary(app, "FusionDataCLI (single binary)") {
        Component(main, "main", "Go — main.go", "CLI entry point. Loads config, wires packages, starts BubbleTea event loop with alternate-screen mode.")
        Component(config, "config", "Go package", "Three-layer config loader: env vars → config.json → build-time linker default. Resolves client ID and APS region.")
        Component(auth, "auth", "Go package", "Full OAuth 2.0 PKCE flow. Generates verifier/challenge, opens browser, runs local callback server, exchanges code for tokens, saves and refreshes token data.")
        Component(api, "api", "Go package", "Typed GraphQL client. Executes cursor-paginated queries for hubs, projects, folders, items, item details, refs, and async assembly-vs-part classification.")
        Component(pins, "pins", "Go package", "Per-hub bookmark storage. Load/Save scoped by sanitized hub ID; MigrateLegacy promotes the pre-hub-scoping single-file pins.json on first run.")
        Component(fusion, "fusion", "Go package", "JSON-RPC client for the running Fusion desktop's local MCP server (open and insert document tools).")
        Component(ui, "ui", "Go package", "BubbleTea Model/Update/View. Three-column ranger-style browser with optional fourth details column. Pins overlay. Three color themes. About and debug overlays.")
    }

    System_Ext(aps_auth, "APS Auth v2", "https://developer.api.autodesk.com/authentication/v2")
    System_Ext(aps_gql, "APS MFG GraphQL v2", "https://developer.api.autodesk.com/mfg/graphql")
    System_Ext(fusion_mcp, "Fusion MCP", "http://127.0.0.1:27182/mcp")
    SystemDb_Ext(fs, "~/.config/fusiondatacli/")

    Rel(main, config, "Loads config")
    Rel(main, ui, "Creates Model, runs program")
    Rel(ui, auth, "Triggers login / token check")
    Rel(ui, api, "Issues data queries")
    Rel(ui, pins, "Load(hubID) / Save(hubID, pins)")
    Rel(ui, fusion, "Open / insert document via MCP")
    Rel(auth, aps_auth, "PKCE token exchange + refresh", "HTTPS")
    Rel(auth, fs, "Persists tokens.json", "os.WriteFile 0600")
    Rel(config, fs, "Reads config.json", "os.ReadFile")
    Rel(pins, fs, "Reads/writes pins-<hubID>.json", "os.WriteFile 0600")
    Rel(api, aps_gql, "GraphQL POST", "HTTPS")
    Rel(fusion, fusion_mcp, "JSON-RPC", "HTTP")
```

---

## Component Diagram — `ui` package

```mermaid
C4Component
    title ui package — Internal Components

    Component(app, "app.go", "BubbleTea Model", "Root state machine. Owns the Model struct, Init/Update/View lifecycle, all message handlers, navigation logic, and renderer orchestration.")
    Component(keys, "keys.go", "keyMap struct", "Declares all key bindings using charmbracelet/bubbles key package. Single keyMap var consumed by app.go Update loop.")
    Component(styles, "styles.go", "Theme + Lipgloss styles", "Defines colorTheme struct, three theme palettes (Rust, Mono, System), applyTheme() that rebuilds every Lipgloss style var, and cycleTheme() called on [t] keypress.")

    Rel(app, keys, "Reads key bindings")
    Rel(app, styles, "Calls cycleTheme(), reads style vars")
```

---

## Package Dependency Graph

```mermaid
graph TD
    main --> config
    main --> ui
    ui --> api
    ui --> auth
    ui --> pins
    ui --> fusion
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

    subgraph charm["Charm.sh (external)"]
        bubbletea["charmbracelet/bubbletea"]
        bubbles["charmbracelet/bubbles"]
        lipgloss["charmbracelet/lipgloss"]
    end

    ui --> bubbletea
    ui --> bubbles
    ui --> lipgloss
```

---

## Data Flow — From Keypress to Screen

### Hierarchy navigation (arrow keys, Enter on a folder)

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

### Tab activation (1-4)

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

### Show in Location (Enter / double-click on a tab row)

See [`docs/navigation.md`](navigation.md#tab-cursor-and-show-in-location) for the user-visible flow; the API-side detail is in [`docs/api.md`](api.md#getitemlocation--show-in-location).

### Async assembly-vs-part classification

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
FusionDataCLI/
├── main.go                  Entry point + deferred recover(); writes ~/.config/fusiondatacli/panic.log
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
│   └── pins.go              Hub-scoped bookmark storage (~/.config/fusiondatacli/pins-<hubID>.json);
│                            Load(hubID), Save(hubID, pins), MigrateLegacy() (one-shot pins.json split
│                            into per-hub files), sanitizeHubID() for cross-platform filenames
│
├── fusion/
│   └── mcp.go               Fusion desktop MCP client (open / insert document)
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
│   └── testing.md           Three-layer test strategy + how to extend
│
├── SECURITY-TODO.md         Pending security follow-ups (M1, M3, L1–L5)
├── .goreleaser.yaml         Build + release pipeline (goreleaser v2)
└── .github/workflows/
    ├── release.yml          GoReleaser + signed/notarized macOS .pkg on tag push
    └── test.yml             go vet + go test -race on every PR and push to main
```
