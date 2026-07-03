import { faHashtag, faLock, faPlus } from '@fortawesome/free-solid-svg-icons'
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome'
import {
  Box,
  Button,
  Checkbox,
  Dialog,
  DialogActions,
  DialogContent,
  DialogTitle,
  FormControlLabel,
  IconButton,
  List,
  ListItemButton,
  ListItemIcon,
  ListItemText,
  TextField,
  Tooltip,
  Typography,
} from '@mui/material'
import { useState } from 'react'
import { useCreateChatChannel } from '../api/queries'
import type { ChatCaps, ChatChannel } from './types'

// ChannelSidebar lists the channels the server says this user can see and
// hosts the create-channel dialog (member management for private channels
// arrives in phase 5; until then a private channel starts owner-only).
export function ChannelSidebar({
  projectId,
  channels,
  currentId,
  caps,
  onSelect,
}: {
  projectId: string | null
  channels: ChatChannel[]
  currentId: string | null
  caps: ChatCaps
  onSelect: (id: string) => void
}) {
  const [createOpen, setCreateOpen] = useState(false)

  return (
    <Box
      sx={{
        width: 200,
        flexShrink: 0,
        borderRight: 1,
        borderColor: 'divider',
        display: 'flex',
        flexDirection: 'column',
        minHeight: 0,
      }}
    >
      <Box sx={{ display: 'flex', alignItems: 'center', px: 1.5, py: 0.75 }}>
        <Typography variant="overline" sx={{ flex: 1, lineHeight: 2 }}>
          Channels
        </Typography>
        {caps.createChannel && (
          <Tooltip title="New channel">
            <IconButton size="small" onClick={() => setCreateOpen(true)} aria-label="new channel">
              <FontAwesomeIcon icon={faPlus} size="xs" />
            </IconButton>
          </Tooltip>
        )}
      </Box>
      <List dense disablePadding sx={{ flex: 1, overflowY: 'auto' }}>
        {channels.map((ch) => (
          <ListItemButton
            key={ch.id}
            selected={ch.id === currentId}
            onClick={() => onSelect(ch.id)}
            sx={{ py: 0.25 }}
          >
            <ListItemIcon sx={{ minWidth: 26, fontSize: 12 }}>
              <FontAwesomeIcon icon={ch.isPrivate ? faLock : faHashtag} />
            </ListItemIcon>
            <ListItemText
              primary={ch.name}
              secondary={ch.archivedAt ? 'archived' : undefined}
              primaryTypographyProps={{ noWrap: true, fontSize: 14 }}
            />
          </ListItemButton>
        ))}
      </List>
      <CreateChannelDialog
        projectId={projectId}
        open={createOpen}
        onClose={() => setCreateOpen(false)}
      />
    </Box>
  )
}

function CreateChannelDialog({
  projectId,
  open,
  onClose,
}: {
  projectId: string | null
  open: boolean
  onClose: () => void
}) {
  const create = useCreateChatChannel(projectId)
  const [name, setName] = useState('')
  const [topic, setTopic] = useState('')
  const [isPrivate, setIsPrivate] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const close = () => {
    setName('')
    setTopic('')
    setIsPrivate(false)
    setError(null)
    onClose()
  }

  const submit = async () => {
    setError(null)
    try {
      await create.mutateAsync({ name: name.trim(), topic: topic.trim() || undefined, isPrivate })
      close()
    } catch (e) {
      setError(e instanceof Error ? e.message : 'could not create channel')
    }
  }

  return (
    <Dialog open={open} onClose={close} maxWidth="xs" fullWidth>
      <DialogTitle>New channel</DialogTitle>
      <DialogContent sx={{ display: 'flex', flexDirection: 'column', gap: 2, pt: '8px !important' }}>
        <TextField
          autoFocus
          label="Name"
          size="small"
          value={name}
          onChange={(e) => setName(e.target.value)}
          inputProps={{ maxLength: 80 }}
        />
        <TextField
          label="Topic (optional)"
          size="small"
          value={topic}
          onChange={(e) => setTopic(e.target.value)}
          inputProps={{ maxLength: 500 }}
        />
        <FormControlLabel
          control={
            <Checkbox checked={isPrivate} onChange={(e) => setIsPrivate(e.target.checked)} />
          }
          label="Private (only invited members can see it)"
        />
        {error && (
          <Typography variant="caption" color="error">
            {error}
          </Typography>
        )}
      </DialogContent>
      <DialogActions>
        <Button onClick={close}>Cancel</Button>
        <Button
          variant="contained"
          disabled={!name.trim() || create.isPending}
          onClick={() => void submit()}
        >
          Create
        </Button>
      </DialogActions>
    </Dialog>
  )
}
