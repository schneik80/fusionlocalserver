import {
  Box,
  CircularProgress,
  List,
  Paper,
  Typography,
} from '@mui/material'
import type { ReactNode } from 'react'

interface ColumnProps {
  title: string
  width?: number | string
  flex?: number
  loading?: boolean
  error?: Error | null
  empty?: boolean
  emptyText?: string
  /** optional control rendered at the right edge of the title bar (e.g. a "+") */
  headerAction?: ReactNode
  children?: ReactNode
}

// Column is the shared shell for the Projects and Contents panes: a titled,
// scrollable surface that renders a centered spinner while loading, the error
// message on failure, an empty-state hint, or its row children.
export function Column({
  title,
  width,
  flex,
  loading,
  error,
  empty,
  emptyText = 'Nothing here',
  headerAction,
  children,
}: ColumnProps) {
  return (
    <Paper
      square
      variant="outlined"
      sx={{
        width,
        flex,
        minWidth: 0,
        display: 'flex',
        flexDirection: 'column',
        borderTop: 0,
        borderBottom: 0,
        borderLeft: 0,
        overflow: 'hidden',
      }}
    >
      <Box
        sx={{
          px: 1.5,
          py: 0.5,
          minHeight: 34,
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'space-between',
          borderBottom: 1,
          borderColor: 'divider',
          flexShrink: 0,
        }}
      >
        <Typography
          variant="subtitle2"
          sx={{
            color: 'text.secondary',
            textTransform: 'uppercase',
            letterSpacing: 0.5,
            fontSize: 11,
          }}
        >
          {title}
        </Typography>
        {headerAction}
      </Box>
      <Box sx={{ flex: 1, overflowY: 'auto', minHeight: 0 }}>
        {loading ? (
          <Centered>
            <CircularProgress size={22} />
          </Centered>
        ) : error ? (
          <Centered>
            <Typography variant="body2" color="error" sx={{ px: 2, textAlign: 'center' }}>
              {error.message}
            </Typography>
          </Centered>
        ) : empty ? (
          <Centered>
            <Typography variant="body2" color="text.secondary">
              {emptyText}
            </Typography>
          </Centered>
        ) : (
          <List dense disablePadding>
            {children}
          </List>
        )}
      </Box>
    </Paper>
  )
}

function Centered({ children }: { children: ReactNode }) {
  return (
    <Box
      sx={{
        height: '100%',
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'center',
        p: 2,
      }}
    >
      {children}
    </Box>
  )
}
