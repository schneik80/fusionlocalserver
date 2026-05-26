import {
  Alert,
  Box,
  Button,
  Dialog,
  DialogContent,
  DialogTitle,
  Stack,
  TextField,
  ToggleButton,
  ToggleButtonGroup,
  Typography,
} from '@mui/material'
import { useEffect, useState } from 'react'
import { useMeta, useSetPort } from '../api/queries'
import { useColorMode } from '../state/colorMode'

const MIN_PORT = 1024
const MAX_PORT = 65535

// Reconnect delay after a port change. The server rebinds within ~0.5s of
// acking, so a short pause lets the new listener come up before we navigate.
const RECONNECT_DELAY_MS = 2500

export function SettingsDialog({ open, onClose }: { open: boolean; onClose: () => void }) {
  const { preference, setPreference } = useColorMode()
  const metaQ = useMeta()
  const meta = metaQ.data

  return (
    <Dialog open={open} onClose={onClose} fullWidth maxWidth="xs">
      <DialogTitle>Settings</DialogTitle>
      <DialogContent dividers>
        <Stack spacing={3} sx={{ py: 1 }}>
          <Field label="Theme">
            <ToggleButtonGroup
              size="small"
              exclusive
              value={preference}
              onChange={(_, v) => v && setPreference(v)}
            >
              <ToggleButton value="light">Light</ToggleButton>
              <ToggleButton value="dark">Dark</ToggleButton>
              <ToggleButton value="system">System</ToggleButton>
            </ToggleButtonGroup>
          </Field>

          <Field label="Server port">
            <PortSetting open={open} />
          </Field>

          <Field label="Region">
            <Typography variant="body2">{meta?.region ?? '—'}</Typography>
            <Typography variant="caption" color="text.secondary">
              Set at server startup (read-only).
            </Typography>
          </Field>

          <Field label="About">
            <Typography variant="body2">fusionlocalserver · {meta?.version ?? '—'}</Typography>
            <Typography variant="caption" color="text.secondary">
              Fusion open/insert and STEP download are not yet available in this build.
            </Typography>
          </Field>
        </Stack>
      </DialogContent>
    </Dialog>
  )
}

// PortSetting shows the current listen port and, when the server owns it,
// lets the user change it. Applying persists the port and restarts the
// listener, so we then redirect the browser to the new port.
function PortSetting({ open }: { open: boolean }) {
  const metaQ = useMeta()
  const meta = metaQ.data
  const setPort = useSetPort()

  const [value, setValue] = useState('')
  const [reconnectTo, setReconnectTo] = useState<string | null>(null)

  // Reset the field to the live port each time the dialog opens.
  useEffect(() => {
    if (open && meta?.port) setValue(String(meta.port))
  }, [open, meta?.port])

  if (!meta) {
    return <Typography variant="body2">—</Typography>
  }

  if (!meta.portConfigurable) {
    return (
      <>
        <Typography variant="body2">{meta.port}</Typography>
        <Typography variant="caption" color="text.secondary">
          Fixed at startup (launched with <code>-addr</code> or in dev mode).
        </Typography>
      </>
    )
  }

  if (reconnectTo) {
    return (
      <Alert severity="info" sx={{ py: 0.5 }}>
        Server restarting. Reconnecting to{' '}
        <Box component="a" href={reconnectTo} sx={{ wordBreak: 'break-all' }}>
          {reconnectTo}
        </Box>
        …
      </Alert>
    )
  }

  const parsed = Number(value)
  const valid = Number.isInteger(parsed) && parsed >= MIN_PORT && parsed <= MAX_PORT
  const unchanged = parsed === meta.port
  const canApply = valid && !unchanged && !setPort.isPending

  const apply = () => {
    if (!canApply) return
    setPort.mutate(parsed, {
      onSuccess: (res) => {
        if (!res.restarting) return
        const url = new URL(window.location.href)
        url.port = String(res.port)
        // Navigate to exactly what we display. The origin root is enough — the
        // SPA keeps nav state in memory, so a reload starts fresh regardless.
        const target = url.origin
        setReconnectTo(target)
        window.setTimeout(() => {
          window.location.href = target
        }, RECONNECT_DELAY_MS)
      },
    })
  }

  return (
    <Stack spacing={1}>
      <Stack direction="row" spacing={1} alignItems="flex-start">
        <TextField
          size="small"
          type="number"
          value={value}
          onChange={(e) => setValue(e.target.value)}
          onKeyDown={(e) => e.key === 'Enter' && apply()}
          error={value !== '' && !valid}
          inputProps={{ min: MIN_PORT, max: MAX_PORT }}
          sx={{ width: 120 }}
        />
        <Button size="small" variant="outlined" disabled={!canApply} onClick={apply}>
          {setPort.isPending ? 'Applying…' : 'Apply & restart'}
        </Button>
      </Stack>
      {setPort.error && (
        <Typography variant="caption" color="error">
          {(setPort.error as Error).message}
        </Typography>
      )}
      <Typography variant="caption" color="text.secondary">
        {MIN_PORT}–{MAX_PORT}. Changing the port restarts the server; you'll reconnect
        on the new port.
      </Typography>
    </Stack>
  )
}

function Field({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <Box>
      <Typography variant="subtitle2" gutterBottom>
        {label}
      </Typography>
      <Stack spacing={0.5}>{children}</Stack>
    </Box>
  )
}
