// Chat DTO shapes (docs/chat/PLAN.md, phase 1). Placeholder module: the
// Chat tab (ChatApp, ChannelSidebar, MessageList, MessageComposer) mirrors
// web/src/wiki/* from the wiki branch and lands with phase 1.

export interface ChatChannel {
  id: string
  name: string
  topic: string
  isRoot: boolean
  isPrivate: boolean
  createdBy: string
  createdAt: string
  archivedAt?: string
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
  deletedAt?: string
  replyCount: number
  lastReplyAt?: string
  reactions: ChatReaction[]
}
