import { useEffect, useState } from 'react'

/**
 * Whole seconds left until an absolute epoch-ms deadline sent by the server.
 * Counting against a timestamp (rather than decrementing a number the server
 * handed us) keeps the display honest across tab throttling and reconnects.
 */
export function useCountdown(endsAtMs: number | undefined, active = true): number {
  const [remaining, setRemaining] = useState(() => secondsUntil(endsAtMs))

  useEffect(() => {
    if (!active || !endsAtMs) {
      setRemaining(0)
      return
    }
    setRemaining(secondsUntil(endsAtMs))
    const id = setInterval(() => {
      const left = secondsUntil(endsAtMs)
      setRemaining(left)
      if (left <= 0) clearInterval(id)
    }, 250)
    return () => clearInterval(id)
  }, [endsAtMs, active])

  return remaining
}

function secondsUntil(endsAtMs?: number): number {
  if (!endsAtMs) return 0
  return Math.max(0, Math.ceil((endsAtMs - Date.now()) / 1000))
}
