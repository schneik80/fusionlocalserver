import { FontAwesomeIcon } from '@fortawesome/react-fontawesome'
import type { IconDefinition } from '@fortawesome/fontawesome-svg-core'
import {
  faBuilding,
  faGear,
  faMagnifyingGlass,
  faMoon,
  faStar,
  faSun,
} from '@fortawesome/free-solid-svg-icons'
import { Box, IconButton, Paper, Stack, Tooltip } from '@mui/material'

interface NavRailProps {
  onOpenSearch: () => void
  onOpenHubs: () => void
  onOpenPins: () => void
  onOpenSettings: () => void
  /** current theme mode (drives the toggle icon) */
  mode: 'light' | 'dark'
  /** flips between light and dark */
  onToggleTheme: () => void
}

export function NavRail({
  onOpenSearch,
  onOpenHubs,
  onOpenPins,
  onOpenSettings,
  mode,
  onToggleTheme,
}: NavRailProps) {
  return (
    <Paper
      square
      variant="outlined"
      sx={{
        width: 60,
        flexShrink: 0,
        borderTop: 0,
        borderBottom: 0,
        borderLeft: 0,
        display: 'flex',
        flexDirection: 'column',
        alignItems: 'center',
        pt: 1.5,
        pb: 1.5,
      }}
    >
      <Stack spacing={1.5}>
        <RailButton icon={faBuilding} label="Hubs" onClick={onOpenHubs} />
        <RailButton icon={faMagnifyingGlass} label="Search" onClick={onOpenSearch} />
        <RailButton icon={faStar} label="Pins" onClick={onOpenPins} />
        <RailButton icon={faGear} label="Settings" onClick={onOpenSettings} />
      </Stack>
      {/* Theme toggle anchored at the very bottom of the rail. */}
      <Box sx={{ mt: 'auto' }}>
        <RailButton
          icon={mode === 'dark' ? faSun : faMoon}
          label={mode === 'dark' ? 'Switch to light' : 'Switch to dark'}
          onClick={onToggleTheme}
        />
      </Box>
    </Paper>
  )
}

function RailButton({
  icon,
  label,
  onClick,
}: {
  icon: IconDefinition
  label: string
  onClick: () => void
}) {
  return (
    <Tooltip title={label} placement="right">
      <IconButton
        aria-label={label}
        onClick={onClick}
        sx={{ width: 44, height: 44, color: 'text.secondary' }}
      >
        <FontAwesomeIcon icon={icon} style={{ fontSize: 18 }} />
      </IconButton>
    </Tooltip>
  )
}
