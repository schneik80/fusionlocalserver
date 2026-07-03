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
  PermLayer,
  PhysicalProperties,
  Pin,
  ProjectGroup,
  Thumbnail,
} from './types'
import type { ChatChannelList, ChatMessageList } from '../chat/types'
import {
  appendPendingMessage,
  removePendingMessage,
  upsertMessage,
} from '../chat/cache'

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

// useDescendants fetches the recursive occurrence tree (all child documents).
// Heavier than useUses (one level) — backs the Activity tab's child roll-up.
export const useDescendants = (
  cvId: string | undefined,
  enabled: boolean,
): UseQueryResult<ComponentRef[]> =>
  useQuery({
    queryKey: ['descendants', cvId],
    queryFn: () => api.descendants(cvId!),
    enabled: enabled && !!cvId,
    staleTime: STALE,
  })

// usePermissionsPath fetches per-layer access (project → folders) for a document.
export const usePermissionsPath = (
  hubId: string | null,
  projectId: string | null | undefined,
  projectName: string | undefined,
  folders: { id: string; name: string }[],
  enabled: boolean,
): UseQueryResult<PermLayer[]> =>
  useQuery({
    queryKey: ['permPath', hubId, projectId, folders.map((f) => f.id)],
    queryFn: () => api.permissionsPath({ hubId: hubId!, projectId: projectId!, projectName, folders }),
    enabled: enabled && !!hubId && !!projectId,
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

// useRollupActivity computes a design's activity merged with all of its child
// documents, server-side. Enabled only while rolled up; staleTime 0 so each
// enable fetches fresh (child docs may change in real time) and keyed by the
// child-id set so a result is never reused for a different document. While the
// document stays mounted, Day/Week/Month/Year flips (handled inside the heat map)
// don't refetch.
export const useRollupActivity = (
  hubId: string | null | undefined,
  itemId: string | null | undefined,
  childItemIds: string[],
  enabled: boolean,
): UseQueryResult<ActivityReport> =>
  useQuery({
    queryKey: ['rollupActivity', hubId, itemId, [...childItemIds].sort().join(',')],
    queryFn: () => api.rollupActivity({ hubId: hubId!, itemId: itemId!, childItemIds }),
    enabled: enabled && !!hubId && !!itemId,
    staleTime: 0,
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

// ---- chat (docs/chat/PLAN.md, phases 1 & 3) ----
// Chat is deliberately volatile: nothing here persists to localStorage (see
// the dehydrate filter in main.tsx). The SSE stream (useChatEvents, mounted
// in ProjectPanel) writes events straight into these caches; `live` demotes
// the phase-1 polling to a FALLBACK that re-arms only while the stream is
// down. `active` further gates everything to the visible Chat tab.

export const useChatChannels = (
  projectId: string | null,
  active: boolean,
  live: boolean,
): UseQueryResult<ChatChannelList> =>
  useQuery({
    queryKey: ['chatChannels', projectId],
    queryFn: () => api.chatChannels(projectId!),
    enabled: active && !!projectId,
    staleTime: 0,
    refetchInterval: active && !live ? 10_000 : false,
  })

export const useChatMessages = (
  projectId: string | null,
  channelId: string | null,
  active: boolean,
  live: boolean,
): UseQueryResult<ChatMessageList> =>
  useQuery({
    queryKey: ['chatMessages', projectId, channelId],
    queryFn: () => api.chatMessages(projectId!, channelId!),
    enabled: active && !!projectId && !!channelId,
    staleTime: 0,
    refetchInterval: active && !live ? 2000 : false,
  })

export const useChatThread = (
  projectId: string | null,
  channelId: string | null,
  rootSeq: number | null,
  active: boolean,
  live: boolean,
): UseQueryResult<ChatMessageList> =>
  useQuery({
    queryKey: ['chatThread', projectId, channelId, rootSeq],
    queryFn: () => api.chatThread(projectId!, channelId!, rootSeq!),
    enabled: active && !!projectId && !!channelId && rootSeq !== null,
    staleTime: 0,
    refetchInterval: active && !live ? 2000 : false,
  })

// useChatMutations bundles the write paths for one channel.
//
// Sends are optimistic (design doc §5): a pending copy keyed on a fresh
// clientMsgId appears immediately with a negative placeholder seq; the
// server's echo — REST response or SSE message.created, whichever lands
// first — replaces it via the clientMsgId match in upsertMessage. A failed
// send removes the pending copy (the composer surfaces the error). The
// clientMsgId also makes retries safe server-side: a replayed POST returns
// the original message instead of double-posting.
export function useChatMutations(projectId: string | null, channelId: string | null) {
  const qc = useQueryClient()
  const me = useAuthMe().data?.user

  const sendMutation = useMutation({
    mutationFn: (args: { body: string; clientMsgId: string; threadRootSeq?: number }) =>
      api.chatSend(projectId!, channelId!, {
        body: args.body,
        clientMsgId: args.clientMsgId,
        threadRootSeq: args.threadRootSeq,
      }),
    onMutate: (args) => {
      appendPendingMessage(qc, projectId!, channelId!, {
        seq: -Date.now(), // unique placeholder; replaced by the echo
        threadRoot: args.threadRootSeq,
        authorId: me?.id ?? '',
        authorName: me?.name || me?.email || 'me',
        clientMsgId: args.clientMsgId,
        body: args.body,
        createdAt: new Date().toISOString(),
        deleted: false,
        replyCount: 0,
        reactions: [],
        pending: true,
      })
    },
    onSuccess: (msg) => upsertMessage(qc, projectId!, channelId!, msg),
    onError: (_err, args) => removePendingMessage(qc, projectId!, channelId!, args.clientMsgId),
  })
  const send = (body: string, threadRootSeq?: number) =>
    sendMutation.mutateAsync({ body, clientMsgId: crypto.randomUUID(), threadRootSeq })

  const edit = useMutation({
    mutationFn: (args: { seq: number; body: string }) =>
      api.chatEditMessage(projectId!, channelId!, args.seq, args.body),
    onSuccess: (msg) => upsertMessage(qc, projectId!, channelId!, msg),
  })
  const remove = useMutation({
    mutationFn: (seq: number) => api.chatDeleteMessage(projectId!, channelId!, seq),
    onSuccess: (msg) => upsertMessage(qc, projectId!, channelId!, msg),
  })
  const react = useMutation({
    mutationFn: (args: { seq: number; emoji: string; on: boolean }) =>
      args.on
        ? api.chatReact(projectId!, channelId!, args.seq, args.emoji)
        : api.chatUnreact(projectId!, channelId!, args.seq, args.emoji),
    onSuccess: (msg) => upsertMessage(qc, projectId!, channelId!, msg),
  })
  return { send, sending: sendMutation.isPending, edit, remove, react }
}

export const useCreateChatChannel = (projectId: string | null) => {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (body: { name: string; topic?: string; isPrivate?: boolean }) =>
      api.chatCreateChannel(projectId!, body),
    onSuccess: () => void qc.invalidateQueries({ queryKey: ['chatChannels', projectId] }),
  })
}
