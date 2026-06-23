# fusionlocalserver

[![test](https://github.com/schneik80/fusionlocalserver/actions/workflows/test.yml/badge.svg)](https://github.com/schneik80/fusionlocalserver/actions/workflows/test.yml)

A local server and web UI for the [Autodesk Platform Services (APS)](https://aps.autodesk.com) Manufacturing Data Model. Run it on your LAN and let people browse your Fusion hubs, projects, folders, and designs from a browser — **each user signs in with their own Autodesk account**.

One Go binary: an HTTP server that exposes a JSON API and serves an embedded React/MUI single-page web UI. There are no external dependencies — it's pure Go standard library.

## Quick start

```sh
echo "your-aps-client-id" > .aps-client-id   # one time; see "Building from source"
make run                                      # build the UI + binary, then serve over HTTPS
```

`make run` serves over **HTTPS** (`-tls` is on by default), binds `0.0.0.0:8080`, and logs every reachable URL at startup:

```
server starting  addr=0.0.0.0:8080 tls=true ...
reachable on the LAN  url=https://localhost:8080
reachable on the LAN  url=https://192.168.1.50:8080
```

The first time, `-tls` generates and caches a self-signed certificate under `~/.config/fusionlocalserver/` (browsers warn once — accept it); pass `-tls-cert`/`-tls-key` to supply your own PEM pair. Open one of those URLs in a browser and click **Sign in with Autodesk**. Each visitor authenticates with their own Autodesk account; the server holds their tokens in a per-session store keyed by an `HttpOnly` cookie, and proxies their data calls under their own identity. Tokens never reach the browser's JavaScript.

> ⚠️ **Don't disable TLS on a shared network.** Over plain HTTP the session cookie is not marked `Secure` (browsers drop `Secure` cookies over `http://`), so anyone able to sniff the wire could capture a cookie and hijack that user's session until it expires. `make run` keeps `-tls` on for this reason; only override it (`make run TLS=`) behind a TLS-terminating proxy or for loopback-only testing. A warning is logged when the server binds a non-loopback address over plain HTTP.

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

- **Details panel** — the document's metadata (type, part number, material, version, dates…) is always shown beside its **thumbnail**; tabs add **History**, **Properties**, **Uses**, **Where Used**, and **Drawings**.
- **Thumbnails** are fetched once, cached server-side, warmed in the background as you browse, and streamed same-origin — so opening a design is usually instant.
- **Properties** shows physical/mass properties (mass, volume, surface area, density, bounding box) from the v2 Manufacturing Data Model API.
- **Uses / Where Used / Drawings** rows are clickable: selecting one navigates the browser straight to that document.
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

Register a web app at [aps.autodesk.com/myapps](https://aps.autodesk.com/myapps) with scope `data:read user-profile:read`. **Register a Callback URL for every origin users will reach the server by** — APS allows no wildcards, so each `http(s)://host:port/api/auth/callback` is a separate exact-match entry. Since the server serves HTTPS by default, that is `https://localhost:8080/api/auth/callback` for local development (use `http://…` only if you run with `make run TLS=`); add each LAN address (and each configured port) you intend to use.

```sh
echo "your-client-id" > .aps-client-id    # git-ignored
echo "https://your-host:8080" > .aps-public-url   # optional, git-ignored: the URL your APS callback is registered under
make build                                # vite build → embed UI (-tags embed_ui) → go build
./fusionlocalserver -tls                  # serve over HTTPS, or just: make run
```

If `.aps-public-url` is present, `make build` bakes it in as the canonical base URL: the binary then builds the OAuth `redirect_uri` from it (so you register **one** callback) and redirects clients on other hosts to it — no `-public-url` flag needed. The flag still overrides the baked-in value.

| Target | What it does |
|--------|--------------|
| `make build` | Build the web UI, embed it (`-tags embed_ui`), and compile with the client ID baked in |
| `make run` | `make build` then serve over HTTPS on the LAN (`-tls` on by default; `make run TLS=` for plain HTTP, `ARGS="-v"` to add flags) |
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
| [`docs/web-ui.md`](docs/web-ui.md) | The web UI: sign-in, the three-column browser, details tabs, pins, settings |
| [`docs/authentication.md`](docs/authentication.md) | Per-user OAuth (PKCE) login, sessions, cookies, token refresh |
| [`docs/api.md`](docs/api.md) | APS Manufacturing Data Model GraphQL queries, retry behaviour, debug logging |
| [`docs/architecture.md`](docs/architecture.md) | C4 diagrams, package layout, request/session flow, performance, resilience |
| [`docs/development.md`](docs/development.md) | Building from source, configuration, release pipeline, dependencies |
| [`docs/debugging.md`](docs/debugging.md) | Logging, `-v`, and **reporting a bug** — what to capture and how to file it |
| [`docs/testing.md`](docs/testing.md) | Test strategy and how to run / extend the suite |
| [`docs/server-webui-plan.md`](docs/server-webui-plan.md) | Historical: the original design plan for the server + web UI |

## License

[GNU General Public License v3.0](LICENSE) — © Kevin Schneider
