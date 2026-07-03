import { Typography } from '@mui/material'

// TypingIndicator is the "N is typing…" caption pinned above the composer.
// It always renders (with a non-breaking space when idle) so the timeline
// doesn't jump when someone starts or stops typing.
export function TypingIndicator({ names }: { names: string[] }) {
  let text = ' '
  if (names.length === 1) text = `${names[0]} is typing…`
  else if (names.length === 2) text = `${names[0]} and ${names[1]} are typing…`
  else if (names.length > 2) text = 'Several people are typing…'

  return (
    <Typography
      variant="caption"
      color="text.secondary"
      sx={{ px: 1.5, display: 'block', lineHeight: '18px', fontStyle: 'italic' }}
    >
      {text}
    </Typography>
  )
}
