import { Component, type ReactNode } from 'react'
import { Box, Typography } from '@mui/material'

// ErrorBoundary stops a crash in its subtree (e.g. the WebGL 3D viewer) from
// blanking the entire app. It renders the error message so failures are
// diagnosable instead of producing a white screen. Note: React error boundaries
// only catch errors thrown during render/lifecycle/effects — not in async
// callbacks (those still surface in the console).
export class ErrorBoundary extends Component<
  { children: ReactNode; label?: string },
  { error: Error | null }
> {
  state: { error: Error | null } = { error: null }

  static getDerivedStateFromError(error: Error) {
    return { error }
  }

  componentDidCatch(error: Error) {
    // eslint-disable-next-line no-console
    console.error(`${this.props.label ?? 'Component'} crashed:`, error)
  }

  render() {
    if (this.state.error) {
      return (
        <Box sx={{ p: 2 }}>
          <Typography variant="body2" color="error" gutterBottom>
            {this.props.label ?? 'This view'} failed to render.
          </Typography>
          <Typography variant="caption" color="text.secondary" sx={{ whiteSpace: 'pre-wrap', wordBreak: 'break-word' }}>
            {this.state.error.message}
          </Typography>
        </Box>
      )
    }
    return this.props.children
  }
}
