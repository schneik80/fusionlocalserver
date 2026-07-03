import type { QueryClient } from '@tanstack/react-query'
import { clearTyping, noteTyping } from './typing'
import type {
  ChatActivityEvent,
  ChatChannel,
  ChatChannelEvent,
  ChatChannelList,
  ChatEvent,
  ChatMessage,
  ChatMessageEvent,
  ChatMessageList,
  ChatTypingEvent,
  ChatUnread,
  ChatUnreadList,
} from './types'

// Cache appliers: SSE events and optimistic sends both write straight into
// the react-query caches (['chatChannels', pid], ['chatMessages', pid, cid],
// ['chatThread', pid, cid, rootSeq], ['chatUnreads', pid]) instead of
// invalidating — the stream IS the refetch. Caches that don't exist yet are
// left alone; the eventual initial fetch is authoritative.

// applyChatEvent folds one server event into the caches.
export function applyChatEvent(qc: QueryClient, projectId: string, ev: ChatEvent) {
  switch (ev.type) {
    case 'message.created': {
      const { channelId, message } = ev.data as ChatMessageEvent
      upsertMessage(qc, projectId, channelId, message)
      if (message.threadRoot) bumpRootCounters(qc, projectId, channelId, message, +1)
      clearTyping(projectId, channelId, message.authorId)
      break
    }
    case 'message.updated':
    case 'reaction.added':
    case 'reaction.removed': {
      const { channelId, message } = ev.data as ChatMessageEvent
      upsertMessage(qc, projectId, channelId, message)
      break
    }
    case 'message.deleted': {
      const { channelId, message } = ev.data as ChatMessageEvent
      upsertMessage(qc, projectId, channelId, message)
      if (message.threadRoot) bumpRootCounters(qc, projectId, channelId, message, -1)
      break
    }
    case 'channel.created':
    case 'channel.updated':
    case 'channel.archived': {
      const { channel } = ev.data as ChatChannelEvent
      upsertChannel(qc, projectId, channel)
      // A new channel needs an unread entry; an archived one must drop its
      // badge. The summary is cheap — refetch rather than reconstruct.
      void qc.invalidateQueries({ queryKey: ['chatUnreads', projectId] })
      break
    }
    case 'channel.member_added':
    case 'channel.member_removed':
      // Membership changes what THIS user may see (they might be the
      // subject) — let the server recompute the visible list.
      void qc.invalidateQueries({ queryKey: ['chatChannels', projectId] })
      void qc.invalidateQueries({ queryKey: ['chatUnreads', projectId] })
      break
    case 'channel.activity': {
      const { channelId, lastMessageSeq } = ev.data as ChatActivityEvent
      bumpUnread(qc, projectId, channelId, lastMessageSeq)
      break
    }
    case 'read.updated':
      // User-only event: this user marked a channel read in some tab; the
      // payload is the server-computed unread summary for that channel.
      applyUnread(qc, projectId, ev.data as ChatUnread)
      break
    case 'typing':
      noteTyping(projectId, ev.data as ChatTypingEvent)
      break
    default:
      // Unknown (future) event types are ignorable here.
      break
  }
}

// upsertMessage inserts or replaces a message in the timeline and any open
// thread cache. Matching is by seq, falling back to clientMsgId so a
// server echo replaces the optimistic pending copy (whose seq is a
// negative placeholder).
export function upsertMessage(
  qc: QueryClient,
  projectId: string,
  channelId: string,
  msg: ChatMessage,
) {
  if (!msg.threadRoot) {
    qc.setQueryData<ChatMessageList>(['chatMessages', projectId, channelId], (cur) =>
      cur ? upsertIntoList(cur, msg) : cur,
    )
  }
  // Thread caches hold the root message too, so top-level edits also patch
  // the thread keyed by their own seq.
  const rootSeq = msg.threadRoot || msg.seq
  qc.setQueryData<ChatMessageList>(['chatThread', projectId, channelId, rootSeq], (cur) =>
    cur ? upsertIntoList(cur, msg) : cur,
  )
}

// appendPendingMessage adds the optimistic copy (before the POST resolves).
export function appendPendingMessage(
  qc: QueryClient,
  projectId: string,
  channelId: string,
  msg: ChatMessage,
) {
  upsertMessage(qc, projectId, channelId, msg)
}

// removePendingMessage drops an optimistic copy after a failed send.
export function removePendingMessage(
  qc: QueryClient,
  projectId: string,
  channelId: string,
  clientMsgId: string,
) {
  const strip = (cur: ChatMessageList | undefined) =>
    cur
      ? { ...cur, messages: cur.messages.filter((m) => m.clientMsgId !== clientMsgId) }
      : cur
  qc.setQueryData<ChatMessageList>(['chatMessages', projectId, channelId], strip)
  for (const [key, data] of qc.getQueriesData<ChatMessageList>({
    queryKey: ['chatThread', projectId, channelId],
  })) {
    if (data) qc.setQueryData(key, strip(data))
  }
}

function upsertIntoList(cur: ChatMessageList, msg: ChatMessage): ChatMessageList {
  const ix = cur.messages.findIndex(
    (m) =>
      (msg.seq > 0 && m.seq === msg.seq) ||
      (msg.clientMsgId !== '' && m.clientMsgId === msg.clientMsgId),
  )
  let messages: ChatMessage[]
  if (ix >= 0) {
    // Replacing a pending copy keeps its position; a real re-delivery
    // (replayed SSE frame after reconnect) is an idempotent overwrite.
    messages = cur.messages.slice()
    messages[ix] = { ...msg, replyCount: Math.max(msg.replyCount, cur.messages[ix].replyCount) }
  } else {
    messages = [...cur.messages, msg]
  }
  return {
    ...cur,
    messages,
    latestSeq: Math.max(cur.latestSeq, msg.seq),
  }
}

// bumpRootCounters adjusts a thread root's replyCount in the timeline when
// a reply is created (+1) or deleted (-1). The heuristic guard skips the
// bump when the timeline was (re)fetched after this reply landed — its
// count already includes it (lastReplyAt >= the reply's createdAt).
function bumpRootCounters(
  qc: QueryClient,
  projectId: string,
  channelId: string,
  reply: ChatMessage,
  delta: 1 | -1,
) {
  qc.setQueryData<ChatMessageList>(['chatMessages', projectId, channelId], (cur) => {
    if (!cur) return cur
    return {
      ...cur,
      messages: cur.messages.map((m) => {
        if (m.seq !== reply.threadRoot) return m
        if (delta > 0 && m.lastReplyAt && m.lastReplyAt >= reply.createdAt) return m
        return {
          ...m,
          replyCount: Math.max(0, m.replyCount + delta),
          lastReplyAt:
            delta > 0 && (!m.lastReplyAt || reply.createdAt > m.lastReplyAt)
              ? reply.createdAt
              : m.lastReplyAt,
        }
      }),
    }
  })
}

// applyUnread replaces one channel's unread summary with a server-computed
// one (mark-read response, or the read.updated echo from another tab).
export function applyUnread(qc: QueryClient, projectId: string, u: ChatUnread) {
  qc.setQueryData<ChatUnreadList>(['chatUnreads', projectId], (cur) => {
    if (!cur) return cur
    const ix = cur.unreads.findIndex((e) => e.channelId === u.channelId)
    if (ix < 0) return { unreads: [...cur.unreads, u] }
    // Cursors only move forward; an out-of-order echo must not rewind.
    if (cur.unreads[ix].lastReadSeq > u.lastReadSeq) return cur
    const unreads = cur.unreads.slice()
    unreads[ix] = u
    return { unreads }
  })
}

// bumpUnread counts one new message (channel.activity) against the local
// unread summary. The viewing tab immediately marks itself read again; a
// hidden channel's badge grows. Exactness self-heals on the next refetch.
function bumpUnread(qc: QueryClient, projectId: string, channelId: string, lastMessageSeq: number) {
  qc.setQueryData<ChatUnreadList>(['chatUnreads', projectId], (cur) => {
    if (!cur) return cur
    const ix = cur.unreads.findIndex((e) => e.channelId === channelId)
    if (ix < 0) {
      // Unknown channel (created since the last fetch) — resync instead.
      void qc.invalidateQueries({ queryKey: ['chatUnreads', projectId] })
      return cur
    }
    const e = cur.unreads[ix]
    // Guard against re-deliveries and refetch races: only a genuinely new
    // seq past the cursor counts.
    if (lastMessageSeq <= e.latestSeq || lastMessageSeq <= e.lastReadSeq) return cur
    const unreads = cur.unreads.slice()
    unreads[ix] = { ...e, latestSeq: lastMessageSeq, unreadCount: e.unreadCount + 1 }
    return { unreads }
  })
}

function upsertChannel(qc: QueryClient, projectId: string, channel: ChatChannel) {
  qc.setQueryData<ChatChannelList>(['chatChannels', projectId], (cur) => {
    if (!cur) return cur
    const ix = cur.channels.findIndex((c) => c.id === channel.id)
    const channels =
      ix >= 0
        ? cur.channels.map((c) => (c.id === channel.id ? channel : c))
        : [...cur.channels, channel]
    return { ...cur, channels }
  })
}
