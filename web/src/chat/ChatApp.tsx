import { faHashtag, faLock } from '@fortawesome/free-solid-svg-icons'
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome'
import { Alert, Box, CircularProgress, Stack, Typography } from '@mui/material'
import { useEffect, useState } from 'react'
import {
  useAuthMe,
  useChatChannels,
  useChatMessages,
  useChatMutations,
} from '../api/queries'
import { useNav } from '../state/nav'
import { ChannelSidebar } from './ChannelSidebar'
import { MessageComposer } from './MessageComposer'
import { MessageList } from './MessageList'
import { ThreadPanel } from './ThreadPanel'
import type { ChatCaps } from './types'

const NO_CAPS: ChatCaps = { post: false, createChannel: false, moderate: false }

// ChatApp is the Chat tab's content: channel sidebar, message timeline,
// composer, and an optional thread panel. Phase 1 transport is polling (2s
// on the open channel, gated to the visible tab via `active`); the SSE
// stream replaces it in phases 2–3 (docs/chat/PLAN.md).
export function ChatApp({ active }: { active: boolean }) {
  const nav = useNav()
  const projectId = nav.project?.id ?? null
  const meId = useAuthMe().data?.user?.id ?? ''

  const channelsQ = useChatChannels(projectId, active)
  const channels = channelsQ.data?.channels ?? []
  const caps = channelsQ.data?.capabilities ?? NO_CAPS

  const [channelId, setChannelId] = useState<string | null>(null)
  const [threadRoot, setThreadRoot] = useState<number | null>(null)
  const current =
    channels.find((c) => c.id === channelId) ?? channels.find((c) => c.isRoot) ?? channels[0] ?? null
  const archived = !!current?.archivedAt

  // Selection resets when the project changes; the thread panel also closes
  // when switching channels.
  useEffect(() => {
    setChannelId(null)
    setThreadRoot(null)
  }, [projectId])
  useEffect(() => {
    setThreadRoot(null)
  }, [current?.id])

  const messagesQ = useChatMessages(projectId, current?.id ?? null, active)
  const { send, remove, react } = useChatMutations(projectId, current?.id ?? null)

  const sendBody = (body: string, threadRootSeq?: number) =>
    send.mutateAsync({ body, threadRootSeq })
  const doDelete = (seq: number) => void remove.mutateAsync(seq).catch(() => {})
  const doToggleReaction = (seq: number, emoji: string, on: boolean) =>
    void react.mutateAsync({ seq, emoji, on }).catch(() => {})

  if (channelsQ.isError) {
    return (
      <Box sx={{ flex: 1, p: 2 }}>
        <Alert severity="warning">
          Chat is unavailable: {channelsQ.error instanceof Error ? channelsQ.error.message : 'error'}
        </Alert>
      </Box>
    )
  }
  if (!channelsQ.data) {
    return (
      <Box sx={{ flex: 1, display: 'flex', alignItems: 'center', justifyContent: 'center' }}>
        <CircularProgress size={28} />
      </Box>
    )
  }

  return (
    <Box sx={{ flex: 1, display: 'flex', minHeight: 0, minWidth: 0 }}>
      <ChannelSidebar
        projectId={projectId}
        channels={channels}
        currentId={current?.id ?? null}
        caps={caps}
        onSelect={setChannelId}
      />
      <Box sx={{ flex: 1, display: 'flex', flexDirection: 'column', minWidth: 0, minHeight: 0 }}>
        {current && (
          <Stack
            direction="row"
            spacing={1}
            alignItems="baseline"
            sx={{ px: 1.5, py: 0.75, borderBottom: 1, borderColor: 'divider' }}
          >
            <Typography variant="subtitle2" sx={{ fontSize: 12 }}>
              <FontAwesomeIcon icon={current.isPrivate ? faLock : faHashtag} />
            </Typography>
            <Typography variant="subtitle2">{current.name}</Typography>
            {current.topic && (
              <Typography variant="caption" color="text.secondary" noWrap>
                {current.topic}
              </Typography>
            )}
            {archived && (
              <Typography variant="caption" color="warning.main">
                archived
              </Typography>
            )}
          </Stack>
        )}
        <MessageList
          messages={(messagesQ.data?.messages ?? []).filter((m) => !m.threadRoot)}
          meId={meId}
          caps={caps}
          emptyText={
            messagesQ.isLoading ? 'Loading…' : 'No messages yet — start the conversation.'
          }
          onOpenThread={setThreadRoot}
          onDelete={doDelete}
          onToggleReaction={doToggleReaction}
        />
        <MessageComposer
          placeholder={current ? `Message #${current.name}` : 'Message'}
          disabled={!caps.post || archived || !current}
          disabledReason={
            archived ? 'This channel is archived' : 'Your project role is read-only'
          }
          sending={send.isPending}
          onSend={(body) => sendBody(body)}
        />
      </Box>
      {threadRoot !== null && current && (
        <ThreadPanel
          projectId={projectId}
          channelId={current.id}
          rootSeq={threadRoot}
          active={active}
          meId={meId}
          caps={caps}
          archived={archived}
          onClose={() => setThreadRoot(null)}
          onSend={(body, root) => sendBody(body, root)}
          onDelete={doDelete}
          onToggleReaction={doToggleReaction}
          sending={send.isPending}
        />
      )}
    </Box>
  )
}
