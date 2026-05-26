// Thin typed fetch wrapper over the Go JSON API. All endpoints live under
// /api and are same-origin (the SPA is served by the Go server; in dev, Vite
// proxies /api to it). IDs are GraphQL URNs containing ':' and '/', so they
// always travel as query params, never path segments.

import type {
  Classify,
  ComponentRef,
  Contents,
  Details,
  DrawingRef,
  Item,
  Location,
  Meta,
  Pin,
} from './types'

export class ApiError extends Error {
  status: number
  constructor(status: number, message: string) {
    super(message)
    this.name = 'ApiError'
    this.status = status
  }
}

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const res = await fetch(path, {
    headers: { 'Content-Type': 'application/json' },
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

  whereUsed: (cvId: string) =>
    request<ComponentRef[]>(`/api/items/where-used${qs({ cvId })}`),

  drawings: (hubId: string, designItemId: string) =>
    request<DrawingRef[]>(`/api/items/drawings${qs({ hubId, designItemId })}`),

  classify: (cvId: string) =>
    request<Classify>(`/api/items/classify${qs({ cvId })}`),

  pins: (hubId: string) => request<Pin[]>(`/api/pins${qs({ hubId })}`),

  addPin: (hubId: string, pin: Partial<Pin>) =>
    request<Pin[]>(`/api/pins${qs({ hubId })}`, {
      method: 'POST',
      body: JSON.stringify(pin),
    }),

  removePin: (hubId: string, id: string) =>
    request<Pin[]>(`/api/pins${qs({ hubId, id })}`, { method: 'DELETE' }),
}
