import {
  Box,
  Button,
  Dialog,
  DialogActions,
  DialogContent,
  DialogTitle,
  TextField,
  Typography,
} from '@mui/material'
import { useEffect, useState } from 'react'

// ImageUrlDialog collects a public image URL (+ optional alt text) for the
// editor's "insert image from URL" toolbar action, with a live preview so the
// user can confirm the link actually resolves to an image before inserting.
export function ImageUrlDialog({
  open,
  onClose,
  onInsert,
}: {
  open: boolean
  onClose: () => void
  onInsert: (url: string, alt: string) => void
}) {
  const [url, setUrl] = useState('')
  const [alt, setAlt] = useState('')
  const [previewFailed, setPreviewFailed] = useState(false)

  useEffect(() => {
    if (!open) return
    setUrl('')
    setAlt('')
    setPreviewFailed(false)
  }, [open])

  const trimmed = url.trim()
  const valid = /^https?:\/\/\S+$/i.test(trimmed)

  function insert() {
    if (!valid) return
    onInsert(trimmed, alt.trim() || 'image')
  }

  return (
    <Dialog open={open} onClose={onClose} maxWidth="sm" fullWidth>
      <DialogTitle>Insert image from URL</DialogTitle>
      <DialogContent sx={{ display: 'flex', flexDirection: 'column', gap: 2 }}>
        <TextField
          autoFocus
          fullWidth
          label="Image URL"
          placeholder="https://example.com/image.png"
          value={url}
          onChange={(e) => {
            setUrl(e.target.value)
            setPreviewFailed(false)
          }}
          onKeyDown={(e) => {
            if (e.key === 'Enter') insert()
          }}
          variant="standard"
          sx={{ mt: 1 }}
          helperText="The URL must be publicly reachable by anyone reading the page."
        />
        <TextField
          fullWidth
          label="Alt text (optional)"
          value={alt}
          onChange={(e) => setAlt(e.target.value)}
          onKeyDown={(e) => {
            if (e.key === 'Enter') insert()
          }}
          variant="standard"
        />
        {valid && (
          <Box sx={{ textAlign: 'center' }}>
            {previewFailed ? (
              <Typography variant="caption" color="text.secondary">
                Couldn't load a preview — check the URL points at an image.
              </Typography>
            ) : (
              <Box
                component="img"
                src={trimmed}
                alt=""
                onError={() => setPreviewFailed(true)}
                sx={{ maxWidth: '100%', maxHeight: 160, borderRadius: 1 }}
              />
            )}
          </Box>
        )}
      </DialogContent>
      <DialogActions>
        <Button onClick={onClose} color="inherit">
          Cancel
        </Button>
        <Button variant="contained" disabled={!valid} onClick={insert}>
          Insert
        </Button>
      </DialogActions>
    </Dialog>
  )
}
