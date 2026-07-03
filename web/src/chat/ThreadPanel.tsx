import { faXmark } from '@fortawesome/free-solid-svg-icons'
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome'
import { Box, IconButton, Typography } from '@mui/material'
import { useChatThread } from '../api/queries'
import { MessageComposer } from './MessageComposer'
import { MessageList } from './MessageList'
import type { ChatCaps } from './types'

// ThreadPanel is the right-hand drawer for one thread: the root message,
// its replies, and a composer that posts replies (threadRootSeq set). It
// polls on the same 2s cadence as the channel while open.
export function ThreadPanel({
  projectId,
  channelId,
  rootSeq,
  active,
  live,
  meId,
  caps,
  archived,
  onClose,
  onSend,
  onDelete,
  onToggleReaction,
  sending,
}: {
  projectId: string | null
  channelId: string | null
  rootSeq: number
  active: boolean
  live: boolean
  meId: string
  caps: ChatCaps
  archived: boolean
  onClose: () => void
  onSend: (body: string, threadRootSeq: number) => Promise<unknown>
  onDelete: (seq: number) => void
  onToggleReaction: (seq: number, emoji: string, on: boolean) => void
  sending: boolean
}) {
  const threadQ = useChatThread(projectId, channelId, rootSeq, active, live)
  const messages = threadQ.data?.messages ?? []

  return (
    <Box
      sx={{
        width: 300,
        flexShrink: 0,
        borderLeft: 1,
        borderColor: 'divider',
        display: 'flex',
        flexDirection: 'column',
        minHeight: 0,
      }}
    >
      <Box
        sx={{
          display: 'flex',
          alignItems: 'center',
          px: 1.5,
          py: 0.75,
          borderBottom: 1,
          borderColor: 'divider',
        }}
      >
        <Typography variant="subtitle2" sx={{ flex: 1 }}>
          Thread
        </Typography>
        <IconButton size="small" onClick={onClose} aria-label="close thread">
          <FontAwesomeIcon icon={faXmark} size="xs" />
        </IconButton>
      </Box>
      <MessageList
        messages={messages}
        meId={meId}
        caps={caps}
        emptyText={threadQ.isLoading ? 'Loading…' : 'Thread not found.'}
        onDelete={onDelete}
        onToggleReaction={onToggleReaction}
      />
      <MessageComposer
        placeholder="Reply…"
        disabled={!caps.post || archived}
        disabledReason={
          archived ? 'This channel is archived' : 'Your project role is read-only'
        }
        sending={sending}
        onSend={(body) => onSend(body, rootSeq)}
      />
    </Box>
  )
}
