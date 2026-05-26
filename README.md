# fusionlocalserver

[![test](https://github.com/schneik80/fusionlocalserver/actions/workflows/test.yml/badge.svg)](https://github.com/schneik80/fusionlocalserver/actions/workflows/test.yml)

A local server and web UI — plus a terminal browser — for the [Autodesk Platform Services (APS)](https://aps.autodesk.com) Manufacturing Data Model. Browse your Fusion hubs, projects, folders, and designs from a browser on your LAN or straight from the command line.

One binary, two front ends over the same data/auth core:

- **`fusionlocalserver -server`** — an HTTP server that exposes a JSON API and an embedded React/MUI web UI. Bind it to your LAN and browse Fusion data from any device.
- **`fusionlocalserver`** — a Bubble Tea terminal UI (the original interface).

Both share the same APS Manufacturing Data Model client, OAuth token cache, and pins, so they behave identically and reuse one sign-in.

## Quick start (web server)

```sh
echo "your-aps-client-id" > .aps-client-id   # one time; see "Building from source"
make run                                      # build the UI + binary, then serve
```

`make run` binds `0.0.0.0:8080` and logs every reachable URL at startup:

```
server starting  addr=0.0.0.0:8080 ...
reachable on the LAN  url=http://localhost:8080
reachable on the LAN  url=http://192.168.1.50:8080
```

Open one of those URLs in a browser. On first run the server completes the Autodesk sign-in (a browser window on the host); after that it reuses the cached token.

> ⚠️ **No auth gate.** Anyone who can reach the bound address browses as the server's APS identity. Run it on a trusted LAN only. A warning is logged whenever it binds a non-loopback address.

### Server flags & settings

| Flag | Default | Purpose |
|------|---------|---------|
| `-server` | off | Run the HTTP server instead of the TUI |
| `-addr` | `0.0.0.0:8080` | Bind address; overrides the configurable port |
| `-dev` | off | Serve the UI by reverse-proxying the Vite dev server (HMR) instead of the embedded build |

The listen **port is configurable at runtime** from the web UI's Settings dialog (persisted to `~/.config/fusionlocalserver/server.json`). Changing it restarts the listener in place; the page then reconnects on the new port. The port field is read-only when `-addr` was passed explicitly or in `-dev` mode.

### Web UI

The web UI recreates the three-column browser — **Projects │ Contents │ Details** — with a global header, a left rail (Hubs / Pins / Settings), and a clickable breadcrumb. Highlights:

- **Details panel** — the document's metadata (type, part number, material, version, dates…) is always shown beside its **thumbnail**; tabs add **History**, **Properties**, **Uses**, **Where Used**, and **Drawings**.
- **Thumbnails** are fetched once, cached server-side (shared across all clients), warmed in the background as you browse, and streamed same-origin — so opening a design is usually instant.
- **Properties** shows physical/mass properties (mass, volume, surface area, density, bounding box) from the v2 Manufacturing Data Model API.
- **Uses / Where Used / Drawings** rows are clickable: selecting one navigates the browser straight to that document.
- **Pins**, **Light/Dark/System theme**, and region are available from the rail and Settings.

## Terminal UI

Run with no flags for the keyboard/mouse terminal browser:

```sh
fusionlocalserver
```

![fusionlocalserver terminal UI](./docs/screenshot.png)

### Key bindings

| Key | Action |
|-----|--------|
| `↑` `↓` / `w` `s` | Move cursor |
| `→` `↵` / `d` | Enter folder / open details |
| `←` / `a` | Go back |
| `h` | Switch hub |
| `u` | Open selected document in browser (after details load) |
| `o` | Open in Fusion desktop (via Fusion MCP) |
| `i` | Insert into active Fusion design (via Fusion MCP) |
| `shift+d` | Download STEP file for the selected design |
| `shift+p` / `p` | Pin or unpin / open the pins overlay |
| `t` / `m` | Cycle theme / toggle mouse |
| `shift+a` / `r` / `?` / `q` | About / refresh / debug log / quit |

Details-pane tabs: `1` Details, `2` Uses, `3` Where Used, `4` Drawings. On a non-Details tab, `Enter` (or double-click) does **Show in Location** — jumps the Contents column to that row's project + folder + item.

### Fusion desktop, STEP, web links

- `o` / `i` drive the running Fusion desktop client via its local MCP server (`http://127.0.0.1:27182/mcp`), after verifying Fusion is on the same hub.
- `shift+d` requests a STEP derivative (`componentVersion(...).derivatives(outputFormat: STEP, generate: true)`), polls until `SUCCESS`/`FAILED`, and streams it to `~/Downloads/<design>-<timestamp>.stp`. Valid only on `DesignItem` rows with a `tipRootComponentVersion`.
- `u` opens the item's `fusionWebUrl` permalink in your default browser (after details load).

### Non-US hubs

```sh
APS_REGION=EMEA fusionlocalserver   # Europe, Middle East, Africa
APS_REGION=AUS  fusionlocalserver   # Australia
```

## Install

**Homebrew (macOS / Linux)**
```sh
brew install schneik80/fusionlocalserver/fusionlocalserver
```

Or grab a binary from [Releases](https://github.com/schneik80/fusionlocalserver/releases):

```sh
# macOS arm64 / amd64, linux amd64 — pick your platform's asset
VERSION=$(curl -s https://api.github.com/repos/schneik80/fusionlocalserver/releases/latest | grep '"tag_name"' | cut -d'"' -f4 | tr -d v)
curl -L "https://github.com/schneik80/fusionlocalserver/releases/latest/download/fusionlocalserver-${VERSION}-darwin-arm64.tar.gz" | tar xz
sudo mv fusionlocalserver /usr/local/bin/
```

Released binaries ship the embedded web UI and a publisher client ID, so they need no build step or configuration.

## Building from source

Requires **Go 1.22+** and **Node/npm** (for the web UI).

```sh
git clone https://github.com/schneik80/fusionlocalserver
cd fusionlocalserver
```

Register a public client at [aps.autodesk.com/myapps](https://aps.autodesk.com/myapps) (redirect URI `http://localhost:7879/callback`, scope `data:read user-profile:read`), then:

```sh
echo "your-client-id" > .aps-client-id    # git-ignored
make build                                # vite build → embed UI (-tags embed_ui) → go build
./fusionlocalserver -server               # or: make run
```

| Target | What it does |
|--------|--------------|
| `make build` | Build the web UI, embed it (`-tags embed_ui`), and compile with the client ID baked in |
| `make run` | `make build` then serve on the LAN (`ARGS="-addr ..."` to override) |
| `make dev` | Go-only build, **no** embedded UI (serves a stub) and no embedded client ID — pair with `cd web && npm run dev` and `./fusionlocalserver -server -dev` for hot reload |
| `make check` | `go vet ./...` + `go test -race ./...` |

`server/webdist/` is entirely gitignored build output; a plain `go build` (no `embed_ui` tag) compiles against an in-memory stub, so the tree never needs a committed placeholder.

## Requirements

- An [Autodesk account](https://accounts.autodesk.com) with access to at least one Fusion Team hub
- macOS 12+, Linux, or Windows 10+
- Port `7879` available locally during sign-in (OAuth callback); the server also needs its listen port (default `8080`) free

## Documentation

| Doc | What it covers |
|---|---|
| [`docs/server-webui-plan.md`](docs/server-webui-plan.md) | The `-server` mode + React/MUI web UI: architecture, API routes, build |
| [`docs/navigation.md`](docs/navigation.md) | Three-column browser, details-pane tabs, Show in Location, mouse, themes |
| [`docs/api.md`](docs/api.md) | APS Manufacturing Data Model GraphQL queries, retry behaviour, debug logging |
| [`docs/authentication.md`](docs/authentication.md) | OAuth PKCE flow, token storage, refresh |
| [`docs/architecture.md`](docs/architecture.md) | C4 diagrams, package layout, data flow, performance, resilience |
| [`docs/debugging.md`](docs/debugging.md) | **Reporting a bug** — what to capture and how to file it |
| [`docs/testing.md`](docs/testing.md) | Test strategy and how to run / extend the suite |
| [`docs/development.md`](docs/development.md) | Building from source, release pipeline, dependencies |

## License

[GNU General Public License v3.0](LICENSE) — © Kevin Schneider
