import { useEffect } from 'react'

/**
 * Keeps the game view in landscape without ever asking the player to rotate.
 *
 * While the table is mounted, <html> carries `data-force-landscape`, and in a
 * portrait viewport the stylesheet turns the whole app a quarter turn so the
 * felt is laid out and drawn in landscape (see index.css). Nothing is put in
 * front of the table, so a seated player never burns their turn clock behind
 * a prompt.
 *
 * Fullscreen and a real orientation lock still need a user gesture, so the
 * player's own taps on the table are borrowed for that, silently. Where the
 * browser allows it (Android Chrome) the viewport turns landscape for real and
 * the CSS rotation switches itself off; where it does not (iOS Safari) the
 * rotation simply stays.
 */
export function useImmersive() {
  useEffect(() => {
    const root = document.documentElement
    root.dataset.forceLandscape = ''

    // Touch devices only: a desktop window is landscape already, and jumping
    // to fullscreen on the first click there would be a surprise.
    const touch = window.matchMedia('(pointer: coarse)').matches
    const onTap = () => {
      if (!document.fullscreenElement) void enterImmersive()
    }
    if (touch) document.addEventListener('click', onTap, true)

    return () => {
      delete root.dataset.forceLandscape
      document.removeEventListener('click', onTap, true)
    }
  }, [])
}

async function enterImmersive() {
  try {
    await document.documentElement.requestFullscreen({ navigationUI: 'hide' })
  } catch {
    // Refused (iOS, or a permissions policy) — the CSS rotation still applies.
  }
  try {
    const orientation = screen.orientation as ScreenOrientation & {
      lock?: (o: string) => Promise<void>
    }
    await orientation.lock?.('landscape')
  } catch {
    // Unsupported on iOS and on desktop; the CSS rotation covers it.
  }
}

/** Leaves fullscreen and releases any orientation lock, on the way out. */
export async function exitImmersive() {
  try {
    const orientation = screen.orientation as ScreenOrientation & { unlock?: () => void }
    orientation.unlock?.()
  } catch {
    /* not supported — nothing was locked */
  }
  try {
    if (document.fullscreenElement) await document.exitFullscreen()
  } catch {
    /* already out */
  }
}
