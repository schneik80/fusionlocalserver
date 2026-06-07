# fusionlocalserver

[![test](https://github.com/schneik80/fusionlocalserver/actions/workflows/test.yml/badge.svg)](https://github.com/schneik80/fusionlocalserver/actions/workflows/test.yml)

A local server and web UI for the [Autodesk Platform Services (APS)](https://aps.autodesk.com) Manufacturing Data Model. Run it on your LAN and let people browse your Fusion hubs, projects, folders, and designs from a browser — **each user signs in with their own Autodesk account**.

> **v3 / Collaborative-Editing hubs only.** This app targets the **v3** Manufacturing Data Model GraphQL API exclusively, which Autodesk documents as available on **Collaborative-Editing (CE) hubs only** (`hubDataVersion` major version ≥ 2). Non-CE hubs are filtered out of the hub list, since their data does not resolve through the v3 graph.

One Go binary: an HTTP server that exposes a JSON API and serves an embedded React/MUI single-page web UI. There are no external dependencies — it's pure Go standard library.

## Quick start

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

Open one of those URLs in a browser and click **Sign in with Autodesk**. Each visitor authenticates with their own Autodesk account; the server holds their tokens in a per-session store keyed by an `HttpOnly` cookie, and proxies their data calls under their own identity. Tokens never reach the browser's JavaScript.

> ⚠️ **Plain HTTP on the LAN.** Without `-tls` the session cookie is not marked `Secure` (browsers drop `Secure` cookies over `http://`), so anyone able to sniff the wire could capture a cookie and hijack that user's session until it expires. Use **`-tls`** (below) or front the server with a TLS-terminating proxy; the cookie automatically becomes `Secure` once requests arrive over HTTPS. A warning is logged when the server binds a non-loopback address over plain HTTP.

### Flags & settings

| Flag | Default | Purpose |
|------|---------|---------|
| `-v` | off | Verbose logging: debug level to the console **and** the log file, including a line per request and (redacted) upstream API traces |
| `-dev` | off | Developer mode: reverse-proxy the web UI to the Vite dev server for HMR instead of serving the embedded build |
| `-tls` | off | Serve over HTTPS so the session cookie is `Secure`. With no cert given, a self-signed one is generated and cached under `~/.config/fusionlocalserver/` (browsers warn once); use `-tls-cert`/`-tls-key` to supply your own PEM pair. The OAuth callback then becomes `https://…/api/auth/callback`. |
| `-public-url` | derived | Canonical external base URL clients use, e.g. `https://fusion.lan:8080`. When set, the OAuth `redirect_uri` is built from it — so you register **one** callback on the APS app — and any client that arrives via a different host is redirected to it. Without it, the callback is derived from each client's address and every distinct origin must be registered separately. |

> **APS callback registration.** APS validates the OAuth `redirect_uri` by exact match (no wildcards). You don't register clients — you register the server's callback URL(s). The simplest setup is to pick **one** stable address everyone uses (a hostname or static IP), pass it as `-public-url`, and register just `<public-url>/api/auth/callback`. `localhost` ≠ `127.0.0.1`, each LAN IP/hostname is distinct, and `-tls` makes the scheme `https` — so a fixed `-public-url` is the way to keep it to a single registration.

The listen **port is configurable at runtime** from the web UI's Settings dialog (persisted to `~/.config/fusionlocalserver/server.json`). Changing it restarts the listener in place; the page then reconnects on the new port. The port field is read-only in `-dev` mode (where the Vite proxy is pinned to the default port).

Sessions are kept in an encrypted file (`~/.config/fusionlocalserver/sessions.enc`), so a server restart no longer logs everyone out. Each browser also remembers its last-used hub.

Logs go to the console and to `~/.config/fusionlocalserver/server.log`. The default level is essential-only; `-v` adds the per-request and upstream-trace detail.

### Web UI

The web UI is a three-column browser — **Projects │ Contents │ Details** — with a global header (signed-in user + sign-out), a left rail (Hubs / Pins / Settings), and a clickable breadcrumb. Highlights:

- **Details panel** — the document's metadata (type, part number, material, dates…) is always shown beside its **thumbnail**; tabs add **History**, **Properties**, **BOM**, **Uses**, **Where Used**, and **Drawings**. v3 has no integer version numbers, so History is a time-based change log (timestamp, change type, author) rather than a numbered version list.
- **Thumbnails** are fetched once, cached server-side, warmed in the background as you browse, and streamed same-origin — so opening a design is usually instant.
- **Properties** shows a component's **extended base properties** (the hub's base-property definitions populated with the component's values) and its **physical/mass properties** (mass, volume, surface area, density, bounding box) from the v3 Manufacturing Data Model API.
- **Search** — a global search lightbox runs a hub-wide search (free-text or by a searchable property); results jump straight to the document via Show-in-Location.
- **Uses / Where Used / Drawings** rows are clickable: selecting one navigates the browser straight to that document. (v3 has no reverse-reference query, so **Where Used returns empty** — see [`docs/v3-where-used.md`](docs/v3-where-used.md).)
- **Projects** can be **created** (the "+" button), **renamed**, and **archived** (right-click a project) using the v3 project mutations.
- **Pins** and **Light/Dark/System theme** are available from the rail and Settings.

See [`docs/web-ui.md`](docs/web-ui.md) for a full tour.

### Non-US hubs

Set the region the server queries (applies to every user) via the APS region, e.g. `APS_REGION=EMEA` or `APS_REGION=AUS` (default is US). See [`docs/development.md`](docs/development.md) for configuration precedence.

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

Requires **Go 1.23+** and **Node/npm** (for the web UI).

```sh
git clone https://github.com/schneik80/fusionlocalserver
cd fusionlocalserver
```

Register a web app at [aps.autodesk.com/myapps](https://aps.autodesk.com/myapps) with scope `data:read data:write data:create data:search user-profile:read`. (v3 needs the wider data scope: `data:search` for hub search and `data:write`/`data:create` for the project create/rename/archive mutations.) **Register a Callback URL for every origin users will reach the server by** — APS allows no wildcards, so each `http(s)://host:port/api/auth/callback` is a separate exact-match entry. For local development that is `http://localhost:8080/api/auth/callback`; add each LAN address (and each configured port) you intend to use.

> **Re-login once after upgrading.** The v3 scope is wider than the old v2 `data:read user-profile:read` set, so existing users must re-consent (sign in again) once for the new scopes to take effect.

```sh
echo "your-client-id" > .aps-client-id    # git-ignored
make build                                # vite build → embed UI (-tags embed_ui) → go build
./fusionlocalserver                       # or: make run
```

| Target | What it does |
|--------|--------------|
| `make build` | Build the web UI, embed it (`-tags embed_ui`), and compile with the client ID baked in |
| `make run` | `make build` then serve on the LAN (`ARGS="-v"` to add flags) |
| `make dev` | Go-only build, **no** embedded UI (serves a stub) and no embedded client ID — pair with `cd web && npm run dev` and `./fusionlocalserver -dev` for hot reload |
| `make check` | `go vet ./...` + `go test -race ./...` |

`server/webdist/` is entirely gitignored build output; a plain `go build` (no `embed_ui` tag) compiles against an in-memory stub, so the tree never needs a committed placeholder.

## Requirements

- An [Autodesk account](https://accounts.autodesk.com) with access to at least one Fusion Team hub (each user needs their own)
- macOS 12+, Linux, or Windows 10+
- The server's listen port (default `8080`) free on the host

## Documentation

| Doc | What it covers |
|---|---|
| [`docs/web-ui.md`](docs/web-ui.md) | The web UI: sign-in, the three-column browser, details tabs, search, project create/rename/archive, pins, settings |
| [`docs/authentication.md`](docs/authentication.md) | Per-user OAuth (PKCE) login, sessions, cookies, token refresh |
| [`docs/api.md`](docs/api.md) | APS Manufacturing Data Model **v3** GraphQL queries, retry behaviour, debug logging |
| [`docs/3d-viewer.md`](docs/3d-viewer.md) | The **3D / Parameters / Timeline** tabs: native-file download chain (MFGDM → Data Management → OSS), on-demand decode via `f3d-reader`, and the three.js viewer (AO, edges, HDRI) |
| [`docs/architecture.md`](docs/architecture.md) | C4 diagrams, package layout, request/session flow, performance, resilience |
| [`docs/development.md`](docs/development.md) | Building from source, configuration, release pipeline, dependencies |
| [`docs/debugging.md`](docs/debugging.md) | Logging, `-v`, and **reporting a bug** — what to capture and how to file it |
| [`docs/testing.md`](docs/testing.md) | Test strategy and how to run / extend the suite |
| [`docs/v3-where-used.md`](docs/v3-where-used.md) | Why Where-Used returns empty on v3, and the options to resume it |
| [`docs/server-webui-plan.md`](docs/server-webui-plan.md) | Historical: the original design plan for the server + web UI |

## License

[GNU General Public License v3.0](LICENSE) — © Kevin Schneider
