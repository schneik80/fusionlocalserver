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
} from './types'
import type { ChatChannel, ChatChannelList, ChatMessage, ChatMessageList } from '../chat/types'

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
  const res = await fetch(path, {
    headers: { 'Content-Type': 'application/json' },
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

  // Chat (docs/chat/PLAN.md, phase 1): per-project channels + threaded
  // messages. Reads poll until the SSE stream lands in phase 2.
  chatChannels: (projectId: string) =>
    request<ChatChannelList>(`/api/chat/channels${qs({ projectId })}`),

  chatCreateChannel: (
    projectId: string,
    body: { name: string; topic?: string; isPrivate?: boolean; memberIds?: string[] },
  ) =>
    request<ChatChannel>(`/api/chat/channels${qs({ projectId })}`, {
      method: 'POST',
      body: JSON.stringify(body),
    }),

  chatMessages: (
    projectId: string,
    channelId: string,
    opts?: { beforeSeq?: number; afterSeq?: number; limit?: number },
  ) =>
    request<ChatMessageList>(
      `/api/chat/messages${qs({
        projectId,
        channelId,
        beforeSeq: opts?.beforeSeq ? String(opts.beforeSeq) : undefined,
        afterSeq: opts?.afterSeq !== undefined ? String(opts.afterSeq) : undefined,
        limit: opts?.limit ? String(opts.limit) : undefined,
      })}`,
    ),

  chatSend: (
    projectId: string,
    channelId: string,
    body: { body: string; clientMsgId: string; threadRootSeq?: number },
  ) =>
    request<ChatMessage>(`/api/chat/messages${qs({ projectId, channelId })}`, {
      method: 'POST',
      body: JSON.stringify(body),
    }),

  chatEditMessage: (projectId: string, channelId: string, seq: number, body: string) =>
    request<ChatMessage>(
      `/api/chat/messages${qs({ projectId, channelId, seq: String(seq) })}`,
      { method: 'PATCH', body: JSON.stringify({ body }) },
    ),

  chatDeleteMessage: (projectId: string, channelId: string, seq: number) =>
    request<ChatMessage>(
      `/api/chat/messages${qs({ projectId, channelId, seq: String(seq) })}`,
      { method: 'DELETE' },
    ),

  chatThread: (projectId: string, channelId: string, rootSeq: number) =>
    request<ChatMessageList>(
      `/api/chat/thread${qs({ projectId, channelId, rootSeq: String(rootSeq) })}`,
    ),

  chatReact: (projectId: string, channelId: string, seq: number, emoji: string) =>
    request<ChatMessage>(
      `/api/chat/reactions${qs({ projectId, channelId, seq: String(seq), emoji })}`,
      { method: 'POST' },
    ),

  chatUnreact: (projectId: string, channelId: string, seq: number, emoji: string) =>
    request<ChatMessage>(
      `/api/chat/reactions${qs({ projectId, channelId, seq: String(seq), emoji })}`,
      { method: 'DELETE' },
    ),

  pins: (hubId: string) => request<Pin[]>(`/api/pins${qs({ hubId })}`),

  addPin: (hubId: string, pin: Partial<Pin>) =>
    request<Pin[]>(`/api/pins${qs({ hubId })}`, {
      method: 'POST',
      body: JSON.stringify(pin),
    }),

  removePin: (hubId: string, id: string) =>
    request<Pin[]>(`/api/pins${qs({ hubId, id })}`, { method: 'DELETE' }),
}
