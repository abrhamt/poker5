import { useEffect, useLayoutEffect, useRef } from 'react'
import { PokerScene } from './PokerScene'
import type { SceneAward, SceneBet, StagePoint, TableBox } from './PokerScene'

export interface TableAward {
  /** Changes once per payout, so a re-render never replays it. */
  key: string
  awards: SceneAward[]
}

interface TableCanvasProps {
  stage: { width: number; height: number }
  box: TableBox
  bets: SceneBet[]
  pot: number
  potAt: StagePoint
  dealerAt: StagePoint | null
  award: TableAward | null
  /** Called with the flight time once a payout's chips are in the air. */
  onAward?: (key: string, flightMs: number) => void
  /** WebGL is unavailable or the context could not be created. */
  onFail: () => void
}

/**
 * The 3D table, drawn into a canvas behind the DOM seats and cards. Loaded on
 * its own chunk (three.js is the bulk of it), so the lobby never pays for it.
 */
export default function TableCanvas({
  stage,
  box,
  bets,
  pot,
  potAt,
  dealerAt,
  award,
  onAward,
  onFail,
}: TableCanvasProps) {
  const canvasRef = useRef<HTMLCanvasElement>(null)
  const overlayRef = useRef<HTMLCanvasElement>(null)
  const sceneRef = useRef<PokerScene | null>(null)
  const playedAward = useRef<string | null>(null)

  useLayoutEffect(() => {
    try {
      sceneRef.current = new PokerScene(canvasRef.current!, overlayRef.current ?? undefined)
    } catch {
      onFail()
    }
    return () => {
      sceneRef.current?.dispose()
      sceneRef.current = null
    }
    // The scene lives as long as the canvas; onFail is only read on creation.
  }, [])

  // Keyed on the numbers, not the objects, which are rebuilt every render.
  useLayoutEffect(() => {
    sceneRef.current?.layout(stage, box)
  }, [stage.width, stage.height, box.left, box.top, box.width, box.height])

  useEffect(() => {
    const scene = sceneRef.current
    if (!scene) return
    scene.setBets(bets)
    scene.setPot(pot, potAt)
    scene.setDealer(dealerAt)
  })

  useEffect(() => {
    const scene = sceneRef.current
    if (!scene || !award || playedAward.current === award.key) return
    playedAward.current = award.key
    // Not inlined into the optional call: `onAward?.(…)` skips evaluating its
    // arguments when there is no listener, which would skip the payout too.
    const flightMs = scene.award(award.awards)
    onAward?.(award.key, flightMs)
  }, [award, onAward])

  return (
    <>
      <canvas ref={canvasRef} className="pointer-events-none absolute inset-0 block size-full" />
      {/* Above the cards and seats, below the result banner: the payout's
          chips fly over the board rather than under it. */}
      <canvas ref={overlayRef} className="pointer-events-none absolute inset-0 z-45 block size-full" />
    </>
  )
}
