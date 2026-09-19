import { useEffect, useRef, useState } from 'react'
import type { Notification } from './types'

/** Lines worth showing: what players did, and who won. Phase announcements are
 *  dropped — the board already shows the flop arriving. */
const SHOWN = new Set(['action', 'result'])

/** How long a line stays up after it arrives. */
const LIFETIME_MS = 9000

/** Most lines on screen at once, oldest dropped first. */
const MAX_VISIBLE = 5

export interface FeedLine {
  seq: number
  kind: string
  text: string
}

/**
 * Turns the rolling notification window in table state into a feed of what has
 * just happened.
 *
 * The table re-reads whole state on every broadcast, so "what is new" has to be
 * derived rather than observed: lines are tracked by the sequence number the
 * server stamps on them, which is the only thing that distinguishes two
 * identical texts ("dev2 checked." twice in an orbit).
 */
export function useActionFeed(notifications: Notification[] | null | undefined): FeedLine[] {
  const [lines, setLines] = useState<FeedLine[]>([])
  const lastSeen = useRef(0)
  const timers = useRef<ReturnType<typeof setTimeout>[]>([])

  useEffect(() => {
    if (!notifications?.length) return

    const newest = notifications[notifications.length - 1].seq

    // A restart, or a different table, resets the server's counter. Start over
    // from the current window rather than replaying it as if it were new.
    if (newest < lastSeen.current) {
      lastSeen.current = newest
      setLines([])
      return
    }

    const fresh = notifications.filter((n) => n.seq > lastSeen.current && SHOWN.has(n.kind))
    lastSeen.current = newest
    if (fresh.length === 0) return

    setLines((current) => [...current, ...fresh].slice(-MAX_VISIBLE))

    for (const line of fresh) {
      const timer = setTimeout(
        () => setLines((current) => current.filter((l) => l.seq !== line.seq)),
        LIFETIME_MS,
      )
      timers.current.push(timer)
    }
  }, [notifications])

  useEffect(() => {
    const pending = timers.current
    return () => pending.forEach(clearTimeout)
  }, [])

  return lines
}
