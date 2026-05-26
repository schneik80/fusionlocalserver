import CssBaseline from '@mui/material/CssBaseline'
import { ThemeProvider } from '@mui/material/styles'
import { useMemo } from 'react'
import { AppLayout } from './components/AppLayout'
import { useColorMode } from './state/colorMode'
import { NavProvider } from './state/nav'
import { makeTheme } from './theme'

export default function App() {
  const { mode } = useColorMode()
  const theme = useMemo(() => makeTheme(mode), [mode])

  return (
    <ThemeProvider theme={theme}>
      <CssBaseline />
      <NavProvider>
        <AppLayout />
      </NavProvider>
    </ThemeProvider>
  )
}
