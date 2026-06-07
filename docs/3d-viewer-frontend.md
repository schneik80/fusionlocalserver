# 3D / Parameters / Timeline — Frontend

This document describes the **frontend** (React + MUI + three.js) of the
"3D / Parameters / Timeline" feature in FusionLocalServer's web app. It covers
how the three tabs are wired into the details panel, the self-contained three.js
viewer and its post-processing pipeline, the institutional history behind
choosing raw three.js over `<model-viewer>`, error handling, and the new
dependencies.

All paths are relative to `web/src/` unless noted.

Relevant files:

- `components/SceneViewer.tsx` — the three.js viewer
- `components/DetailsPanel.tsx` — the tabs and the shared model gate
- `components/ParametersTable.tsx` — the Parameters table
- `components/TimelineList.tsx` — the Timeline list
- `components/ErrorBoundary.tsx` — render-error fallback wrapping the viewer
- `api/client.ts` — `modelStatus` / `modelData` / `modelGlbUrl`
- `api/queries.ts` — `useModelStatus` / `useModelData`
- `api/types.ts` — `ModelStatus` / `ModelData` / `ModelParameter` / `ModelTimelineEntry`
- `n8ao.d.ts` — local typings for the `n8ao` package
- `package.json` — the `three` / `postprocessing` / `n8ao` dependencies

---

## 1. The three tabs and the shared decode job

### Where the tabs live

`DetailsPanel.tsx` renders the per-document details panel. Each selected
document gets a tab set determined by `tabsFor(kind)`:

- `design` → `['model', 'parameters', 'timeline', 'history', 'properties', 'bom', 'uses', 'whereUsed', 'drawings', 'permissions']`
- `configured` → `['model', 'parameters', 'timeline', 'history', 'properties', 'bom', 'permissions']`
- `drawing` → `['history', 'uses', 'permissions']`
- everything else → `['history']`

So the three feature tabs — **3D** (`model`), **Parameters** (`parameters`),
and **Timeline** (`timeline`) — appear only for designs and configured designs.
Their display labels come from `TAB_LABEL`: `model → "3D"`, `parameters →
"Parameters"`, `timeline → "Timeline"`.

The tab components are `Model3DTab`, `ModelParametersTab`, and
`ModelTimelineTab`. All three are rendered inside `SelectedDetails` and are
passed `hubId`, `item.id`, `ver={detailsQ.data?.tipTimestamp}`, and `active`.

### One shared server-side decode job

All three tabs draw from a **single** server-side decode job. The job lazily
downloads the design's native file, decodes it with f3d-reader, and produces a
GLB plus the projected parameters/timeline (`data.json`). The three tabs do not
each kick off their own work:

- Each tab calls `useModelStatus` (via the shared `useModelGate` helper). Because
  every call uses the **same React Query key** — `['model', hubId, itemId, ver]`
  — React Query **dedupes** the query. Opening any one of the three tabs
  triggers the job; opening the others **reuses** the in-flight or completed
  result.
- `useModelStatus` polls every ~2s while the job is `PENDING`, stopping on
  `SUCCESS` or `FAILED` (`refetchInterval` returns `false` for those states).
  Unlike the thumbnail probe's tight cap, the model poll is capped **generously**
  — `dataUpdateCount >= 150` (≈150 polls ≈ 5 min) — because large assemblies can
  take minutes to download and decode.
- The data tabs (Parameters, Timeline) additionally call `useModelData`
  (key `['modelData', hubId, itemId, ver]`), enabled only **after** the status
  is `SUCCESS`. The 3D tab does not need `data` (`needData={false}`); it only
  needs the status (to know `hasGlb`) and then loads the GLB binary directly in
  the viewer.

`ver` is the design's tip timestamp (`detailsQ.data?.tipTimestamp`), so a
re-saved design decodes fresh under a new query key.

### `dmProjectId` from nav context

`useModelGate` sources `dmProjectId` from `nav.project?.altId` and passes it to
`useModelStatus`. This is required because **the item's own `project` field
can't be resolved server-side** on these hubs — the project alt-id must come
from the navigation context the user browsed through.

### `ModelGate` — shared loading / pending / failed UI

`ModelGate` centralizes the shared states so every tab renders them uniformly.
Its phases (in order):

1. `statusQ.isLoading` (or active with no status yet) → `ModelProgress`
   ("Preparing model…").
2. `statusQ.error` → `TabError`.
3. status `PENDING` → `ModelProgress` ("Downloading & decoding the design… this
   can take a while for large assemblies.").
4. status `FAILED` → `TabError` (with `statusQ.data?.error` or a fallback).
5. status `SUCCESS`:
   - if `needData` and `dataQ` is loading → `TabSpinner`;
   - if `needData` and `dataQ.error` → `TabError`;
   - otherwise it calls `render({ data, hasGlb })`.

Each tab supplies its own `render`:

- `Model3DTab` (`needData={false}`): if `!hasGlb || !hubId`, shows
  "No 3D geometry (this design was saved without cached graphics).";
  otherwise builds `glbUrl = api.modelGlbUrl({ hubId, itemId, ver })` and
  renders `<ErrorBoundary label="3D viewer"><SceneViewer key={glbUrl}
  glbUrl={glbUrl} /></ErrorBoundary>`. The viewer is **keyed by `glbUrl`** so
  switching documents fully remounts it.
- `ModelParametersTab` (`needData`): renders `<ParametersTable
  parameters={data?.parameters ?? {}} />`.
- `ModelTimelineTab` (`needData`): renders `<TimelineList
  timeline={data?.timeline ?? []} />`.

A design saved without cached graphics still yields parameters/timeline (decode
succeeds); only the 3D tab reports no geometry.

### Layout: 3D fills, data tabs scroll

The content area in `SelectedDetails` switches its sizing on the active tab:

```ts
...(tab === 'model'
  ? { display: 'flex', overflow: 'hidden' }   // 3D fills the area, no padding
  : { overflowY: 'auto', p: 2 })              // Parameters/Timeline scroll, padded
```

So the **3D tab fills the content area** (tree pane + canvas, no padding), while
**Parameters and Timeline scroll** with `p: 2` padding.

### Re-visits don't invalidate

`SelectedDetails` has an effect keyed on `tab` that invalidates the active tab's
queries on tab change so the user never sees stale data — **except** for `model`
/ `parameters` / `timeline`, which fall through with a comment: the decoded model
(geometry, parameters, timeline) is **immutable for a given version**, so
invalidating would needlessly re-trigger the download + decode. `useModelStatus`
and `useModelData` use `staleTime: Infinity` to match.

### Parameters and Timeline rendering

- `ParametersTable.tsx` renders `Record<string, ModelParameter>` as a
  filterable, name-sorted table with columns **Name**, **ID** (`userName`, the
  Fusion handle like `d12`), **Expression** (display value, e.g. `40 mm`), and
  **Value** (the raw internal magnitude, trimmed to 6 significant figures, plus
  `unit`). A `TextField` filters on name/userName/expression; empty input shows
  "No parameters".
- `TimelineList.tsx` renders `ModelTimelineEntry[]` as an ordered list in
  timeline order. Each row shows a 1-based position, a display label
  (`displayName || name || "Feature N"`), and an outlined `Chip` with a
  humanized feature type — `humanizeFeatureType` strips a leading
  `adsk::fusion::` namespace and a trailing `Feature` suffix (e.g.
  `adsk::fusion::ExtrudeFeature` → `Extrude`). Empty input shows "No timeline".

---

## 2. The three.js viewer (`SceneViewer.tsx`)

`SceneViewer` is a **self-contained three.js viewer** — explicitly **not**
`<model-viewer>` (see §4 for why). It takes a single prop, `glbUrl`, and builds
its whole scene inside one `useEffect` keyed on `glbUrl`, tearing everything down
in the cleanup.

### Renderer, controls, camera

- **`WebGLRenderer`** with `{ antialias: false, alpha: true, powerPreference:
  'high-performance' }`. Pixel ratio capped at 2. Tone mapping on the renderer is
  set to `THREE.NoToneMapping` — tone mapping is done in the composer (see §3),
  and the clear color is transparent (`setClearColor(0x000000, 0)`).
- **`OrbitControls`** (from `three/examples/jsm/controls/OrbitControls.js`),
  `enableDamping = false`.
- **`PerspectiveCamera`** at FOV 45.

### GLB loading

The model is loaded with **`GLTFLoader`** from `glbUrl`, which is
`api.modelGlbUrl({ hubId, itemId, ver })` → `/api/items/model/glb?...`. The
binary GLB is streamed (Range-capable) by the Go server, not fetched through the
JSON `request()` wrapper. On load the scene is added and the loader's `onError`
sets a local `error` state, which the overlay surfaces as "No 3D geometry to
display."

### HDRI image-based lighting

Lighting is **image-based** from an HDR environment map:

- **`RGBELoader`** loads the HDR at `HDR_URL = '/environments/Generic.hdr'`.
- The equirectangular texture (`EquirectangularReflectionMapping`) is converted
  with **`PMREMGenerator`** (`pmrem.fromEquirectangular(tex).texture`), assigned
  to **`scene.environment`**, and the source texture + PMREM generator are
  disposed.
- If the HDR fails to load, lighting falls back to none but geometry still shows
  (the error callback is a no-op).

### Render-on-demand (no idle loop)

There is **no continuous animation loop**. Rendering is coalesced:

```ts
const render = () => { if (g.disposed) return; g.pending = false; composer.render() }
requestRender.current = () => {
  if (g.pending || g.disposed) return
  g.pending = true
  g.raf = requestAnimationFrame(render)
}
controls.addEventListener('change', requestRender.current)
```

`requestRender` schedules a single `requestAnimationFrame`; the `pending` flag
**coalesces** multiple requests into one frame. A render is requested on
OrbitControls' `'change'` event, after the HDR loads, after the GLB loads, on
resize, and whenever a control (brightness / AO / edges / node visibility)
changes. This keeps the GPU idle when nothing is moving.

### Camera framing with SANE near/far planes

After the GLB loads, the camera is framed to the model's bounding box
(`Box3.setFromObject`). It computes `maxDim`, a viewing `dist` from the FOV, and
positions the camera offset from the box center with `controls.target` at the
center.

Critically, it sets **sane near/far planes scaled to the model**:

```ts
camera.near = maxDim * 0.05
camera.far  = maxDim * 50
```

**Why this matters:** a huge `near:far` ratio destroys the depth buffer's
precision. With a tiny near and an enormous far, the depth buffer can't
distinguish nearby surfaces, which produces two visible bugs at once:

- **see-through faces** — back faces bleed through front faces (z-fighting /
  depth ambiguity), and
- **dead AO** — N8AO's occlusion is computed from the depth buffer, so with no
  usable depth precision it produces nothing.

Scaling near/far to `maxDim` keeps the ratio bounded regardless of model size.

### The "Components" scene-graph tree

After loading, the code **walks `gltf.scene`** to build a flat, depth-indexed
list of named nodes (`TreeNode { uuid, name, depth }`), skipping the synthetic
edge-overlay objects (`LineSegments2`). The left pane (240px, scrollable, titled
"Components") renders each node with a MUI `Checkbox` whose `checked` reflects
`object.visible`.

Toggling a checkbox (`toggleNode`) flips `object.visible` on the mapped
`Object3D`, bumps a `forceTick` to re-render the checkbox state, and requests a
frame.

The walk also honors the reader's hidden flag: during the post-load traverse,
any object whose `userData.ogsHidden === true` is set `obj.visible = false`
up front (extras carried through from the reader via glTF `extras`).

### Brightness / AO / Edges overlay

When loaded, an overlay sits top-right with:

- a **Brightness** `Slider` (min 0.2, max 2, step 0.05) bound to `brightness`,
- an **AO** `Checkbox` bound to `ao`,
- an **Edges** `Checkbox` bound to `edges`.

Each is wired through its own effect (see §3 for what each actually drives).

---

## 3. Post-processing pipeline (pmndrs `postprocessing`)

The viewer composites through an **`EffectComposer`** (from the pmndrs
`postprocessing` package) created with **`{ multisampling: 4 }`** — i.e. MSAA is
done by the composer's render target, not a separate AA pass.

Pass order:

1. **`RenderPass(scene, camera)`** — renders the scene.
2. **`N8AOPostPass(scene, camera, width, height)`** — ambient occlusion.
   Configured with `intensity = 3`, `aoSamples = 16`, `denoiseSamples = 8`,
   `setQualityMode('Medium')`, and after the GLB loads
   `aoRadius = maxDim * 0.15` (and `distanceFalloff = aoRadius`).
3. **`EffectPass(camera, new ToneMappingEffect({ mode: ToneMappingMode.NEUTRAL
   }))`** — tone mapping.

### N8AO is whole-frame screen-space AO

`N8AOPostPass` is a **screen-space** AO pass operating over the **shared depth
buffer** of the whole frame. It occludes across **all meshes at once** — crease
and contact shadows form between separate parts — rather than being a per-mesh
material effect. This is the central reason the viewer looks like a real CAD
render and not flat-shaded geometry. (`aoRadius` auto-scaled to the model size is
what makes the contact shadows the right physical scale.)

### Brightness drives `scene.environmentIntensity`, not exposure

```ts
g.scene.environmentIntensity = brightness   // on the brightness effect
```

Brightness is driven by **`scene.environmentIntensity`** — i.e. it scales the
**IBL (HDRI) lighting** — **not** `renderer.toneMappingExposure`. The reason:
the composer **bypasses the renderer's tone mapping** (the renderer is set to
`NoToneMapping` and the actual tone mapping is the `ToneMappingEffect` in the
composer). So `renderer.toneMappingExposure` has no effect on the composited
image; the only way to change apparent brightness is to scale the environment
lighting that feeds the scene.

### Edges = fat lines

The Edges overlay is drawn with **fat lines** because the simple
`LineBasicMaterial` ignores `linewidth` on most platforms (it is locked to 1px).
The fat-line trio is used instead:

- **`LineSegmentsGeometry`** (built from an `EdgesGeometry`),
- **`LineSegments2`** (the renderable object),
- **`LineMaterial`** with `linewidth: 2` (px), color `0x202020`.

Edge geometry comes from **`EdgesGeometry(mesh.geometry, 30)`** — a **30°**
hard-edge threshold (`EDGE_THRESHOLD_DEG`) so only true CAD edges are outlined.
`LineMaterial.resolution` is a screen-space value, so it is set on creation and
**updated on every resize** (the `ResizeObserver` calls
`m.resolution.set(w, h)` for each edge material). Edge objects render with
`renderOrder = 1` and their `visible` flag follows the Edges checkbox.

### Resize handling

A `ResizeObserver` on the mount node resizes the renderer **and** the composer,
updates `camera.aspect` / projection, updates every edge material's resolution,
and requests a frame.

---

## 4. Why three.js instead of `<model-viewer>` (history / lessons)

This is important institutional knowledge: the feature was **first built on
`@google/model-viewer` + `@google/model-viewer-effects`**, and **SSAO never
rendered**. We hit a chain of causes, in order:

1. **Dual-three.** npm's `model-viewer` and `model-viewer-effects` bundle
   **mismatched copies of three.js** ("dual-three"). With two three.js runtimes
   in the page, the SSAO pass composited the **model translucent** — the effect
   couldn't share types/state with the model's renderer.
2. **Vendoring fixed translucency, not SSAO.** Vendoring the exact f3d-viewer
   trio (matching versions) **fixed the translucency**, but SSAO **still
   produced nothing**.
3. **Hardcoded radius.** `model-viewer-effects`' SSAO has a **hardcoded, tiny
   world-space radius**, and only `strength` is actually wired through — so there
   was no way to scale the AO radius to the model, and it effectively did
   nothing on real-sized CAD models.
4. **Mount-order vs. child-scan.** React's component mount order conflicted with
   the composer's scan of its children, so the effect didn't reliably attach to
   the rendered scene.

We **abandoned `<model-viewer>` for raw three.js + N8AO** specifically so that we
**own the three.js version and the post-processing pipeline** — no bundled,
mismatched three, no opaque effect with a fixed radius.

Three further bugs were then fixed in the raw three.js viewer:

- **`SMAAEffect` can't be merged.** `SMAAEffect` **cannot be combined with other
  effects in a single `EffectPass`** — doing so **threw and produced a white
  screen**. Fixed by dropping SMAA and using the **composer's MSAA**
  (`multisampling: 4`) for anti-aliasing.
- **Depth precision (near/far).** As described in §2, an unscaled `near:far`
  ratio caused **see-through faces and no AO**. Fixed by scaling near/far to the
  model's `maxDim`.
- **Infinite recursion building edges.** The edge overlay was originally created
  **during** `gltf.scene.traverse(...)`, adding `LineSegments2` children as it
  went. Because `LineSegments2` is itself a `Mesh` subclass, `traverse` then
  **visited each newly added edge**, which added another edge, recursing forever
  (`RangeError: Maximum call stack size exceeded`). Fixed by **collecting the
  meshes first**, then adding edges in a separate loop:

  ```ts
  const meshes: THREE.Mesh[] = []
  gltf.scene.traverse((obj) => {
    const mesh = obj as THREE.Mesh
    if (mesh.isMesh && mesh.geometry && !(obj instanceof LineSegments2)) meshes.push(mesh)
  })
  for (const mesh of meshes) { /* build + add the LineSegments2 edge child */ }
  ```

---

## 5. `ErrorBoundary.tsx`

The 3D tab wraps the viewer in `<ErrorBoundary label="3D viewer">`.
`ErrorBoundary` is a class component using `getDerivedStateFromError` /
`componentDidCatch`. If the viewer throws during render/lifecycle (e.g. a WebGL
or effect-construction error), the boundary renders a small message
("3D viewer failed to render." plus the error message) instead of **blanking the
entire app** with a white screen, and logs the error to the console.

**Limitation (documented in the file):** React error boundaries only catch
errors thrown during **render / lifecycle / effects** — they **cannot catch
errors in async callbacks** such as the `GLTFLoader` / `RGBELoader` load
callbacks or `requestAnimationFrame` render callbacks. Those still surface in the
console. (The viewer handles loader failures itself via its `error` state and the
"No 3D geometry" overlay.)

---

## 6. Dependencies added

The feature added three runtime dependencies plus one dev typings package
(`web/package.json`):

| Package | Version | Role |
| --- | --- | --- |
| `three` | `^0.184.0` | the WebGL renderer, loaders (GLTF, RGBE), OrbitControls, fat-line classes |
| `postprocessing` | `^6.39.1` | pmndrs `EffectComposer`, `RenderPass`, `EffectPass`, `ToneMappingEffect` |
| `n8ao` | `^1.10.1` | `N8AOPostPass` screen-space ambient occlusion |
| `@types/three` (dev) | `^0.184.1` | three.js typings |

`n8ao` ships no typings, so `web/src/n8ao.d.ts` provides a minimal
`declare module 'n8ao'` covering just `N8AOPostPass` (its `configuration` fields,
`setQualityMode`, `enabled`) used with the pmndrs composer.

**Bundle size note:** three.js + postprocessing + n8ao are sizable, so the app
bundle grew accordingly. The viewer is currently imported eagerly by
`DetailsPanel`. **Lazy-loading the viewer** (e.g. `React.lazy` so the three.js
chunk only downloads when the 3D tab is first opened) is a possible future
optimization to keep the initial bundle lean.
