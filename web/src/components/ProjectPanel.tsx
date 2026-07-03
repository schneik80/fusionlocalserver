import { Box, Paper, Tab, Tabs } from '@mui/material'
import { useState } from 'react'
import { ChatApp } from '../chat/ChatApp'
import { useChatEvents } from '../chat/useChatEvents'
import { useNav } from '../state/nav'
import { ProjectDashboard } from './Dashboards'

// ProjectPanel is the project-level pane: a tab shell over the existing
// dashboard and the new chat. It replaces the bare <ProjectDashboard/> that
// used to fill slot B, so the Chat tab exists only at the project level —
// never for a selected folder or document (those levels render
// ContentsColumn / DetailsPanel instead, see BrowserStage). It owns the same
// left-bordered Paper frame the dashboard used, so the slide swap to a
// document's DetailsPanel stays seamless.
//
// This shell deliberately mirrors the wiki branch's ProjectPanel (which
// hosts Dashboard | Wiki) so the two branches merge into one strip with a
// one-<Tab> conflict. Both tabs stay mounted (hidden via display) so
// switching preserves the dashboard's scroll and the chat's draft/scroll
// state; `active` gates chat's polling to the visible tab.

type ProjectTab = 'dashboard' | 'chat'

export function ProjectPanel() {
  const [tab, setTab] = useState<ProjectTab>('dashboard')
  const nav = useNav()
  // The project's single SSE stream lives here — at the project level, not
  // inside the Chat tab — so events keep the chat caches warm from any tab
  // (design doc §5's subscription-scoping table, collapsed to one stream).
  const { live } = useChatEvents(nav.project?.id ?? null)

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
        value={tab}
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
        <Tab label="Chat" value="chat" />
      </Tabs>
      <Box sx={{ flex: 1, minHeight: 0, display: tab === 'dashboard' ? 'flex' : 'none' }}>
        <ProjectDashboard />
      </Box>
      <Box sx={{ flex: 1, minHeight: 0, display: tab === 'chat' ? 'flex' : 'none' }}>
        <ChatApp active={tab === 'chat'} live={live} />
      </Box>
    </Paper>
  )
}
