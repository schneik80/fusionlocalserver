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

Metadata (type, part number, description, material, dates) shows beside
the document's **thumbnail**. The **type** reads as a friendly label and, for
designs, appends the assembly/part classification — e.g. "3D Design — Assembly",
"3D Design — Part". (v3 has no integer version number, so there is no version
field in the metadata — change history is the History tab below.) Tabs:

| Tab | Shows |
|-----|-------|
| **History** | The time-based change log (newest first). v3 has no integer version numbers, so each entry is a change — timestamp, change type (e.g. "Version Created"), and author — rather than a numbered version |
| **Properties** | A component's **extended base properties** (the hub's base-property definitions, populated with this component's values where set) followed by its **physical/mass properties** — mass, volume, surface area, density, bounding box (v3 Manufacturing Data Model) |
| **BOM** | Immediate bill of materials — Component / Part № / Material / Qty. Quantity is a real v3 `quantity` field on each BOM relation (not an occurrence count). v3 `bomRelations` is depth-1, so this is the direct-children BOM, not a flattened multi-level tree |
| **Uses** | Components used by this design (or, for a drawing, its source design) |
| **Where Used** | Designs that use this component. v3 has **no reverse-reference query**, so this currently returns empty — see [`v3-where-used.md`](v3-where-used.md) |
| **Drawings** | Drawings made from this design |
| **Permissions** | The groups (and roles) with access to the item's project; expand a group to list its member users (member listing needs hub-admin access, otherwise a "no permission" note is shown) |

The core metadata and thumbnail are always shown above the tabs (there is no
separate "Details" tab). Rows in **Uses / Where Used / Drawings** are clickable —
selecting one navigates the browser straight to that document.

Thumbnails and physical properties are generated asynchronously by APS; the UI
polls until each settles. Thumbnails are cached server-side and streamed
same-origin, so repeat views are instant.

## Search

A **search lightbox** in the global header runs a **hub-wide** search across the
active hub (v3 `searchByHub`). It supports two modes:

- **Free text** — full-text search over the hub's documents.
- **By property** — pick a searchable property (the picker is populated from the
  hub's `searchablePropertiesByHub`) and supply a value.

Each result row shows a thumbnail (when available) and the matched text;
selecting a row uses Show-in-Location to navigate the browser straight to that
document. Results are paginated and loaded on demand.

## Projects — create, rename, archive

The **Projects** column supports the v3 project lifecycle mutations:

- **Create** — the **"+"** button adds a new project to the active hub
  (`createProject`).
- **Rename** — right-click a project and choose **Rename** (`renameProject`).
- **Archive** — right-click a project and choose **Archive** (`archiveProject`;
  reversible server-side via `restoreProject`).

These require the wider v3 scope (`data:write` / `data:create`); see
[`authentication.md`](authentication.md).

## Hubs, Pins, Settings

- **Hubs** (rail or the header chip) — switch the active hub. Your last hub is
  remembered in this browser and reselected on return (if you still have access
  to it).
- **Pins** — bookmark a project, folder, or document for fast access; pins are
  scoped per hub and persisted on the server.
- **Settings** — Light/Dark/System **theme** (stored in your browser), the APS
  **region** the server is using, and the **listen port**. Changing the port
  restarts the listener in place and the page reconnects on the new port; the
  field is read-only in `-dev` mode.
