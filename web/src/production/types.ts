// TypeScript mirrors of the production DTOs. Keep in sync with
// server/dto_production.go. A Job carries its full graph (steps, edges,
// batches) so one fetch hydrates the whole canvas.

export interface ProdUser {
  id: string
  name?: string
  email?: string
}

// A version-pinned document reference. Unlike an fls:doc token (which follows
// the tip), a ProdDoc freezes the exact version: versionId + versionNumber +
// rootComponentVersionId render that specific version's badge and thumbnail.
export interface ProdDoc {
  hubId: string
  itemId: string
  name: string
  kind?: string
  versionId: string
  versionNumber: number
  rootComponentVersionId?: string
  dmProjectId?: string
}

export interface ProdPlanDoc {
  id: string
  doc: ProdDoc
  addedBy: ProdUser
  addedAt: string
}

export interface ProdPlaceholder {
  id: string
  label: string
  kind?: string
  required: boolean
}

export interface ProdStep {
  id: string
  num: number
  title: string
  description?: string
  x: number
  y: number
  planDocs: ProdPlanDoc[]
  placeholders: ProdPlaceholder[]
  createdAt: string
  updatedAt: string
}

export interface ProdEdge {
  id: string
  from: string
  to: string
}

// A frozen step within a batch: identity, pinned plan docs, and placeholder
// slots as they stood at batch creation. The batch record renders from these,
// never the live plan.
export interface ProdBatchStep {
  stepId: string
  num: number
  title: string
  planDocs: ProdPlanDoc[]
  placeholders: ProdPlaceholder[]
}

export interface ProdFulfillment {
  id: string
  stepId: string
  placeholderId?: string
  doc: ProdDoc
  source?: string
  isAsRun: boolean
  suppliedBy: ProdUser
  suppliedAt: string
}

export interface ProdBatch {
  id: string
  num: number
  name: string
  kind: string // 'prove' | 'production'
  runAt: string
  status: string // 'planned' | 'running' | 'complete'
  steps: ProdBatchStep[]
  fulfillments: ProdFulfillment[]
  refs: string[] // fls:task / fls:doc tokens — related tasks & documents
  createdBy: ProdUser
  createdAt: string
  updatedAt: string
}

export interface Job {
  id: string
  num: number
  projectId: string
  hubId: string
  projectName: string
  name: string
  description?: string
  steps: ProdStep[]
  edges: ProdEdge[]
  batches: ProdBatch[]
  createdBy: ProdUser
  createdAt: string
  updatedAt: string
}

export interface ProdCaps {
  write: boolean
  moderate: boolean
}

export interface JobList {
  jobs: Job[]
  capabilities: ProdCaps
}

// MyProduction is GET /api/production/mine — cross-project, so each job carries
// its own projectId/hubId/projectName.
export interface MyProduction {
  jobs: Job[]
}

// ---- request payloads ----

export interface JobDraft {
  hubId: string
  projectName: string
  name: string
  description?: string
}

export interface JobPatch {
  name?: string
  description?: string
}

export interface StepDraft {
  title: string
  description?: string
  x?: number
  y?: number
}

export interface StepPatch {
  title?: string
  description?: string
  x?: number
  y?: number
}

export interface PlaceholderDraft {
  label: string
  kind?: string
  required?: boolean
}

export interface PlaceholderPatch {
  label?: string
  kind?: string
  required?: boolean
}

// DocPin is a client-side document reference. The server resolves the exact
// version from it (hubId/itemId/dmProjectId); name/kind are display hints.
// versionId is optional: the upload path passes the version urn the upload
// just created so the pin records that version; the server rejects a urn that
// doesn't belong to the item's lineage. Absent → the current tip is pinned.
export interface DocPin {
  hubId: string
  itemId: string
  dmProjectId: string
  name: string
  kind?: string
  versionId?: string
}

export interface FulfillInput extends DocPin {
  stepId: string
  placeholderId?: string
  source?: string // 'hub' | 'upload'
  isAsRun?: boolean
}

export interface BatchDraft {
  name: string
  kind?: string // 'prove' | 'production'
  runAt?: string // RFC3339; defaults to now
}

export interface BatchPatch {
  name?: string
  kind?: string
  status?: string
  runAt?: string
}

// UI constants
export const BATCH_KINDS = ['prove', 'production'] as const
export const BATCH_STATUSES = ['planned', 'running', 'complete'] as const

export const jobDisplayId = (j: { num: number }) => `J-${j.num}`
