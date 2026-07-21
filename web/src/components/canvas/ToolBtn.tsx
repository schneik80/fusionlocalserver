import type { IconDefinition } from '@fortawesome/fontawesome-svg-core'
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome'
import { IconButton, Tooltip } from '@mui/material'

// ToolBtn is the overlay button used by the pan/zoom canvases (the relationship
// graph and the production flow canvas): a small paper-backed icon button that
// floats over the graph. It stops mousedown from reaching the canvas, which
// would otherwise start a pan under the cursor.
export function ToolBtn({
  label,
  icon,
  onClick,
}: {
  label: string
  icon: IconDefinition
  onClick: () => void
}) {
  return (
    <Tooltip title={label}>
      <IconButton
        size="small"
        onMouseDown={(e) => e.stopPropagation()}
        onClick={onClick}
        sx={{
          bgcolor: 'background.paper',
          border: 1,
          borderColor: 'divider',
          '&:hover': { bgcolor: 'background.paper' },
        }}
      >
        <FontAwesomeIcon icon={icon} style={{ fontSize: 12 }} />
      </IconButton>
    </Tooltip>
  )
}
