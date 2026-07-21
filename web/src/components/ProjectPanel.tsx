import { Box, Paper, Stack, Tab, Tabs } from '@mui/material'
import { useState } from 'react'
import { useChatUnreads } from '../api/queries'
import { ChatApp } from '../chat/ChatApp'
import { useChatEvents } from '../chat/useChatEvents'
import { useNav } from '../state/nav'
import { ProductionApp } from '../production/ProductionApp'
import { TasksApp } from '../tasks/TasksApp'
import { WhiteboardsApp } from '../whiteboards/WhiteboardsApp'
import { WikiApp } from '../wiki/WikiApp'
import { ProjectDashboard } from './Dashboards'

// ProjectPanel is the project-level pane: a tab shell over the dashboard, the
// wiki, and chat. It replaces the bare <ProjectDashboard/> that used to fill
// slot B. Wiki and Chat are project-ROOT concepts — drilling into any folder
// hides them (they belong to the project, not a folder), and selected
// documents render DetailsPanel instead (see BrowserStage). It owns the same
// left-bordered Paper frame the dashboard used, so the slide swap to a
// document's DetailsPanel stays seamless.
//
// All tabs stay mounted (hidden via display) so switching preserves the
// dashboard's scroll, the wiki editor's in-progress state, and the chat
// scroll/draft. `active` gates chat's fetching to the visible tab; the chat
// SSE stream (below) lives regardless of the tab, keeping the chat caches
// warm from any tab.

type ProjectTab = 'dashboard' | 'wiki' | 'chat' | 'tasks' | 'production' | 'whiteboards'

export function ProjectPanel() {
  const nav = useNav()
  const [tab, setTab] = useState<ProjectTab>('dashboard')

  // One SSE stream per open project, opened here (not inside the Chat tab) so
  // events keep the channel list and activity badges warm from any tab. `live`
  // demotes chat's polling to a fallback while the stream is healthy.
  const { live } = useChatEvents(nav.project?.id ?? null)

  // Unread total for the Chat tab badge — server read cursors, kept live by
  // the same stream (channel.activity / read.updated events).
  const unreadsQ = useChatUnreads(nav.project?.id ?? null, live)
  const totalUnread = (unreadsQ.data?.unreads ?? []).reduce((n, u) => n + u.unreadCount, 0)

  // Inside a folder the Wiki/Chat tabs are hidden, so the dashboard shows
  // regardless of the chosen tab. The choice itself is kept, not reset —
  // returning to the project root lands back where the user was.
  const atRoot = nav.folderStack.length === 0
  const effectiveTab: ProjectTab = atRoot ? tab : 'dashboard'

  return (
    <Paper
      square
      variant="outlined"
      sx={{
        flex: 1,
        minWidth: 320,
        height: '100%',
        display: 'flex',
        flexDirection: 'column',
        borderTop: 0,
        borderBottom: 0,
        borderRight: 0,
        overflow: 'hidden',
      }}
    >
      <Tabs
        value={effectiveTab}
        onChange={(_, v) => setTab(v as ProjectTab)}
        sx={{
          minHeight: 40,
          px: 1,
          borderBottom: 1,
          borderColor: 'divider',
          '& .MuiTab-root': { minHeight: 40, py: 0, textTransform: 'none' },
        }}
      >
        <Tab label="Dashboard" value="dashboard" />
        {atRoot && <Tab label="Production" value="production" />}
        {atRoot && <Tab label="Tasks" value="tasks" />}
        {atRoot && <Tab label="Whiteboards" value="whiteboards" />}
        {atRoot && <Tab label="Wiki" value="wiki" />}
        {atRoot && (
          <Tab
            value="chat"
            label={
              totalUnread > 0 ? (
                <Stack direction="row" spacing={0.75} alignItems="center">
                  <span>Chat</span>
                  <Box
                    component="span"
                    sx={{
                      px: 0.75,
                      minWidth: 18,
                      borderRadius: 9,
                      bgcolor: 'primary.main',
                      color: 'primary.contrastText',
                      fontSize: 11,
                      lineHeight: '18px',
                      textAlign: 'center',
                    }}
                  >
                    {totalUnread > 99 ? '99+' : totalUnread}
                  </Box>
                </Stack>
              ) : (
                'Chat'
              )
            }
          />
        )}
      </Tabs>
      <Box sx={{ flex: 1, minHeight: 0, display: effectiveTab === 'dashboard' ? 'flex' : 'none' }}>
        <ProjectDashboard />
      </Box>
      <Box sx={{ flex: 1, minHeight: 0, display: effectiveTab === 'production' ? 'flex' : 'none' }}>
        <ProductionApp active={effectiveTab === 'production'} />
      </Box>
      <Box sx={{ flex: 1, minHeight: 0, display: effectiveTab === 'tasks' ? 'flex' : 'none' }}>
        <TasksApp active={effectiveTab === 'tasks'} />
      </Box>
      <Box sx={{ flex: 1, minHeight: 0, display: effectiveTab === 'whiteboards' ? 'flex' : 'none' }}>
        <WhiteboardsApp active={effectiveTab === 'whiteboards'} />
      </Box>
      <Box sx={{ flex: 1, minHeight: 0, display: effectiveTab === 'wiki' ? 'flex' : 'none' }}>
        <WikiApp active={effectiveTab === 'wiki'} />
      </Box>
      <Box sx={{ flex: 1, minHeight: 0, display: effectiveTab === 'chat' ? 'flex' : 'none' }}>
        <ChatApp active={effectiveTab === 'chat'} live={live} />
      </Box>
    </Paper>
  )
}
