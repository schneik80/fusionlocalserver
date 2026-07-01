import { Box, Paper, Tab, Tabs } from '@mui/material'
import { useEffect, useState } from 'react'
import { useNav } from '../state/nav'
import { WikiApp } from '../wiki/WikiApp'
import { ProjectDashboard } from './Dashboards'

// ProjectPanel is the project-level pane: a tab shell over the existing
// dashboard and the new wiki. It replaces the bare <ProjectDashboard/> that used
// to fill slot B, so the Wiki tab exists only at the project level — never for a
// selected folder or document (those levels render ContentsColumn / DetailsPanel
// instead, see BrowserStage). It owns the same left-bordered Paper frame the
// dashboard used, so the slide swap to a document's DetailsPanel stays seamless.
//
// Both tabs stay mounted (hidden via display) so switching tabs preserves the
// dashboard's scroll and the wiki editor's in-progress state.

type ProjectTab = 'dashboard' | 'wiki'

export function ProjectPanel() {
  const nav = useNav()
  const [tab, setTab] = useState<ProjectTab>('dashboard')

  // Drilling into the project's "Wiki" folder surfaces the wiki directly, rather
  // than leaving the user on the dashboard looking at raw .md files.
  const topFolder = nav.folderStack[nav.folderStack.length - 1]
  const inWikiFolder = topFolder?.name?.toLowerCase() === 'wiki'
  useEffect(() => {
    if (inWikiFolder) setTab('wiki')
  }, [inWikiFolder])

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
        <Tab label="Wiki" value="wiki" />
      </Tabs>
      <Box sx={{ flex: 1, minHeight: 0, display: tab === 'dashboard' ? 'flex' : 'none' }}>
        <ProjectDashboard />
      </Box>
      <Box sx={{ flex: 1, minHeight: 0, display: tab === 'wiki' ? 'flex' : 'none' }}>
        <WikiApp active={tab === 'wiki'} />
      </Box>
    </Paper>
  )
}
