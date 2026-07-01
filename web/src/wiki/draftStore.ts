// Device-local wiki drafts, persisted in IndexedDB. This is the "local" tier of
// the two-tier storage model: a user authors markdown here and it never leaves
// the device until they publish it to the project's Wiki folder in Fusion Team.
// Drafts are scoped per project (a browser can hold drafts for many projects).
//
// A raw IndexedDB store (behind a thin promise wrapper) keeps this dependency-
// free; the shape is a single object store keyed by a composite `${projectId}::
// ${pageKey}`, indexed by projectId for per-project listing.

export type DraftStatus =
  | 'draft' // authored locally, never published
  | 'published' // pulled from a published page, unchanged since
  | 'modified' // local edits ahead of the published base

export interface WikiDraft {
  /** composite primary key: `${projectId}::${pageKey}` */
  key: string
  /** GraphQL project id — scopes the draft to its project */
  projectId: string
  /** stable local id: a generated uuid for a new page, or the published itemId once linked */
  pageKey: string
  title: string
  slug: string
  markdown: string
  /** published page lineage urn, once this draft is linked to one (Phase 2) */
  baseItemId?: string
  /** tip version urn the draft was pulled from / last published as (Phase 2) */
  baseVersion?: string
  status: DraftStatus
  /** epoch millis of the last local edit */
  updatedAt: number
}

const DB_NAME = 'fls-wiki'
const DB_VERSION = 1
const STORE = 'drafts'
const PROJECT_INDEX = 'byProject'

export function draftKey(projectId: string, pageKey: string): string {
  return `${projectId}::${pageKey}`
}

// newPageKey mints a stable local id for a brand-new page. crypto.randomUUID is
// available in every browser we target (secure context / localhost).
export function newPageKey(): string {
  return crypto.randomUUID()
}

// slugify turns a title into a filename-safe slug (also the basis for the .md
// name on publish). Kept deterministic so the same title round-trips.
export function slugify(title: string): string {
  return (
    title
      .toLowerCase()
      .trim()
      .replace(/[^a-z0-9]+/g, '-')
      .replace(/^-+|-+$/g, '')
      .slice(0, 80) || 'untitled'
  )
}

let dbPromise: Promise<IDBDatabase> | null = null

function openDB(): Promise<IDBDatabase> {
  if (dbPromise) return dbPromise
  dbPromise = new Promise((resolve, reject) => {
    const req = indexedDB.open(DB_NAME, DB_VERSION)
    req.onupgradeneeded = () => {
      const db = req.result
      if (!db.objectStoreNames.contains(STORE)) {
        const store = db.createObjectStore(STORE, { keyPath: 'key' })
        store.createIndex(PROJECT_INDEX, 'projectId', { unique: false })
      }
    }
    req.onsuccess = () => resolve(req.result)
    req.onerror = () => reject(req.error)
  })
  return dbPromise
}

function tx(db: IDBDatabase, mode: IDBTransactionMode): IDBObjectStore {
  return db.transaction(STORE, mode).objectStore(STORE)
}

// req wraps an IDBRequest as a promise.
function req<T>(r: IDBRequest<T>): Promise<T> {
  return new Promise((resolve, reject) => {
    r.onsuccess = () => resolve(r.result)
    r.onerror = () => reject(r.error)
  })
}

export async function listDrafts(projectId: string): Promise<WikiDraft[]> {
  const db = await openDB()
  const store = tx(db, 'readonly')
  const rows = await req(store.index(PROJECT_INDEX).getAll(projectId))
  return (rows as WikiDraft[]).sort((a, b) => b.updatedAt - a.updatedAt)
}

export async function getDraft(key: string): Promise<WikiDraft | undefined> {
  const db = await openDB()
  return (await req(tx(db, 'readonly').get(key))) as WikiDraft | undefined
}

export async function putDraft(draft: WikiDraft): Promise<void> {
  const db = await openDB()
  await req(tx(db, 'readwrite').put(draft))
}

export async function deleteDraft(key: string): Promise<void> {
  const db = await openDB()
  await req(tx(db, 'readwrite').delete(key))
}
