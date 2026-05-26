import {
  useMutation,
  useQuery,
  useQueryClient,
  type UseQueryResult,
} from '@tanstack/react-query'
import { api } from './client'
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

// Data fetched here is effectively static for a browsing session, so cache
// generously and don't refetch on window focus. enabled flags gate queries on
// the required ids being present.
const STALE = 5 * 60 * 1000

export const useMeta = (): UseQueryResult<Meta> =>
  useQuery({ queryKey: ['meta'], queryFn: api.meta, staleTime: Infinity })

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
