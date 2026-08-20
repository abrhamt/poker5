import { useCallback, useEffect, useState } from 'react'

interface ImmersiveState {
  isPortrait: boolean
  isFullscreen: boolean
  /** True until the player has taken the gesture that unlocks fullscreen. */
  needsPrompt: boolean
  enter: () => Promise<void>
}

/**
 * Drives the landscape game view.
 *
 * The web cannot force an orientation on its own: `screen.orientation.lock`
 * only works from inside fullscreen, and iOS Safari does not implement it at
 * all. So the table asks for the gesture once, uses it to go fullscreen and
 * lock where that is supported, and otherwise falls back to holding a
 * "rotate your device" prompt up until the viewport actually is landscape.
 */
export function useImmersive(): ImmersiveState {
  const [isPortrait, setIsPortrait] = useState(
    () => typeof window !== 'undefined' && window.innerHeight > window.innerWidth,
  )
  const [isFullscreen, setIsFullscreen] = useState(false)
  const [prompted, setPrompted] = useState(false)

  useEffect(() => {
    const query = window.matchMedia('(orientation: portrait)')
    const sync = () => setIsPortrait(query.matches)
    sync()
    query.addEventListener('change', sync)

    const syncFullscreen = () => setIsFullscreen(!!document.fullscreenElement)
    document.addEventListener('fullscreenchange', syncFullscreen)

    return () => {
      query.removeEventListener('change', sync)
      document.removeEventListener('fullscreenchange', syncFullscreen)
    }
  }, [])

  const enter = useCallback(async () => {
    setPrompted(true)
    try {
      if (!document.fullscreenElement) {
        await document.documentElement.requestFullscreen({ navigationUI: 'hide' })
      }
    } catch {
      // Fullscreen can be refused (iOS, or a permissions policy). The rotate
      // prompt below still gets the player to a usable landscape view.
    }
    try {
      const orientation = screen.orientation as ScreenOrientation & {
        lock?: (o: string) => Promise<void>
      }
      await orientation.lock?.('landscape')
    } catch {
      // Unsupported on iOS and on desktop; nothing to do but let them rotate.
    }
  }, [])

  return {
    isPortrait,
    isFullscreen,
    needsPrompt: !prompted || isPortrait,
    enter,
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
