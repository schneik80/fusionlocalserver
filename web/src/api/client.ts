// Thin typed fetch wrapper over the Go JSON API. All endpoints live under
// /api and are same-origin (the SPA is served by the Go server; in dev, Vite
// proxies /api to it). IDs are GraphQL URNs containing ':' and '/', so they
// always travel as query params, never path segments.

import type {
  ActivityReport,
  AuthMe,
  BOMRow,
  Classify,
  ComponentRef,
  Contents,
  Details,
  DrawingRef,
  GroupMember,
  Item,
  Location,
  Meta,
  NamedProperty,
  PhysicalProperties,
  PermLayer,
  Pin,
  ProjectGroup,
  SetPortResponse,
  Thumbnail,
  UploadJob,
  WikiImageResult,
  WikiPage,
  WikiPageContent,
} from './types'

export class ApiError extends Error {
  status: number
  constructor(status: number, message: string) {
    super(message)
    this.name = 'ApiError'
    this.status = status
  }
}

// redirectToLogin sends the browser into the OAuth flow when a data call comes
// back 401 (session lost or expired). Guarded so a burst of parallel 401s only
// navigates once. A full navigation (not fetch) is required so the browser
// follows the server's 302 to Autodesk and accepts the Set-Cookie.
let redirecting = false
function redirectToLogin() {
  if (redirecting) return
  redirecting = true
  window.location.assign('/api/auth/login')
}

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  // Let the browser set the multipart boundary for FormData bodies (image
  // uploads); JSON calls get the explicit content-type.
  const isForm = init?.body instanceof FormData
  const res = await fetch(path, {
    headers: isForm ? undefined : { 'Content-Type': 'application/json' },
    credentials: 'same-origin',
    ...init,
  })
  if (!res.ok) {
    let msg = `request failed (HTTP ${res.status})`
    try {
      const body = (await res.json()) as { error?: string }
      if (body?.error) msg = body.error
    } catch {
      /* non-JSON error body — keep the generic message */
    }
    // A 401 on a data call means the session is gone; bounce to login. The
    // /api/auth/me probe never 401s (it returns 200 with authenticated:false),
    // so this can't loop on the login gate.
    if (res.status === 401) redirectToLogin()
    throw new ApiError(res.status, msg)
  }
  // 204/empty bodies shouldn't happen on our GET endpoints, but guard anyway.
  if (res.status === 204) return undefined as T
  return (await res.json()) as T
}

const qs = (params: Record<string, string | undefined>): string => {
  const sp = new URLSearchParams()
  for (const [k, v] of Object.entries(params)) {
    if (v) sp.set(k, v)
  }
  const s = sp.toString()
  return s ? `?${s}` : ''
}

export const api = {
  meta: () => request<Meta>('/api/meta'),

  authMe: () => request<AuthMe>('/api/auth/me'),

  logout: () => request<void>('/api/auth/logout', { method: 'POST' }),

  setPort: (port: number) =>
    request<SetPortResponse>('/api/settings/port', {
      method: 'POST',
      body: JSON.stringify({ port }),
    }),

  hubs: () => request<Item[]>('/api/hubs'),

  projects: (hubId: string) => request<Item[]>(`/api/projects${qs({ hubId })}`),

  projectContents: (projectId: string) =>
    request<Contents>(`/api/projects/contents${qs({ projectId })}`),

  folderContents: (hubId: string, folderId: string) =>
    request<Item[]>(`/api/folders/contents${qs({ hubId, folderId })}`),

  itemDetails: (hubId: string, itemId: string) =>
    request<Details>(`/api/items/details${qs({ hubId, itemId })}`),

  itemLocation: (hubId: string, itemId: string) =>
    request<Location>(`/api/items/location${qs({ hubId, itemId })}`),

  // fileUrl is the same-origin URL streaming an uploaded (non-native) file's tip
  // bytes, used directly as the src for <img>/<video> and by the PDF viewer. It
  // carries the session cookie as a subresource; the server forwards Range so
  // video/PDF can seek. dmProjectId is the project's altId, itemId its lineage
  // urn. name is the file name *with extension* — the server derives the
  // Content-Type from it, which the response's X-Content-Type-Options: nosniff
  // makes mandatory for <img>/<video> to render (the stored version name can
  // lack the extension).
  fileUrl: (dmProjectId: string, itemId: string, name?: string) =>
    `/api/items/file${qs({ dmProjectId, itemId, name })}`,

  // fileText fetches an uploaded file's bytes as text for the source/code viewer.
  // It returns tooLarge=true — without downloading the body — when the server
  // caps the file (413) or it exceeds maxBytes, so the UI can offer a plain
  // download instead of choking the editor on a huge blob.
  fileText: async (
    dmProjectId: string,
    itemId: string,
    maxBytes = 8 << 20,
  ): Promise<{ text: string; tooLarge: boolean }> => {
    const res = await fetch(api.fileUrl(dmProjectId, itemId), { credentials: 'same-origin' })
    if (res.status === 401) {
      redirectToLogin()
      throw new ApiError(401, 'not authenticated')
    }
    if (res.status === 413) return { text: '', tooLarge: true }
    if (!res.ok) throw new ApiError(res.status, `request failed (HTTP ${res.status})`)
    const len = Number(res.headers.get('Content-Length') || '0')
    if (len && len > maxBytes) return { text: '', tooLarge: true }
    return { text: await res.text(), tooLarge: false }
  },

  uses: (args: { cvId?: string; hubId?: string; drawingItemId?: string }) =>
    request<ComponentRef[]>(`/api/items/uses${qs(args)}`),

  // descendants is the recursive occurrence tree (all child documents), used by
  // the Activity tab's child roll-up.
  descendants: (cvId: string) =>
    request<ComponentRef[]>(`/api/items/descendants${qs({ cvId })}`),

  whereUsed: (cvId: string) =>
    request<ComponentRef[]>(`/api/items/where-used${qs({ cvId })}`),

  drawings: (hubId: string, designItemId: string) =>
    request<DrawingRef[]>(`/api/items/drawings${qs({ hubId, designItemId })}`),

  bom: (cvId: string) => request<BOMRow[]>(`/api/items/bom${qs({ cvId })}`),

  projectGroups: (projectId: string) =>
    request<ProjectGroup[]>(`/api/projects/groups${qs({ projectId })}`),

  groupMembers: (hubId: string, groupId: string) =>
    request<GroupMember[]>(`/api/groups/members${qs({ hubId, groupId })}`),

  classify: (cvId: string) =>
    request<Classify>(`/api/items/classify${qs({ cvId })}`),

  thumbnail: (cvId: string) =>
    request<Thumbnail>(`/api/items/thumbnail${qs({ cvId })}`),

  properties: (cvId: string) =>
    request<PhysicalProperties>(`/api/items/properties${qs({ cvId })}`),

  customProperties: (cvId: string) =>
    request<NamedProperty[]>(`/api/items/custom-properties${qs({ cvId })}`),

  // designActivity reports a single design's activity, sourced from the
  // Manufacturing Data Model GraphQL (the notifications feed rejects this app's
  // token). hubId is the GraphQL hub id and itemId the lineage urn — the same
  // pair the Details endpoints take.
  designActivity: (args: { hubId: string; itemId: string; bucket?: string }) =>
    request<ActivityReport>(
      `/api/activity/report${qs({ scope: 'design', hubId: args.hubId, id: args.itemId, bucket: args.bucket })}`,
    ),

  // rollupActivity merges a design's activity with all of its child documents'
  // activity, computed server-side (bounded concurrency, generous timeout). The
  // caller passes the descendant lineage ids it enumerated.
  rollupActivity: (args: { hubId: string; itemId: string; childItemIds: string[] }) =>
    request<ActivityReport>('/api/activity/rollup', {
      method: 'POST',
      body: JSON.stringify(args),
    }),

  // permissionsPath returns the access at each layer of a document's path
  // (project → folders, root→leaf): groups + individual members with roles.
  permissionsPath: (args: {
    hubId: string
    projectId: string
    projectName?: string
    folders: { id: string; name: string }[]
  }) => {
    const p = new URLSearchParams()
    p.set('hubId', args.hubId)
    p.set('projectId', args.projectId)
    if (args.projectName) p.set('projectName', args.projectName)
    for (const f of args.folders) {
      p.append('folderId', f.id)
      p.append('folderName', f.name)
    }
    return request<PermLayer[]>(`/api/permissions/path?${p.toString()}`)
  },

  // Wiki: published markdown pages in a project's root "Wiki" folder. hubId is
  // the GraphQL hub id (the server resolves it to the DM hub id); dmProjectId is
  // the project's altId. itemId is a page's lineage urn.
  wikiPages: (hubId: string, dmProjectId: string) =>
    request<WikiPage[]>(`/api/wiki/pages${qs({ hubId, dmProjectId })}`),

  wikiPage: (dmProjectId: string, itemId: string) =>
    request<WikiPageContent>(`/api/wiki/page${qs({ dmProjectId, itemId })}`),

  // wikiPublish uploads a page's markdown to the project's Wiki folder. itemId
  // links to an already-published page (empty for a new one); baseVersion + force
  // drive stale-overwrite detection. Returns the resulting page (new tip version).
  wikiPublish: (body: {
    hubId: string
    dmProjectId: string
    itemId?: string
    slug: string
    markdown: string
    baseVersion?: string
    force?: boolean
  }) => request<WikiPage>('/api/wiki/publish', { method: 'POST', body: JSON.stringify(body) }),

  // wikiUploadImage stores an image under Wiki/<slug>/images/ and returns its
  // item id, which wikiImageUrl turns into a same-origin <img> src.
  wikiUploadImage: (fields: { hubId: string; dmProjectId: string; slug: string }, file: File) => {
    const fd = new FormData()
    fd.set('hubId', fields.hubId)
    fd.set('dmProjectId', fields.dmProjectId)
    fd.set('slug', fields.slug)
    fd.set('file', file)
    return request<WikiImageResult>('/api/wiki/image', { method: 'POST', body: fd })
  },

  // wikiRename renames a published page's file (and its images subfolder) to
  // "<newSlug>.md". oldSlug locates the images subfolder; the lineage id is
  // unchanged, so a linked draft keeps its base version.
  wikiRename: (body: {
    hubId: string
    dmProjectId: string
    itemId: string
    oldSlug: string
    newSlug: string
  }) => request<WikiPage>('/api/wiki/rename', { method: 'POST', body: JSON.stringify(body) }),

  wikiImageUrl: (dmProjectId: string, itemId: string) =>
    `/api/wiki/image${qs({ dmProjectId, itemId })}`,

  // Uploads: background file-upload jobs into a project folder. uploadFile
  // spools the bytes to the local server and resolves as soon as the job is
  // accepted; the APS-side transfer continues asynchronously and is observed by
  // polling uploads(). The target fields go into the FormData BEFORE the file —
  // the server reads them streaming and must know the target first. folderPath
  // is the folder-name trail from the project root (empty = project root);
  // projectId/folderId are the GraphQL ids echoed back for cache invalidation.
  uploads: () => request<UploadJob[]>('/api/uploads'),

  uploadFile: (
    fields: {
      hubId: string
      dmProjectId: string
      projectId?: string
      folderId?: string
      folderPath: string[]
    },
    file: File,
  ) => {
    const fd = new FormData()
    fd.set('hubId', fields.hubId)
    fd.set('dmProjectId', fields.dmProjectId)
    if (fields.projectId) fd.set('projectId', fields.projectId)
    if (fields.folderId) fd.set('folderId', fields.folderId)
    fd.set('folderPath', JSON.stringify(fields.folderPath))
    fd.set('file', file) // last: the server stops reading parts after the file
    return request<UploadJob>('/api/uploads', { method: 'POST', body: fd })
  },

  cancelUpload: (id: string) =>
    request<UploadJob[]>(`/api/uploads/cancel${qs({ id })}`, { method: 'POST' }),

  // dismissUploads clears finished jobs (one by id, or all when omitted) and
  // returns the refreshed list.
  dismissUploads: (id?: string) =>
    request<UploadJob[]>(`/api/uploads/dismiss${qs({ id })}`, { method: 'POST' }),

  pins: (hubId: string) => request<Pin[]>(`/api/pins${qs({ hubId })}`),

  addPin: (hubId: string, pin: Partial<Pin>) =>
    request<Pin[]>(`/api/pins${qs({ hubId })}`, {
      method: 'POST',
      body: JSON.stringify(pin),
    }),

  removePin: (hubId: string, id: string) =>
    request<Pin[]>(`/api/pins${qs({ hubId, id })}`, { method: 'DELETE' }),
}
