import { useEffect, useRef, useState } from 'react'

// How far outside the viewport a row starts loading. Enough that normal
// scrolling finds its thumbnail already there, small enough that landing on a
// long folder doesn't fetch the whole thing.
const ROOT_MARGIN = '250px'

// useInView reports whether an element has come within ROOT_MARGIN of the
// viewport, and then LATCHES — once true it never goes back to false.
//
// The latch is the point. It gates work that is idempotent and cached
// (a classify query, a thumbnail image), so re-running it when a row scrolls
// away and back would be pure waste; and an unlatched flag would make rows
// flicker between an icon and its thumbnail while scrolling.
//
// Intersection is clipped by ancestor scroll containers, so a plain viewport
// root correctly reports rows scrolled out of an `overflow: auto` column as
// off-screen — no need to thread the scrolling parent through.
export function useInView<T extends Element>(): [(node: T | null) => void, boolean] {
  const [node, setNode] = useState<T | null>(null)
  const [seen, setSeen] = useState(false)
  // Read inside the observer callback so the effect never re-subscribes when
  // `seen` flips — the disconnect below is what stops the work.
  const seenRef = useRef(false)

  useEffect(() => {
    if (!node || seenRef.current) return
    // No IntersectionObserver (jsdom, very old browsers) → show everything
    // rather than a column of permanently empty rows.
    if (typeof IntersectionObserver === 'undefined') {
      seenRef.current = true
      setSeen(true)
      return
    }
    const observer = new IntersectionObserver(
      (entries) => {
        if (entries.some((e) => e.isIntersecting)) {
          seenRef.current = true
          setSeen(true)
          observer.disconnect()
        }
      },
      { rootMargin: ROOT_MARGIN },
    )
    observer.observe(node)
    return () => observer.disconnect()
  }, [node])

  return [setNode, seen]
}
