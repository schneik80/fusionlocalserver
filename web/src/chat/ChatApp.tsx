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
// composer, and an optional thread panel. Transport is the project's SSE
// stream (opened by ProjectPanel; `live` reports its health) writing into
// the react-query caches; the 2s polling from phase 1 survives only as the
// fallback while the stream is down. `active` gates fetching to the
// visible tab.
export function ChatApp({ active, live }: { active: boolean; live: boolean }) {
  const nav = useNav()
  const projectId = nav.project?.id ?? null
  const meId = useAuthMe().data?.user?.id ?? ''

  const channelsQ = useChatChannels(projectId, active, live)
  const channels = channelsQ.data?.channels ?? []
  const caps = channelsQ.data?.capabilities ?? NO_CAPS

  const [channelId, setChannelId] = useState<string | null>(null)
  const [threadRoot, setThreadRoot] = useState<number | null>(null)
  // seen[channelId] = highest seq the user has had on screen; channels
  // whose lastActivitySeq (patched in by channel.activity events) is
  // beyond it get bolded in the sidebar. Local-only until phase 4's read
  // cursors.
  const [seen, setSeen] = useState<Record<string, number>>({})
  const current =
    channels.find((c) => c.id === channelId) ?? channels.find((c) => c.isRoot) ?? channels[0] ?? null
  const archived = !!current?.archivedAt

  // Selection resets when the project changes; the thread panel also closes
  // when switching channels.
  useEffect(() => {
    setChannelId(null)
    setThreadRoot(null)
    setSeen({})
  }, [projectId])
  useEffect(() => {
    setThreadRoot(null)
  }, [current?.id])

  const messagesQ = useChatMessages(projectId, current?.id ?? null, active, live)
  const { send, sending, remove, react } = useChatMutations(projectId, current?.id ?? null)

  // Viewing a channel marks everything in it as seen.
  const latestSeq = messagesQ.data?.latestSeq ?? 0
  const currentId = current?.id
  useEffect(() => {
    if (!active || !currentId || latestSeq <= 0) return
    setSeen((prev) =>
      (prev[currentId] ?? 0) >= latestSeq ? prev : { ...prev, [currentId]: latestSeq },
    )
  }, [active, currentId, latestSeq])

  const unseen = new Set(
    channels
      .filter((c) => c.id !== currentId && (c.lastActivitySeq ?? 0) > (seen[c.id] ?? 0))
      .map((c) => c.id),
  )

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
        unseen={unseen}
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
            <Box sx={{ flex: 1 }} />
            {!live && (
              <Typography variant="caption" color="text.disabled">
                reconnecting…
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
          sending={sending}
          onSend={(body) => send(body)}
        />
      </Box>
      {threadRoot !== null && current && (
        <ThreadPanel
          projectId={projectId}
          channelId={current.id}
          rootSeq={threadRoot}
          active={active}
          live={live}
          meId={meId}
          caps={caps}
          archived={archived}
          onClose={() => setThreadRoot(null)}
          onSend={(body, root) => send(body, root)}
          onDelete={doDelete}
          onToggleReaction={doToggleReaction}
          sending={sending}
        />
      )}
    </Box>
  )
}
