import { useQueryClient } from '@tanstack/react-query'
import { useEffect, useState } from 'react'
import { applyChatEvent } from './cache'
import type { ChatEvent } from './types'

// useChatEvents owns the project's single EventSource (mounted in
// ProjectPanel, so the stream lives while the project is open on ANY tab —
// that's what keeps the channel list and activity bolding warm). Events
// apply straight into the react-query caches; the returned `live` flag
// lets the query hooks demote their 2s polling to a fallback while the
// stream is healthy.
//
// Recovery is layered (docs/chat/PLAN.md phases 2–3):
//  - EventSource reconnects by itself, sending Last-Event-ID; the server
//    replays what its ring still holds.
//  - When it can't (restart → new epoch, or the ring aged out), it sends a
//    named "reset" frame and we invalidate every chat query for the
//    project — full REST resync, then live again.
//  - While the stream is down (`live` false), the hooks' refetchInterval
//    re-arms, so nothing is missed even if the server stays away a while.
//
// Note for the Vite-direct dev workflow (:5173): EventSource sends cookies
// only same-origin, so use the Vite proxy (default) or `-dev` Go proxying;
// see PLAN.md open question 5.
export function useChatEvents(projectId: string | null): { live: boolean } {
  const qc = useQueryClient()
  const [live, setLive] = useState(false)

  useEffect(() => {
    if (!projectId) return
    const es = new EventSource(`/api/chat/events?projectId=${encodeURIComponent(projectId)}`)

    es.onopen = () => setLive(true)
    // EventSource retries on its own; `live=false` re-arms the polling
    // fallback in the meantime.
    es.onerror = () => setLive(false)
    es.onmessage = (e) => {
      try {
        applyChatEvent(qc, projectId, JSON.parse(e.data) as ChatEvent)
      } catch {
        /* one malformed frame must not kill the stream */
      }
    }
    es.addEventListener('reset', () => {
      void qc.invalidateQueries({ queryKey: ['chatChannels', projectId] })
      void qc.invalidateQueries({ queryKey: ['chatMessages', projectId] })
      void qc.invalidateQueries({ queryKey: ['chatThread', projectId] })
    })

    return () => {
      es.close()
      setLive(false)
    }
  }, [projectId, qc])

  return { live }
}
