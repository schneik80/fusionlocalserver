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
