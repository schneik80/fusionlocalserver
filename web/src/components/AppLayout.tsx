import { FontAwesomeIcon } from '@fortawesome/react-fontawesome'
import {
  faCubes,
  faMoon,
  faRightFromBracket,
  faSun,
} from '@fortawesome/free-solid-svg-icons'
import {
  AppBar,
  Box,
  Chip,
  IconButton,
  Toolbar,
  Tooltip,
  Typography,
} from '@mui/material'
import { useQueryClient } from '@tanstack/react-query'
import { useEffect, useState } from 'react'
import { api } from '../api/client'
import { QUERY_CACHE_KEY } from '../queryPersist'
import { useAuthMe, useHubs, useMeta } from '../api/queries'
import { useColorMode } from '../state/colorMode'
import { loadLastHub, useNav } from '../state/nav'
import { BreadcrumbBar } from './BreadcrumbBar'
import { BrowserColumns } from './BrowserColumns'
import { HubSwitcher } from './HubSwitcher'
import { NavRail } from './NavRail'
import { PinsDialog } from './PinsDialog'
import { SearchDialog } from './SearchDialog'
import { SettingsDialog } from './SettingsDialog'

type DialogKind = 'search' | 'hubs' | 'pins' | 'settings' | null

export function AppLayout() {
  const [dialog, setDialog] = useState<DialogKind>(null)
  const nav = useNav()
  const metaQ = useMeta()
  const authQ = useAuthMe()
  const hubsQ = useHubs()
  const qc = useQueryClient()
  const { mode, toggle } = useColorMode()

  // Restore the last-used hub once the hub list loads, but only if it's still
  // one of the user's hubs (so a since-revoked hub or a different user on this
  // browser falls back to picking manually).
  useEffect(() => {
    if (nav.hubId || !hubsQ.data) return
    const saved = loadLastHub()
    if (!saved) return
    const hub = hubsQ.data.find((h) => h.id === saved.id)
    if (hub) nav.selectHub(hub.id, hub.name)
  }, [nav.hubId, hubsQ.data]) // eslint-disable-line react-hooks/exhaustive-deps

  // logout drops the server session and clears the persisted query cache (so a
  // different user on this browser doesn't briefly see the prior user's data),
  // then reloads at the root so the gate shows the login screen.
  const logout = async () => {
    try {
      await api.logout()
    } finally {
      qc.clear()
      try {
        localStorage.removeItem(QUERY_CACHE_KEY)
      } catch {
        /* storage unavailable — nothing to clear */
      }
      window.location.assign('/')
    }
  }

  return (
    <Box sx={{ display: 'flex', flexDirection: 'column', height: '100vh' }}>
      <AppBar position="static">
        <Toolbar variant="dense" sx={{ gap: 1.5 }}>
          <FontAwesomeIcon icon={faCubes} style={{ fontSize: 18, color: '#0696d7' }} />
          <Typography variant="h6" sx={{ fontWeight: 600 }}>
            fusionlocalserver
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
          {authQ.data?.user && (authQ.data.user.name || authQ.data.user.email) && (
            <Typography variant="caption" color="text.secondary">
              {authQ.data.user.name || authQ.data.user.email}
            </Typography>
          )}
          <Tooltip title="Sign out">
            <IconButton aria-label="Sign out" onClick={logout} sx={{ color: 'text.secondary' }}>
              <FontAwesomeIcon icon={faRightFromBracket} style={{ fontSize: 16 }} />
            </IconButton>
          </Tooltip>
        </Toolbar>
      </AppBar>

      <Box sx={{ display: 'flex', flex: 1, minHeight: 0 }}>
        <NavRail
          onOpenSearch={() => setDialog('search')}
          onOpenHubs={() => setDialog('hubs')}
          onOpenPins={() => setDialog('pins')}
          onOpenSettings={() => setDialog('settings')}
        />
        <Box sx={{ display: 'flex', flexDirection: 'column', flex: 1, minWidth: 0 }}>
          <BreadcrumbBar onOpenHubs={() => setDialog('hubs')} />
          <BrowserColumns />
        </Box>
      </Box>

      <SearchDialog
        open={dialog === 'search'}
        onClose={() => setDialog(null)}
        hubId={nav.hubId}
        hubName={nav.hubName}
      />
      <HubSwitcher open={dialog === 'hubs'} onClose={() => setDialog(null)} />
      <PinsDialog open={dialog === 'pins'} onClose={() => setDialog(null)} />
      <SettingsDialog open={dialog === 'settings'} onClose={() => setDialog(null)} />
    </Box>
  )
}
