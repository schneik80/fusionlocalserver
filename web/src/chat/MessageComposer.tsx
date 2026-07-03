import { faPaperPlane } from '@fortawesome/free-solid-svg-icons'
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome'
import { Box, IconButton, TextField, Tooltip, Typography } from '@mui/material'
import { useState } from 'react'

// MessageComposer is the send box for a channel or thread. Enter sends,
// Shift+Enter inserts a newline. Read-only roles see a disabled box with an
// explanation instead of bouncing off the server's 403.
export function MessageComposer({
  placeholder,
  disabled,
  disabledReason,
  sending,
  onSend,
}: {
  placeholder: string
  disabled: boolean
  disabledReason?: string
  sending: boolean
  onSend: (body: string) => Promise<unknown>
}) {
  const [text, setText] = useState('')
  const [error, setError] = useState<string | null>(null)

  const send = async () => {
    const body = text.trim()
    if (!body || sending) return
    setError(null)
    try {
      await onSend(body)
      setText('')
    } catch (e) {
      setError(e instanceof Error ? e.message : 'send failed')
    }
  }

  const box = (
    <Box sx={{ display: 'flex', alignItems: 'flex-end', gap: 0.5, p: 1, pt: 0.5 }}>
      <TextField
        fullWidth
        size="small"
        multiline
        maxRows={6}
        placeholder={placeholder}
        value={text}
        disabled={disabled}
        onChange={(e) => setText(e.target.value)}
        onKeyDown={(e) => {
          if (e.key === 'Enter' && !e.shiftKey) {
            e.preventDefault()
            void send()
          }
        }}
      />
      <IconButton
        size="small"
        color="primary"
        disabled={disabled || sending || !text.trim()}
        onClick={() => void send()}
        aria-label="send message"
      >
        <FontAwesomeIcon icon={faPaperPlane} />
      </IconButton>
    </Box>
  )

  return (
    <Box sx={{ borderTop: 1, borderColor: 'divider' }}>
      {error && (
        <Typography variant="caption" color="error" sx={{ px: 1.5, pt: 0.5, display: 'block' }}>
          {error}
        </Typography>
      )}
      {disabled && disabledReason ? (
        <Tooltip title={disabledReason}>
          <span>{box}</span>
        </Tooltip>
      ) : (
        box
      )}
    </Box>
  )
}
