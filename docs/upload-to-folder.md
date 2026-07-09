# Upload Files to a Folder — APIs & Method

A shareable summary of how **fusionlocalserver** uploads a file into a Fusion Team
project folder. Covers the Autodesk Platform Services (APS) APIs it calls, the
exact request sequence, and how the capability is wired across the Go backend and
the React frontend.

> **Status: shipped.** This is not a proposal — the feature exists end to end
> (drag-and-drop upload with a live progress list). This doc describes the working
> implementation so it can be reviewed, reused, or extended.

---

## 1. What it does

The user drops a file onto a folder (or the project root) in the browser. The
server accepts the bytes, streams them to APS storage, and registers them as a
new **item** (first upload) or a new **version** (a file of the same name already
exists in that folder — matching Fusion Team's own semantics). Large files are
split into multiple signed-S3 parts. The upload runs as a **background job**; the
UI polls a job list to render a progress bar.

---

## 2. APS APIs used

Two APS surfaces are involved. All calls are 3-legged (the signed-in user's
token); required scopes are `data:create data:write` (plus the `data:read` the app
already holds). Base host: `https://developer.api.autodesk.com`.

| Purpose | API | Endpoint |
|---|---|---|
| Create a storage location | Data Management v1 | `POST /data/v1/projects/{projectId}/storage` |
| Request signed upload URL(s) | OSS v2 (direct-to-S3) | `GET /oss/v2/buckets/{bucket}/objects/{object}/signeds3upload?parts=N` |
| Upload the bytes | Amazon S3 | `PUT <signed url>` (one per part) |
| Finalize the object | OSS v2 | `POST /oss/v2/buckets/{bucket}/objects/{object}/signeds3upload` |
| Create the first item + version | Data Management v1 | `POST /data/v1/projects/{projectId}/items` |
| Add a version to an existing item | Data Management v1 | `POST /data/v1/projects/{projectId}/versions` |
| Resolve the target folder | Data Management (project-v1 + data-v1) | `GET /project/v1/hubs/{hub}/projects/{pid}/topFolders`, `GET /data/v1/projects/{pid}/folders/{fid}/contents` |

Notes:
- The bytes go **directly to S3** on the presigned `PUT` — that request carries no
  `Authorization` header. All other calls send `Authorization: Bearer <token>`.
- DM write bodies are **JSON:API** documents. This codebase posts them with
  `Content-Type: application/json` (works); PATCH/rename uses
  `application/vnd.api+json`.
- OSS v1 direct upload is deprecated; the signed-S3 (v2) flow above is the current,
  supported path.

---

## 3. The upload sequence (the "method")

For a file of `size` bytes going into folder `folderID` of project `dmProjectID`:

1. **Look for a same-named item** in the target folder
   (`GET .../folders/{fid}/contents`). If found, this becomes a *new version*;
   otherwise a *new item*.
2. **Create a storage location** — `POST /data/v1/projects/{pid}/storage`:
   ```jsonc
   { "jsonapi": {"version":"1.0"},
     "data": { "type":"objects",
               "attributes": {"name":"<filename>"},
               "relationships": {"target": {"data": {"type":"folders","id":"<folderID>"}}} } }
   ```
   Response `data.id` is an OSS object URN:
   `urn:adsk.objects:os.object:<bucket>/<object>`.
3. **Request signed upload URL(s)** — `GET /oss/v2/buckets/{bucket}/objects/{object}/signeds3upload?parts=N`.
   Response: `{ "uploadKey": "...", "urls": [ ... ] }` — one S3 URL per part.
4. **PUT each part to its S3 URL** (no bearer token; sets `Content-Type` from the
   filename). Byte progress is reported as each part streams up.
5. **Finalize** — `POST .../signeds3upload` with `{ "uploadKey": "<key>" }`. This
   registers the completed object in OSS.
6. **Create the item or version**, pointing its `storage` relationship at the object
   URN from step 2:
   - **New item** — `POST /data/v1/projects/{pid}/items` with `data` (type `items`,
     `tip` → version `"1"`, `parent` → the folder) **and** an `included` version
     (type `versions`, `storage` → object URN).
   - **New version** — `POST /data/v1/projects/{pid}/versions` with `data`
     (type `versions`, `relationships.item` → existing item id, `storage` → object
     URN). *(There is no `items/{id}/versions` create endpoint — posting there
     404s.)*

Returns the item **lineage URN** and the resulting **version URN**.

### Part sizing
- Files ≤ **64 MiB** upload as a single part.
- Larger files split into equal 64 MiB parts.
- A single `signeds3upload` request returns at most **25** URLs; beyond
  25 × 64 MiB the part *size* grows instead of the part *count*.

---

## 4. ID spaces — the important wrinkle

The app browses via the Manufacturing Data Model **GraphQL** API but writes via the
Data Management **REST** API, and **their ids are not interchangeable**.

- **Hub / project ids** translate through GraphQL `alternativeIdentifiers`. DM URLs
  want the `b.`-prefixed project id and the DM hub id (`GetHubDataManagementID`).
- **Folder ids do *not* translate** — a MFGDM GraphQL folder id can't be mapped to a
  DM folder id. So the target folder is resolved by **walking display names from the
  project root** in DM space: `topFolders` → repeatedly fetch folder `contents` and
  match the next name in the path. An empty path means the project root.
- **URN shapes** seen in the flow:
  - OSS object: `urn:adsk.objects:os.object:<bucket>/<object>`
  - Item lineage: `urn:adsk.wipprod:dm.lineage:…`
- URNs contain `:` and `/`, so ids travel as **query params** (URL-escaped with
  `url.QueryEscape`), never as path segments.

---

## 5. Backend architecture

Because a large transfer can outlive a single request *and* a single access token,
the upload is a background job, not a synchronous handler.

- **Accept (`POST /api/uploads`, multipart):** the target fields (`hubId`,
  `dmProjectId`, `folderPath`, plus optional `projectId`/`folderId` GraphQL-id
  echoes for cache invalidation) **must precede the file part** so the target is
  known before the bytes arrive. The file is **spooled to a temp file** (never held
  in RAM; capped at 10 GiB), a job is queued, and the handler returns **`202
  Accepted`** immediately. The APS transfer proceeds asynchronously.
- **Token refresh across the job:** the job holds the whole `*Session` and calls a
  `TokenSource` closure (`func(ctx) (token, error)`) that re-fetches a *fresh* token
  before each authenticated APS call — the S3 phase can outlast the token that
  signed the request.
- **Streaming, not buffering:** bytes are read from the temp file via
  `io.ReaderAt`/`SectionReader` per part; a `progressReader` forwards byte deltas so
  the UI can show a live bar.
- **Concurrency & lifecycle:** jobs wait on a semaphore for an upload slot; each has
  a timeout and is cancelable; the temp file is always removed on completion.
- **Observe/cancel/dismiss:** `GET /api/uploads`, `POST /api/uploads/cancel?id=`,
  `POST /api/uploads/dismiss?id=` (id optional → clears all finished jobs). Errors
  are redacted to a category by default (full detail only under `-v`).

### Route table
```
POST /api/uploads           multipart create → 202 + job DTO
GET  /api/uploads           list this session's jobs
POST /api/uploads/cancel    ?id=<job id>
POST /api/uploads/dismiss   ?id=<job id, optional>
```

---

## 6. Web layer

- **`api.uploadFile(fields, file)`** builds a `FormData` and sets the target fields
  **before** `file` (the browser sets the multipart boundary; the server reads parts
  streaming and stops after the file). `folderPath` is the folder-name trail from the
  project root (`[]` = root).
- Uploads are **not** in `queries.ts`; they live in a dedicated context provider
  (`web/src/state/uploads.tsx`) that polls `api.uploads()` via TanStack Query,
  optimistically appends each started job to the `['uploads']` cache, and invalidates
  affected folder/project queries on completion.
- UI: `web/src/components/UploadDialog.tsx`.

---

## 7. Key files

| Layer | File | Role |
|---|---|---|
| API | `api/upload.go` | `UploadFileToFolder`, `ResolveFolderPath`, streaming multi-part OSS upload, `TokenSource` |
| API | `api/wiki_publish.go` | Reusable DM write primitives: `createStorage`, `createItem`, `createVersion`, `createFolder`, buffered `uploadToOSS`, `dmPost`/`dmPatch`, `dataID` |
| API | `api/datamanagement.go`, `api/wiki.go` | REST helpers (`dmGet`, `dmEscape`), id/URN translation, `parseOSSObjectURN`, `dmTopFolders`, `findSubfolderID` |
| Server | `server/handlers_upload.go` | Multipart accept, temp spool, job creation |
| Server | `server/uploads.go` | Background job runner, semaphore, token-refresh closure |
| Server | `server/dto_upload.go`, `server/routes.go` | Job DTO, route registration |
| Web | `web/src/api/client.ts` | `uploadFile`, `uploads`, `cancelUpload`, `dismissUploads` |
| Web | `web/src/state/uploads.tsx`, `web/src/components/UploadDialog.tsx` | Job state + UI |

---

## 8. Reuse & gotchas

- **Reuse the DM primitives.** `createStorage` → `uploadToOSS[Stream]` →
  `createItem`/`createVersion` is the canonical DM write chain; `uploadFile`
  (buffered) and `UploadFileToFolder` (streaming) are two callers of the same
  sequence. New mutations should build on these rather than reissuing raw requests.
- **Item vs. version is decided by name match** in the destination folder, so
  re-uploading a file adds a version instead of a duplicate — intended.
- **Don't try to reuse GraphQL folder ids for writes** — resolve the folder in DM
  space by name from the root.
- **Signed PUTs are unauthenticated** to APS (S3 presigns them); everything else
  needs a bearer token, and the token must be re-fetched per call for long jobs.
- **Finalize is mandatory** — without the `POST …/signeds3upload { uploadKey }`
  step the object is never registered and item/version creation will fail.
- Call the finalize/complete step within APS's upload window (the signed URLs and
  upload key are time-limited).

---

*References: APS Data Management API v2 — "Upload a File" tutorial; OSS
direct-to-S3 signed-upload flow (`signeds3upload`). Endpoints and payloads above are
mirrored from this repo's working implementation.*
