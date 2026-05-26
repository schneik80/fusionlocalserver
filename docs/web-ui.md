# Web UI

The web UI is a React/MUI single-page app served by the Go binary (embedded in
release builds). It talks to the server over the same-origin JSON API under
`/api` — see [`docs/api.md`](api.md) for the upstream data layer and
[`docs/architecture.md`](architecture.md) for the HTTP route surface.

## Signing in

On load the SPA probes `GET /api/auth/me`:

- **Loading** — a centered spinner while the probe is in flight.
- **Signed out** — the **login screen**: a "Sign in with Autodesk" button that
  navigates to `/api/auth/login`. The server redirects to Autodesk; after you
  consent it redirects back, sets your session cookie, and the app loads. If a
  sign-in attempt fails, the server returns to `/?auth_error=<reason>` and the
  login screen shows a readable message.
- **Signed in** — the three-column browser.

The signed-in user's name (or email) and a **sign-out** button sit at the right
of the header. Sign-out calls `POST /api/auth/logout` and reloads at `/`, which
returns you to the login screen. If your session expires while browsing, the
next data call gets a 401 and the app bounces you to sign in again.

See [`docs/authentication.md`](authentication.md) for the full flow.

## Layout

```
┌───────────────────────────────────────────────────────────────┐
│ ▣ fusionlocalserver  v1.2.3      [Hub ▾]  ☾   user   ⏏          │  header
├──┬────────────────────────────────────────────────────────────┤
│  │ Hub › Project › Folder › Item                               │  breadcrumb
│R ├─────────────┬──────────────────┬───────────────────────────┤
│a │ Projects    │ Contents         │ Details                    │
│i │  • Proj A   │  ▸ Folder        │  ┌─────────┐  Name          │
│l │  • Proj B   │  ▦ Design        │  │ thumb   │  Type / PN …   │
│  │             │  ▦ Drawing       │  └─────────┘  [tabs]        │
└──┴─────────────┴──────────────────┴───────────────────────────┘
```

- **Left rail** — Hubs, Pins, and Settings.
- **Breadcrumb** — Hub › Project › Folder… › Item; each segment is clickable.
- **Projects** column — projects in the selected hub.
- **Contents** column — folders and items in the selected project/folder.
- **Details** panel — the selected document's metadata and thumbnail, plus tabs.

## Details panel

Metadata (type, part number, description, material, version, dates) shows beside
the document's **thumbnail**. Tabs:

| Tab | Shows |
|-----|-------|
| **Details** | Core metadata and the thumbnail |
| **History** | Version history (newest first) |
| **Properties** | Physical/mass properties — mass, volume, surface area, density, bounding box (v2 Manufacturing Data Model) |
| **Uses** | Components used by this design (or, for a drawing, its source design) |
| **Where Used** | Designs that use this component |
| **Drawings** | Drawings made from this design |

Rows in **Uses / Where Used / Drawings** are clickable — selecting one navigates
the browser straight to that document.

Thumbnails and physical properties are generated asynchronously by APS; the UI
polls until each settles. Thumbnails are cached server-side and streamed
same-origin, so repeat views are instant.

## Hubs, Pins, Settings

- **Hubs** (rail or the header chip) — switch the active hub.
- **Pins** — bookmark a project, folder, or document for fast access; pins are
  scoped per hub and persisted on the server.
- **Settings** — Light/Dark/System **theme** (stored in your browser), the APS
  **region** the server is using, and the **listen port**. Changing the port
  restarts the listener in place and the page reconnects on the new port; the
  field is read-only in `-dev` mode.
