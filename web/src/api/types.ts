// TypeScript mirrors of the Go DTOs in server/dto.go. Keep field names in sync
// with the json tags there.

export interface Meta {
  version: string
  region: string
  port: number
  portConfigurable: boolean
  debug?: boolean // server running with -v: reveal developer-only UI affordances
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
  slug?: string // hub slug (e.g. "imallc"), populated for hubs only
}

export interface Contents {
  folders: Item[]
  items: Item[]
}

export interface VersionSummary {
  number: number
  createdOn?: string
  createdBy?: string
  comment?: string
  rootComponentVersionId?: string // per-version cvId for the thumbnail
  isMilestone?: boolean // marks the "release" lane + a dev→release merge edge
  revision?: string // reserved: the "main"/release lane; no API source yet
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
  versionNumber: number
  partNumber?: string
  partDesc?: string
  material?: string
  isMilestone: boolean
  rootComponentVersionId?: string
  versions: VersionSummary[]
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

// PermMember mirrors server.MemberDTO — an individual user with a role + status
// on a project or folder (a contributor / folder member).
export interface PermMember {
  userId: string
  name: string
  email?: string
  role: string
  status?: string // ACTIVE | INACTIVE | PENDING
}

// PermLayer mirrors server.PermLayerDTO — one layer of a document's access path
// (the project, or a folder) with the groups and individual members granted there.
export interface PermLayer {
  type: string // "project" | "folder"
  id: string
  name?: string
  groups: ProjectGroup[]
  members: PermMember[]
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

// --- Activity reports (mirror server/dto_activity.go) ---

export type ActivityScope = 'hub' | 'project' | 'folder' | 'design'
export type ActivityBucket = 'hour' | 'day' | 'month' | 'year'

export interface ActivityActor {
  accountId?: string
  displayName: string
  email?: string
}

export interface ActivityContributor {
  accountId?: string
  displayName: string
  email?: string
  eventCount: number
  firstSeen?: string
  lastSeen?: string
}

export interface ActivityTimeBucket {
  start: string // RFC3339
  count: number
}

export interface ActivityChild {
  type: string // "project" | "folder" | "design"
  id: string
  name: string
  eventCount: number
  lastChange?: string
}

export interface ActivityEvent {
  entityType: string // "design" | "community"
  entityId: string
  entityName: string
  timestamp?: string
  action: string
  actor: ActivityActor
  versionNumber?: number
  projectId?: string
  projectName?: string
  folderUrn?: string
  lineageUrn?: string
  fileType?: string
  webUrl?: string
  views?: number
  comments?: number
  likes?: number
  detail?: string
}

export interface ActivityReport {
  scope: ActivityScope | string
  scopeId?: string
  scopeName?: string
  hubId?: string
  totalEvents: number
  designCount: number
  versionCount: number
  contributorCount: number
  createdOn?: string
  lastChange?: string
  bucket: ActivityBucket | string
  timeline: ActivityTimeBucket[]
  contributors: ActivityContributor[]
  children: ActivityChild[]
  events: ActivityEvent[]
  eventsTruncated: boolean
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
