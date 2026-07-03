import { Box, Paper, Tab, Tabs } from '@mui/material'
import { useState } from 'react'
import { useNav } from '../state/nav'
import { WikiApp } from '../wiki/WikiApp'
import { ProjectDashboard } from './Dashboards'

// ProjectPanel is the project-level pane: a tab shell over the existing
// dashboard and the new wiki. It replaces the bare <ProjectDashboard/> that used
// to fill slot B. The Wiki tab exists only at the project ROOT — drilling into
// any folder hides it (the wiki is a project-level concept, not a per-folder
// one), and selected documents render DetailsPanel instead (see BrowserStage).
// It owns the same left-bordered Paper frame the dashboard used, so the slide
// swap to a document's DetailsPanel stays seamless.
//
// Both tabs stay mounted (hidden via display) so switching tabs preserves the
// dashboard's scroll and the wiki editor's in-progress state.

type ProjectTab = 'dashboard' | 'wiki'

export function ProjectPanel() {
  const nav = useNav()
  const [tab, setTab] = useState<ProjectTab>('dashboard')

  // Inside a folder the Wiki tab is hidden, so the dashboard shows regardless
  // of the chosen tab. The choice itself is kept, not reset — returning to the
  // project root lands back on the Wiki tab if that's where the user was.
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
        {atRoot && <Tab label="Wiki" value="wiki" />}
      </Tabs>
      <Box sx={{ flex: 1, minHeight: 0, display: effectiveTab === 'dashboard' ? 'flex' : 'none' }}>
        <ProjectDashboard />
      </Box>
      <Box sx={{ flex: 1, minHeight: 0, display: effectiveTab === 'wiki' ? 'flex' : 'none' }}>
        <WikiApp active={effectiveTab === 'wiki'} />
      </Box>
    </Paper>
  )
}
