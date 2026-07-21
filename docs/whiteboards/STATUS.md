# Whiteboards — status

A per-project whiteboard built on [tldraw](https://tldraw.dev): the fifth project
app beside Tasks, Wiki, Chat and Production. Branch `feat/whiteboards`.

Draw freely, and drop **live app cards** — tasks, jobs, batches, documents — onto
the canvas alongside the sketching.

## ⚠️ Licensing — read before shipping this

**tldraw's SDK is not free for production use.** Its licence permits use *in
development*; production requires a licence:

- **Commercial** — a paid licence key, obtained via tldraw's sales form.
- **Hobby** (non-commercial only) — free, but the "made with tldraw" watermark
  must remain visible on the canvas.

This integration ships **no licence key**, which is the compliant
development/hobby path and leaves the watermark in place. If this app is used
commercially, a licence must be purchased and the key supplied to the `Tldraw`
component — that is a business decision, not a code change we should make
silently. See https://tldraw.dev/community/license.

## Assets, CSP and the watermark

tldraw loads its fonts, icons and translations from `cdn.tldraw.com` by default.
This app ships a strict CSP (`default-src 'self'`), so every one of those
requests was blocked — the fonts never arrived (unreadable canvas text) and the
blocked translations fetch rejected inside React's commit phase, killing the
board seconds after it opened.

Fixed by resolving the assets through the bundler
(`getAssetUrlsByMetaUrl()` from `@tldraw/assets`, passed as `assetUrls`), so
Vite emits them as same-origin files. **The CSP was not weakened**, and the
whiteboard now works offline like the rest of this local-first app.

One CDN call remains and is still blocked: the **unlicensed watermark**. Without
a licence key tldraw fetches its watermark image and pings a tracking URL. That
ping carries the full page URL — which for this app includes the hub and project
URNs and the project's name. Leaking internal project names to a third party is
a poor trade for a local engineering tool, so the CSP deliberately still blocks
it. The cost is a console warning and an unhandled rejection from tldraw's
tracker, plus the licensing wrinkle below.

If that trade should go the other way, allow `https://cdn.tldraw.com` in
`connect-src` and `img-src` in `securityHeaders` (`server/middleware.go`) — but
see the privacy note above first. Buying a commercial licence removes the
watermark, the ping and the question.

The tracker ping is **inert**, despite the scary console output.
`LicenseManager.maybeTrack` (`@tldraw/editor/.../license/LicenseManager.js:126`)
calls `fetch(url)` and discards the promise, so the CSP rejection can never
reach the `.catch()` that sets the licence state — it only ever surfaces as an
unhandled rejection in the console. It is **not** why the board disappears; see
below.

## The board vanishes after ~5s — tldraw's licence gate, not a bug

Symptom: the canvas mounts and works, then a few seconds later silently
disappears, with **no error, no React crash and nothing for an ErrorBoundary to
catch**. This is deliberate tldraw behaviour, not a fault in this app:

```js
// @tldraw/editor/.../license/LicenseProvider.js
function shouldHideEditorAfterDelay(s) { return s === 'expired' || s === 'unlicensed-production' }
const LICENSE_TIMEOUT = 5e3
// …after 5s: setShowEditor(false)  →  <LicenseGate/>  →  <div style={{display:'none'}}/>
```

With no key the state is `unlicensed-production` **whenever tldraw decides the
page is not development**. Its heuristic
(`LicenseManager.getIsDevelopment`) treats a page as development only if the
protocol isn't `https:`, **or** the hostname is loopback (`localhost`, `::1`,
`127.x`), **or** `NODE_ENV !== 'production'`.

This app is served over **HTTPS on a LAN hostname** (`.aps-public-url`, e.g.
`https://ryzen-nobara.local:8080`) from a production Vite build, so all three
are false and tldraw classifies a single-user local tool as a production
deployment. Reaching the same server at `https://localhost:8080` flips the
loopback clause and the gate never fires — **but** the OAuth callback host must
match the APS app registration, so changing the host breaks login unless that
callback is registered too (see the redirect_uri gotcha).

There is no code fix here. The options are: buy a licence key and pass it to
`<Tldraw licenseKey=…>`, use the app over loopback where that genuinely is
hobby/development use, or drop the feature. Serving whiteboards to other
machines on the network is production use and needs a licence.

## Model

| Concept | What it is |
|---|---|
| **Whiteboard** | A named board in a project. `w<n>`, listed newest-first. |
| **Document** | The tldraw document (shapes, pages, bindings) for one board. Stored opaquely — the server never parses tldraw's schema. |
| **fls-card shape** | A custom tldraw shape whose only state is an `fls:` token. |

### Storage shape, and why it differs from its siblings

Tasks and Production keep a project's whole feature state in one JSON file. That
does not work here: a tldraw document is megabytes of shapes and is rewritten on
every autosave. So whiteboards split:

```
<config>/whiteboards/<sanitized-projectId>/
  whiteboards.json      metadata only — names, timestamps, sizes
  doc-w1.json           one tldraw document per board
  doc-w2.json
```

Listing boards therefore never touches a document, and saving a board rewrites
only that board. Both files are written atomically (temp + rename) — the
difference between a whiteboard and a truncated whiteboard.

### Cards are references, not screenshots

The `fls-card` shape stores an `fls:doc` / `fls:task` / `fls:job` / `fls:batch`
token and renders it through the shared `components/RefCard.tsx`. A card on a
board is the *live* task or batch, re-hydrated on every render — rename the task
and the board follows. It also means any future card scheme works here for free,
since `RefCard` is the single place tokens map to renderers.

Cards are placed from the canvas toolbar, reusing the existing pickers
(`AttachTaskDialog`, `ProductionRefDialog`, `HubBrowserDialog`) — the same
dialogs chat, the wiki and task details use, so "insert a card" behaves
identically everywhere.

Pointer events on a card are off until it is the only selected shape: otherwise
the card's own click targets would swallow the drag that moves it. One click
selects, the next interacts.

## Layout

**Backend**
- `whiteboards/types.go`, `whiteboards/store.go` — the store described above.
  Caps: 200 boards/project, 24 MiB per document.
- `server/handlers_whiteboards.go`, `dto_whiteboards.go`, routes in `routes.go`.
  Authorization reuses `chat.Authorizer` (`CapRead` view, `CapPost` edit,
  `CapModerate`-or-creator delete), like every other project app.

**Frontend** — `web/src/whiteboards/`
- `WhiteboardsApp` — project tab: board rail (create / rename on double-click /
  delete) + the selected board's canvas.
- `WhiteboardCanvas` — loads the document once, autosaves on a 1.5s debounce,
  and flushes on unmount. **Lazy-loaded**: tldraw is ~1.7 MB, so it is code-split
  out of the entry bundle and only fetched when the tab is opened.
- `cardshape.tsx` — the `fls-card` ShapeUtil.
- `whiteboard.css` — **the only stylesheet in this app.** Everything else is
  styled through MUI `sx`, but tldraw is themed via CSS variables and can only
  be reskinned from CSS. It is scoped to `.fls-tldraw` so nothing leaks, and
  only overrides presentation (Montserrat, the `#0696d7` accent, 6px radii).
  The light/dark scheme is driven from the app's colour mode, not tldraw's.

## API

```
GET    /api/whiteboards      ?projectId              list + capabilities
POST   /api/whiteboards      ?projectId              {hubId,projectName,name}
PATCH  /api/whiteboards      ?projectId&boardId      rename
DELETE /api/whiteboards      ?projectId&boardId      moderator or creator
GET    /api/whiteboards/doc  ?projectId&boardId      the document, or null if unsaved
PUT    /api/whiteboards/doc  ?projectId&boardId      replace the document (autosave)
```

The document endpoints carry their own much larger body cap (24 MiB) than the
64 KiB used everywhere else, and pass the payload through opaquely — the store
checks it is JSON and within the cap, and nothing parses tldraw's schema.

## Known gaps / next

- **No realtime collaboration.** Two people editing the same board will
  last-write-wins each other's autosaves. tldraw offers a sync service; this
  ships single-writer.
- Documents are stored whole on every save; there is no incremental diffing.
- No board thumbnails in the list.
- No cross-project "my whiteboards" screen (the store's self-describing project
  file supports adding one, as with tasks/production).
- The tldraw skin is deliberately light-touch — brand colours, type and radii.
  Deeper chrome restyling is possible but couples us to tldraw's internal class
  names.

## Verifying

```
go build ./... && go test ./...      # store tests: CRUD, snapshot round-trip,
                                     # document deleted with its board, caps,
                                     # corruption + future-version recovery
cd web && npx tsc --noEmit && npm run build
```

End-to-end (needs APS login): open a project → Whiteboards → create a board →
draw → place a task, a job/batch and a document card → reload and confirm the
board and its cards return → rename and delete a board → confirm a read-only
project member gets a non-editable canvas.
