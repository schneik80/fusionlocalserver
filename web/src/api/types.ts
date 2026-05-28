// TypeScript mirrors of the Go DTOs in server/dto.go. Keep field names in sync
// with the json tags there.

export interface Meta {
  version: string
  region: string
  port: number
  portConfigurable: boolean
}

// AuthMe mirrors server.AuthMeDTO (GET /api/auth/me): the login-state probe.
export interface AuthUser {
  name: string
  email: string
}

export interface AuthMe {
  authenticated: boolean
  user?: AuthUser
}

export interface SetPortResponse {
  port: number
  restarting: boolean
}

export type ItemKind =
  | 'hub'
  | 'project'
  | 'folder'
  | 'design'
  | 'configured'
  | 'drawing'
  | 'schematic'
  | 'pcb'
  | 'ecad'
  | 'unknown'

export interface Item {
  id: string
  name: string
  kind: ItemKind | string
  altId?: string
  webUrl?: string
  isContainer: boolean
  componentVersionId?: string
  subtype?: string // "assembly" | "part" | "dwg" | "template" | ""
  lastModifiedOn?: string // RFC3339; populated by the server on items/folders rows
}

export interface Contents {
  folders: Item[]
  items: Item[]
}

// HistoryEntry mirrors server.HistoryEntryDTO — one entry in an item's
// time-based change log. v3 has no integer version numbers; entries are keyed
// by timestamp + id and labelled by change type.
export interface HistoryEntry {
  id: string
  timestamp?: string
  changeType?: string
  description?: string
  author?: string
}

export interface Details {
  id: string
  name: string
  typename: string
  size?: string
  mimeType?: string
  extensionType?: string
  fusionWebUrl?: string
  createdOn?: string
  createdBy?: string
  modifiedOn?: string
  modifiedBy?: string
  partNumber?: string
  partDesc?: string
  material?: string
  rootComponentVersionId?: string
  tipTimestamp?: string
  history: HistoryEntry[]
}

// Thumbnail mirrors server.ThumbnailDTO. status is the async generation state
// ("PENDING" | "SUCCESS" | "FAILED"); signedUrl is set only once SUCCESS.
export interface Thumbnail {
  status: string
  signedUrl?: string
}

// Measure / PhysicalProperties mirror the v2 physical-properties DTOs.
// status is "COMPLETED" | "FAILED" | (computing).
export interface Measure {
  display?: string
  units?: string
}

export interface PhysicalProperties {
  status: string
  area: Measure
  volume: Measure
  mass: Measure
  density: Measure
  bboxLength: Measure
  bboxWidth: Measure
  bboxHeight: Measure
}

// NamedProperty mirrors server.NamedPropertyDTO — a custom/standard property
// (name + display value) shown in the Details Properties tab.
export interface NamedProperty {
  name: string
  value: string
}

export interface ComponentRef {
  id: string
  name: string
  partNumber?: string
  partDesc?: string
  material?: string
  designItemId?: string
  designItemName?: string
  fusionWebUrl?: string
}

// BOMRow mirrors server.BOMRowDTO — one line of a design's bill of materials.
// quantity is the occurrence count (the v2 API has no explicit quantity field).
export interface BOMRow {
  componentVersionId: string
  name: string
  partNumber?: string
  partDesc?: string
  material?: string
  quantity: number
}

export interface DrawingRef {
  id: string
  name: string
  drawingItemId: string
  modifiedOn?: string
  modifiedBy?: string
  fusionWebUrl?: string
}

// ProjectGroup mirrors server.ProjectGroupDTO — a group with access to the
// item's project, and its role.
export interface ProjectGroup {
  id: string
  name: string
  role: string
}

// GroupMember mirrors server.GroupMemberDTO — a user in a group (listable only
// with hub-admin access; otherwise the members request returns 403).
export interface GroupMember {
  userId: string
  name: string
  email?: string
  status?: string
}

// SearchHit mirrors server.SearchHitDTO — one row in the hub search results.
// itemId + hubId identify the navigable document for Show-in-Location.
export interface SearchHit {
  name: string
  thumbnailUrl?: string
  matched?: string
  itemId?: string
  hubId?: string
  kind: string
}

export interface SearchResponse {
  hits: SearchHit[]
  nextCursor?: string
}

// SearchableProperty mirrors server.SearchablePropertyDTO — one option in the
// search form's property picker; id is the propertyDefinition id.
export interface SearchableProperty {
  displayName: string
  id: string
}

export interface FolderRef {
  id: string
  name: string
}

export interface Location {
  hubId: string
  projectId: string
  projectAltId?: string
  projectName: string
  folderPath: FolderRef[]
}

export interface Classify {
  componentVersionId: string
  isAssembly: boolean
  subtype: string // "assembly" | "part"
}

// Pin mirrors pins.Pin (snake_case json tags, unlike the camelCase DTOs).
export interface Pin {
  id: string
  name: string
  kind: string
  hub_id: string
  project_id?: string
  project_alt_id?: string
  folder_path?: FolderRef[]
  pinned_at?: string
}
