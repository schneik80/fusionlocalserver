import { FontAwesomeIcon } from '@fortawesome/react-fontawesome'
import type { IconDefinition } from '@fortawesome/fontawesome-svg-core'
import {
  faBuilding,
  faChartLine,
  faFolderTree,
  faGear,
  faStar,
} from '@fortawesome/free-solid-svg-icons'
import { Divider, IconButton, Paper, Stack, Tooltip } from '@mui/material'

export type MainView = 'browser' | 'dashboard'

interface NavRailProps {
  view: MainView
  onSelectView: (v: MainView) => void
  onOpenHubs: () => void
  onOpenPins: () => void
  onOpenSettings: () => void
}

export function NavRail({
  view,
  onSelectView,
  onOpenHubs,
  onOpenPins,
  onOpenSettings,
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
      }}
    >
      <Stack spacing={1.5}>
        <RailButton icon={faBuilding} label="Hubs" onClick={onOpenHubs} />
        <Divider flexItem sx={{ mx: 1 }} />
        <RailButton
          icon={faFolderTree}
          label="Browser"
          active={view === 'browser'}
          onClick={() => onSelectView('browser')}
        />
        <RailButton
          icon={faChartLine}
          label="Activity"
          active={view === 'dashboard'}
          onClick={() => onSelectView('dashboard')}
        />
        <Divider flexItem sx={{ mx: 1 }} />
        <RailButton icon={faStar} label="Pins" onClick={onOpenPins} />
        <RailButton icon={faGear} label="Settings" onClick={onOpenSettings} />
      </Stack>
    </Paper>
  )
}

function RailButton({
  icon,
  label,
  active,
  onClick,
}: {
  icon: IconDefinition
  label: string
  active?: boolean
  onClick: () => void
}) {
  return (
    <Tooltip title={label} placement="right">
      <IconButton
        aria-label={label}
        onClick={onClick}
        sx={{
          width: 44,
          height: 44,
          color: active ? 'primary.main' : 'text.secondary',
          bgcolor: active ? 'action.selected' : 'transparent',
        }}
      >
        <FontAwesomeIcon icon={icon} style={{ fontSize: 18 }} />
      </IconButton>
    </Tooltip>
  )
}
