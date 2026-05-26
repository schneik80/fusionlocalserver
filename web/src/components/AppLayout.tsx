import { FontAwesomeIcon } from '@fortawesome/react-fontawesome'
import { faCubes, faMoon, faSun } from '@fortawesome/free-solid-svg-icons'
import {
  AppBar,
  Box,
  Chip,
  IconButton,
  Toolbar,
  Tooltip,
  Typography,
} from '@mui/material'
import { useState } from 'react'
import { useMeta } from '../api/queries'
import { useColorMode } from '../state/colorMode'
import { useNav } from '../state/nav'
import { BreadcrumbBar } from './BreadcrumbBar'
import { BrowserColumns } from './BrowserColumns'
import { HubSwitcher } from './HubSwitcher'
import { NavRail } from './NavRail'
import { PinsDialog } from './PinsDialog'
import { SettingsDialog } from './SettingsDialog'

type DialogKind = 'hubs' | 'pins' | 'settings' | null

export function AppLayout() {
  const [dialog, setDialog] = useState<DialogKind>(null)
  const nav = useNav()
  const metaQ = useMeta()
  const { mode, toggle } = useColorMode()

  return (
    <Box sx={{ display: 'flex', flexDirection: 'column', height: '100vh' }}>
      <AppBar position="static">
        <Toolbar variant="dense" sx={{ gap: 1.5 }}>
          <FontAwesomeIcon icon={faCubes} style={{ fontSize: 18, color: '#0696d7' }} />
          <Typography variant="h6" sx={{ fontWeight: 600 }}>
            FusionDataCLI
          </Typography>
          {metaQ.data && (
            <Typography variant="caption" color="text.secondary">
              {metaQ.data.version}
            </Typography>
          )}
          <Box sx={{ flex: 1 }} />
          {nav.hubName && (
            <Chip
              size="small"
              icon={<FontAwesomeIcon icon={faCubes} style={{ fontSize: 11 }} />}
              label={nav.hubName}
              onClick={() => setDialog('hubs')}
              variant="outlined"
            />
          )}
          <Tooltip title={mode === 'dark' ? 'Switch to light' : 'Switch to dark'}>
            <IconButton aria-label="Toggle theme" onClick={toggle} sx={{ color: 'text.secondary' }}>
              <FontAwesomeIcon icon={mode === 'dark' ? faSun : faMoon} style={{ fontSize: 16 }} />
            </IconButton>
          </Tooltip>
        </Toolbar>
      </AppBar>

      <Box sx={{ display: 'flex', flex: 1, minHeight: 0 }}>
        <NavRail
          onOpenHubs={() => setDialog('hubs')}
          onOpenPins={() => setDialog('pins')}
          onOpenSettings={() => setDialog('settings')}
        />
        <Box sx={{ display: 'flex', flexDirection: 'column', flex: 1, minWidth: 0 }}>
          <BreadcrumbBar onOpenHubs={() => setDialog('hubs')} />
          <BrowserColumns />
        </Box>
      </Box>

      <HubSwitcher open={dialog === 'hubs'} onClose={() => setDialog(null)} />
      <PinsDialog open={dialog === 'pins'} onClose={() => setDialog(null)} />
      <SettingsDialog open={dialog === 'settings'} onClose={() => setDialog(null)} />
    </Box>
  )
}
