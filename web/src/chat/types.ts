// TypeScript mirrors of the chat DTOs in server/dto_chat.go. Keep field
// names in sync with the json tags there.

export interface ChatChannel {
  id: string
  name: string
  topic: string
  isRoot: boolean
  isPrivate: boolean
  createdBy: string
  createdAt: string
  archivedAt?: string
  memberIds?: string[]
  // lastActivitySeq is CLIENT-ONLY: patched into the cache by
  // channel.activity SSE events, never sent by REST. The sidebar compares
  // it against the last seq the user saw to bold channels with unseen
  // activity (proper read cursors land in phase 4).
  lastActivitySeq?: number
}

export interface ChatReaction {
  userId: string
  emoji: string
  at: string
}

export interface ChatMessage {
  seq: number
  threadRoot?: number
  authorId: string
  authorName: string
  clientMsgId: string
  body: string
  createdAt: string
  editedAt?: string
  deleted: boolean
  replyCount: number
  lastReplyAt?: string
  reactions: ChatReaction[]
  // pending is CLIENT-ONLY: true on the optimistic copy shown between
  // hitting send and the server's echo (REST response or SSE event,
  // whichever lands first — both reconcile on clientMsgId). Pending
  // messages carry a negative placeholder seq.
  pending?: boolean
}

// ChatCaps is what the signed-in user may do in this project's chat,
// derived server-side from their APS project role.
export interface ChatCaps {
  post: boolean
  createChannel: boolean
  moderate: boolean
}

export interface ChatChannelList {
  channels: ChatChannel[]
  capabilities: ChatCaps
}

export interface ChatMessageList {
  messages: ChatMessage[]
  // latestSeq is the channel's newest seq — the polling cursor once SSE
  // recovery (phase 2/3) starts using afterSeq deltas.
  latestSeq: number
}

// ---- SSE event payloads (mirror server/dto_chat.go's ChatXxxEventDTO) ----

// ChatEvent is the {type, v, data} envelope every frame carries
// (design doc §3).
export interface ChatEvent {
  type: string
  v: number
  data: unknown
}

export interface ChatMessageEvent {
  channelId: string
  message: ChatMessage
}

export interface ChatChannelEvent {
  channel: ChatChannel
}

export interface ChatMemberEvent {
  channelId: string
  userId: string
  channel: ChatChannel
}

export interface ChatActivityEvent {
  channelId: string
  lastMessageSeq: number
}
