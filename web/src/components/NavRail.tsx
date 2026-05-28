import { FontAwesomeIcon } from '@fortawesome/react-fontawesome'
import type { IconDefinition } from '@fortawesome/fontawesome-svg-core'
import {
  faBuilding,
  faGear,
  faMagnifyingGlass,
  faStar,
} from '@fortawesome/free-solid-svg-icons'
import { IconButton, Paper, Stack, Tooltip } from '@mui/material'

interface NavRailProps {
  onOpenSearch: () => void
  onOpenHubs: () => void
  onOpenPins: () => void
  onOpenSettings: () => void
}

export function NavRail({
  onOpenSearch,
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
        <RailButton icon={faMagnifyingGlass} label="Search" onClick={onOpenSearch} />
        <RailButton icon={faStar} label="Pins" onClick={onOpenPins} />
        <RailButton icon={faGear} label="Settings" onClick={onOpenSettings} />
      </Stack>
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
