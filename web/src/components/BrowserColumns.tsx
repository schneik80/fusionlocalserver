import { Box } from '@mui/material'
import { ContentsColumn } from './ContentsColumn'
import { DetailsPanel } from './DetailsPanel'
import { ProjectsColumn } from './ProjectsColumn'

// The three-pane browser: fixed-width Projects and Contents nav columns on the
// left, the Details panel filling all remaining width out to the right edge.
export function BrowserColumns() {
  return (
    <Box sx={{ display: 'flex', flex: 1, minHeight: 0 }}>
      <ProjectsColumn />
      <ContentsColumn />
      <DetailsPanel />
    </Box>
  )
}
