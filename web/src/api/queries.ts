import {
  useMutation,
  useQuery,
  useQueryClient,
  type UseQueryResult,
} from '@tanstack/react-query'
import { api } from './client'
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
  Pin,
  ProjectGroup,
  Thumbnail,
} from './types'

// Data fetched here is effectively static for a browsing session, so cache
// generously and don't refetch on window focus. enabled flags gate queries on
// the required ids being present.
const STALE = 5 * 60 * 1000

export const useMeta = (): UseQueryResult<Meta> =>
  useQuery({ queryKey: ['meta'], queryFn: api.meta, staleTime: Infinity })

// useAuthMe drives the login gate. It is volatile (logout / session expiry),
// so unlike the browsing data it isn't cached long and doesn't retry — a clean
// "not authenticated" answer should render the login screen immediately.
export const useAuthMe = (): UseQueryResult<AuthMe> =>
  useQuery({ queryKey: ['authMe'], queryFn: api.authMe, staleTime: 0, retry: false })

// useSetPort persists a new listen port and triggers a server restart. There's
// nothing to invalidate — the server rebinds and the caller reconnects on the
// new port.
export const useSetPort = () =>
  useMutation({ mutationFn: (port: number) => api.setPort(port) })

export const useHubs = (): UseQueryResult<Item[]> =>
  useQuery({ queryKey: ['hubs'], queryFn: api.hubs, staleTime: STALE })

export const useProjects = (hubId: string | null): UseQueryResult<Item[]> =>
  useQuery({
    queryKey: ['projects', hubId],
    queryFn: () => api.projects(hubId!),
    enabled: !!hubId,
    staleTime: STALE,
  })

export const useProjectContents = (
  projectId: string | null,
): UseQueryResult<Contents> =>
  useQuery({
    queryKey: ['projectContents', projectId],
    queryFn: () => api.projectContents(projectId!),
    enabled: !!projectId,
    staleTime: STALE,
  })

export const useFolderContents = (
  hubId: string | null,
  folderId: string | null,
): UseQueryResult<Item[]> =>
  useQuery({
    queryKey: ['folderContents', hubId, folderId],
    queryFn: () => api.folderContents(hubId!, folderId!),
    enabled: !!hubId && !!folderId,
    staleTime: STALE,
  })

export const useItemDetails = (
  hubId: string | null,
  itemId: string | null,
): UseQueryResult<Details> =>
  useQuery({
    queryKey: ['details', hubId, itemId],
    queryFn: () => api.itemDetails(hubId!, itemId!),
    enabled: !!hubId && !!itemId,
    staleTime: STALE,
  })

// useClassify drives the per-row async refinement: each design row issues one
// query to upgrade its icon to assembly/part. Concurrency is bounded by the
// browser's per-host connection cap and the server's classify semaphore.
export const useClassify = (cvId: string | undefined): UseQueryResult<Classify> =>
  useQuery({
    queryKey: ['classify', cvId],
    queryFn: () => api.classify(cvId!),
    enabled: !!cvId,
    staleTime: Infinity,
  })

// useThumbnail fetches a component version's thumbnail. APS generates it
// asynchronously, so the first response may be PENDING with no URL; poll every
// 2s until the status settles on SUCCESS or FAILED.
export const useThumbnail = (
  cvId: string | undefined,
  enabled: boolean,
): UseQueryResult<Thumbnail> =>
  useQuery({
    queryKey: ['thumbnail', cvId],
    queryFn: () => api.thumbnail(cvId!),
    enabled: enabled && !!cvId,
    staleTime: Infinity,
    refetchInterval: (q) => {
      const s = q.state.data?.status
      return s === 'SUCCESS' || s === 'FAILED' ? false : 2000
    },
  })

// useProperties fetches a component version's physical (mass) properties.
// Like thumbnails, generation is async, so poll every 2s until the status is
// COMPLETED/FAILED — capped at ~15 polls so a stuck computation doesn't poll
// forever.
export const useProperties = (
  cvId: string | undefined,
  enabled: boolean,
): UseQueryResult<PhysicalProperties> =>
  useQuery({
    queryKey: ['properties', cvId],
    queryFn: () => api.properties(cvId!),
    enabled: enabled && !!cvId,
    staleTime: Infinity,
    refetchInterval: (q) => {
      const s = q.state.data?.status
      if (s === 'COMPLETED' || s === 'FAILED') return false
      if (q.state.dataUpdateCount >= 15) return false
      return 2000
    },
  })

export const useUses = (args: {
  cvId?: string
  hubId?: string
  drawingItemId?: string
  enabled: boolean
}): UseQueryResult<ComponentRef[]> =>
  useQuery({
    queryKey: ['uses', args.cvId, args.hubId, args.drawingItemId],
    queryFn: () =>
      api.uses({ cvId: args.cvId, hubId: args.hubId, drawingItemId: args.drawingItemId }),
    enabled: args.enabled,
    staleTime: STALE,
  })

export const useWhereUsed = (
  cvId: string | undefined,
  enabled: boolean,
): UseQueryResult<ComponentRef[]> =>
  useQuery({
    queryKey: ['whereUsed', cvId],
    queryFn: () => api.whereUsed(cvId!),
    enabled: enabled && !!cvId,
    staleTime: STALE,
  })

export const useDrawings = (
  hubId: string | null,
  designItemId: string | undefined,
  enabled: boolean,
): UseQueryResult<DrawingRef[]> =>
  useQuery({
    queryKey: ['drawings', hubId, designItemId],
    queryFn: () => api.drawings(hubId!, designItemId!),
    enabled: enabled && !!hubId && !!designItemId,
    staleTime: STALE,
  })

export const useCustomProperties = (
  cvId: string | undefined,
  enabled: boolean,
): UseQueryResult<NamedProperty[]> =>
  useQuery({
    queryKey: ['customProperties', cvId],
    queryFn: () => api.customProperties(cvId!),
    enabled: enabled && !!cvId,
    staleTime: STALE,
  })

export const useBOM = (
  cvId: string | undefined,
  enabled: boolean,
): UseQueryResult<BOMRow[]> =>
  useQuery({
    queryKey: ['bom', cvId],
    queryFn: () => api.bom(cvId!),
    enabled: enabled && !!cvId,
    staleTime: STALE,
  })

export const useProjectGroups = (
  projectId: string | null | undefined,
): UseQueryResult<ProjectGroup[]> =>
  useQuery({
    queryKey: ['projectGroups', projectId],
    queryFn: () => api.projectGroups(projectId!),
    enabled: !!projectId,
    staleTime: STALE,
  })

// useGroupMembers loads a group's users lazily (on expand). Listing members
// needs hub-admin access, so a 403 is expected for ordinary users — don't
// retry it, and let the caller render it as "no permission".
export const useGroupMembers = (
  hubId: string | null,
  groupId: string | null,
  enabled: boolean,
): UseQueryResult<GroupMember[]> =>
  useQuery({
    queryKey: ['groupMembers', hubId, groupId],
    queryFn: () => api.groupMembers(hubId!, groupId!),
    enabled: enabled && !!hubId && !!groupId,
    staleTime: STALE,
    retry: false,
  })

export const useItemLocation = (
  hubId: string | null,
  itemId: string | undefined,
  enabled: boolean,
): UseQueryResult<Location> =>
  useQuery({
    queryKey: ['location', hubId, itemId],
    queryFn: () => api.itemLocation(hubId!, itemId!),
    enabled: enabled && !!hubId && !!itemId,
    staleTime: STALE,
  })

// useActivityReport fetches the scoped activity report. `hub` is the hub slug
// (Item.slug). scope/id select the level; the whole hub feed is fetched and
// aggregated server-side, so changing scope/id is cheap and cache-friendly.
export const useActivityReport = (
  hub: string | null | undefined,
  scope: string,
  id: string,
  bucket: string,
): UseQueryResult<ActivityReport> =>
  useQuery({
    queryKey: ['activityReport', hub, scope, id, bucket],
    queryFn: () => api.activityReport({ hub: hub!, scope, id: id || undefined, bucket }),
    enabled: !!hub,
    staleTime: STALE,
  })

// useDesignActivity fetches one design's activity report (GraphQL-sourced).
// hubId is the GraphQL hub id and itemId the lineage urn — the same pair the
// Details endpoints use. Daily buckets; the heatmap re-buckets client-side.
export const useDesignActivity = (
  hubId: string | null | undefined,
  itemId: string | null | undefined,
): UseQueryResult<ActivityReport> =>
  useQuery({
    queryKey: ['designActivity', hubId, itemId],
    queryFn: () => api.designActivity({ hubId: hubId!, itemId: itemId!, bucket: 'day' }),
    enabled: !!hubId && !!itemId,
    staleTime: STALE,
  })

export const usePins = (hubId: string | null): UseQueryResult<Pin[]> =>
  useQuery({
    queryKey: ['pins', hubId],
    queryFn: () => api.pins(hubId!),
    enabled: !!hubId,
    staleTime: STALE,
  })

export function usePinMutations(hubId: string | null) {
  const qc = useQueryClient()
  const invalidate = () => qc.invalidateQueries({ queryKey: ['pins', hubId] })

  const add = useMutation({
    mutationFn: (pin: Partial<Pin>) => api.addPin(hubId!, pin),
    onSuccess: (pins) => {
      qc.setQueryData(['pins', hubId], pins)
      void invalidate()
    },
  })
  const remove = useMutation({
    mutationFn: (id: string) => api.removePin(hubId!, id),
    onSuccess: (pins) => {
      qc.setQueryData(['pins', hubId], pins)
      void invalidate()
    },
  })
  return { add, remove }
}
