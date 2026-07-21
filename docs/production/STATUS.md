# Production — status

A light **MES / product planning & tracking** app: the fourth project app beside
Tasks, Wiki and Chat. Branch `Production`.

**The problem it solves.** Paper "travellers" — the order-specific packet that
follows a job through the shop — have one hard failure mode: *which document
version did this run actually use?* Copies circulate, revisions land mid-run, and
the record of what was on the machine is lost. Production answers that by making
every document reference a **version pin** and every run an **immutable freeze**
of the plan it started from.

## Model

| Concept | What it is |
|---|---|
| **Job** | The *as-planned* process: a DAG of Steps. `j<n>` per project. |
| **Step** | A node with a persisted canvas position, version-pinned **plan documents**, and **placeholder** slots for documents supplied per run. |
| **Edge** | A directed Step→Step link. Stored as a flat list on the Job; self-loops, duplicates and cycles are rejected. |
| **Batch** | A dated *as-run* instance. On creation it **freezes** the plan. `b<n>` per job. |
| **Fulfillment** | A version-pinned document supplied into a batch — filling a placeholder, or an extra **as-run artifact** (e.g. on-machine-modified NC code). |
| **Refs** | `fls:task` / `fls:doc` tokens on a batch — related tasks and wiki/hub documents, rendered as live cards. |

### The two invariants

1. **A pin is exact.** `DocSnapshot` stores `versionId` (the DM version urn),
   `versionNumber`, and `rootComponentVersionId` (that version's thumbnail cvId).
   The client sends only a document reference; the server resolves the pin
   (`api.SnapshotDocVersion`) so a version can never be forged. The human version
   number is parsed from the version urn's `?version=N`, which is authoritative
   for *every* item kind — the MDM GraphQL details call is best-effort decoration
   for design thumbnails only (plain files carry no `tipVersion` there, and
   DM-created items may not have propagated to the MDM graph at all).
2. **A run is immutable.** `CreateBatch` deep-copies each step's identity, pinned
   documents and placeholders into `Batch.Steps`. The batch UI renders — and
   `AddFulfillment` validates — against that frozen copy, never the live graph.
   Deleting a step or adding a placeholder for the next run cannot alter, hide, or
   retroactively re-score a batch that already happened.

## Layout

**Backend**
- `production/types.go`, `production/store.go` — one `production.json` per project
  under `<config>/production/<sanitized-projectId>/`. Copies the `tasks/` store
  posture: atomic temp+rename writes, per-project mutex, `.bak` on corruption,
  future-version guard, clone + rollback on save failure. Mutations copy the
  returned object **under the lock**.
- `api/production_snapshot.go` — version-pin resolution (+ `versionBelongsToItem`,
  so an upload may assert the version it just created but never a foreign one).
- `server/handlers_production.go`, `server/dto_production.go`, routes in
  `server/routes.go`. Authorization reuses `chat.Authorizer`: `CapRead` to view,
  `CapPost` to edit, `CapModerate`-or-creator to delete a job/batch.

**Frontend** — `web/src/production/`
- `ProductionApp` → project tab; master/detail over `JobList` + `JobDetail`.
- `JobDetail` has three views: **Flow** (`JobCanvas` — pan/zoom SVG canvas lifted
  from `RelationGraph`, draggable persisted step positions, drag-from-port to
  connect), **List**, and **Batches**.
- `BatchesView` / `BatchDetail` / `BatchTimeline` — prove vs production lanes on a
  time axis (rust-orange `#b7410e`, the History graph's share-lane hue), per-step
  frozen documents, placeholder fulfillment, as-run artifacts, completeness bar.
- `DocSourceButton` supplies a document from **the hub** or **an upload**;
  `PinnedDocChip` renders a pin with its exact version badge and jumps to the
  document via `useGoToDocument`.
- `ProductionScreen` — the cross-project rail screen (`app=production`): runs in
  flight across every project, and jobs you own.

**Cross-linking** — `components/productioncard/`
- `fls:job` / `fls:batch` tokens (`prodref.ts`) unfurl into a `ProductionCard`
  wherever tokens render (chat, wiki, task bodies), opening a read-only
  `ProductionViewDialog`. `ProductionRefDialog` inserts them from the chat
  composer, the wiki toolbar, and task details.

## API

All IDs are query params. `projectId` is the project URN; `jobId`/`batchId` are
per-scope ids.

```
GET    /api/production/jobs          ?projectId                     list + capabilities
POST   /api/production/jobs          ?projectId                     {hubId,projectName,name,description}
PATCH  /api/production/jobs          ?projectId&jobId
DELETE /api/production/jobs          ?projectId&jobId               moderator or creator
GET    /api/production/job           ?projectId&jobId               one job, full graph
GET    /api/production/mine                                         cross-project (no roster check)

POST   /api/production/steps         ?projectId&jobId               {title,description,x,y}
PATCH  /api/production/steps         ?projectId&jobId&stepId        x,y must be sent together
DELETE /api/production/steps         ?projectId&jobId&stepId        also drops incident edges

POST   /api/production/edges         ?projectId&jobId               {from,to} — DAG enforced
DELETE /api/production/edges         ?projectId&jobId&edgeId

POST   /api/production/placeholders  ?projectId&jobId&stepId
PATCH  /api/production/placeholders  ?projectId&jobId&stepId&placeholderId
DELETE /api/production/placeholders  ?projectId&jobId&stepId&placeholderId

POST   /api/production/plandocs      ?projectId&jobId&stepId        {hubId,itemId,dmProjectId,name,kind}
DELETE /api/production/plandocs      ?projectId&jobId&stepId&planDocId

POST   /api/production/batches       ?projectId&jobId               freezes the plan
GET    /api/production/batch         ?projectId&jobId&batchId
PATCH  /api/production/batches       ?projectId&jobId&batchId
DELETE /api/production/batches       ?projectId&jobId&batchId       moderator or creator

POST   /api/production/fulfillments  ?projectId&jobId&batchId       {stepId,placeholderId,…doc,source,isAsRun}
DELETE /api/production/fulfillments  ?projectId&jobId&batchId&fulfillmentId

POST   /api/production/batchrefs     ?projectId&jobId&batchId       {token} — fls:task | fls:doc
DELETE /api/production/batchrefs     ?projectId&jobId&batchId&token
```

Job/step/edge/placeholder/plandoc mutations return the **whole updated job** (it
drops straight into the `['prodJob', …]` cache); batch/fulfillment/ref mutations
return the **affected batch**.

## Shipped

- P1 store + CRUD, P2 flow canvas, P3 plan documents + batches + version pinning,
  P4 upload-to-fulfill + timeline + completeness, P5 cross-project screen.
- A `/code-review` pass at high effort; all 10 reported findings fixed (notably a
  copy-outside-the-lock data race in `CreateJob`/`UpdateJob`, the live-plan batch
  record, tip-following pins on uploads, and `v0` on plain files).

## Known gaps / next

- Uploaded fulfillments land in the **project root**; a `Production/<Job>/<Batch>`
  folder tree needs DM folder creation (`api/wiki_publish.go` has the primitives).
- Pins always freeze the **tip**; pinning an arbitrary historical version is not
  exposed.
- `Placeholder.Kind` and `Fulfillment.Source` are unvalidated for length.
- `SnapshotDocVersion` does two independent calls with no consistency check — a
  save landing between them can pin a mismatched number/thumbnail.
- Efficiency: every mutation deep-clones the whole project file for rollback; the
  canvas re-renders all nodes per mousemove; the jobs list and the selected job
  poll the same payload separately.

## Verifying

```
go build ./... && go test ./...        # store tests cover freeze immutability, DAG cycles, Mine, corruption
cd web && npx tsc --noEmit && npm run build
make run                               # needs APS credentials; HTTPS + login
```

End-to-end: create a job → add and connect steps on the canvas → attach a plan
document → add a placeholder → create a batch → supply the placeholder (browse
and upload) → add an as-run artifact → **publish a new version of the plan
document upstream and confirm the batch still shows the old pinned version while
the plan shows the new tip.**
