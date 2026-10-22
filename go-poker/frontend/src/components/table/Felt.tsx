import { Suspense, lazy, useEffect, useMemo, useRef, useState } from 'react'
import type { TableState } from '../../lib/types'
import { useElementSize } from '../../lib/useElementSize'
import { madeHandCards } from '../../lib/bestHand'
import { playChips, playDeal, playFanfare, playShuffle } from '../../lib/sound'
import { CardSlot, DealtCard, PlayingCard, isFaceUp, normalizeCardCode } from '../Card'
import { EmptySeat, Seat } from './Seat'
import { rotateToHero, seatAnchors, seatPoint } from './seatLayout'
import { tableMetrics } from './tableMetrics'
import type { TableMetrics } from './tableMetrics'
import { ShowdownOverlay, winnerNames } from './ShowdownOverlay'
import type { TableAward } from './three/TableCanvas'

// three.js is most of the table's weight, so it arrives on its own chunk; the
// flat felt stands in while it loads and wherever WebGL is unavailable.
const TableCanvas = lazy(() => import('./three/TableCanvas'))

/** Gap between consecutive cards of one deal (the flop's three). */
const DEAL_STAGGER_MS = 160

const EMPTY = new Set<string>()

interface FeltProps {
  state: TableState
  heroName: string
  /** 0–1 of the acting player's turn clock still left. */
  turnFraction: number
  onSit?: () => void
  canSit: boolean
}

export function Felt({ state, heroName, turnFraction, onSit, canSit }: FeltProps) {
  const [stageRef, stage] = useElementSize<HTMLDivElement>()
  const [webgl, setWebgl] = useState(true)

  const seats = rotateToHero(state.players ?? [], heroName)
  const seatCount = Math.max(state.max_players || 6, seats.length, 2)

  const metrics = useMemo(() => tableMetrics(stage), [stage])
  const anchors = useMemo(
    () => seatAnchors(seatCount, metrics.width, metrics.height),
    [seatCount, metrics.width, metrics.height],
  )

  const board = useMemo(() => state.community_cards ?? [], [state.community_cards])
  const revealed = state.intermission || state.game_finished
  const ready = stage.width > 0
  const H = metrics.height
  const cx = metrics.left + metrics.width / 2
  const cy = metrics.top + metrics.height / 2
  const hero = seats.find((p) => p.name === heroName)
  const winners = winnerNames(state)

  // Which cards to lift. At showdown: the winner's made hand (all five when
  // it is only a high card), with everything that did not play pushed back.
  // During a hand: whatever the hero has made so far, for their eyes only.
  const { highlight, dim } = useMemo(() => {
    const five = (state.winner?.winning_cards ?? []).filter(isFaceUp).map(normalizeCardCode)
    if (revealed && five.length) {
      const core = madeHandCards(five)
      const used = new Set(five)
      const unused = [...board, ...(state.winner?.hole_cards ?? [])]
        .filter(isFaceUp)
        .map(normalizeCardCode)
        .filter((c) => !used.has(c))
      return { highlight: core.size ? core : used, dim: new Set(unused) }
    }
    if (!revealed && hero && !hero.folded && isFaceUp(hero.cards?.[0])) {
      return { highlight: madeHandCards([...hero.cards, ...board]), dim: EMPTY }
    }
    return { highlight: EMPTY, dim: EMPTY }
  }, [revealed, state.winner, board, hero])

  // Each board card gets its deal delay once, the first time it is seen, so a
  // re-render mid-animation never restarts it. Cards already on the board
  // when the table opens are simply there.
  const dealDelays = useRef<Map<string, number> | null>(null)
  if (board.length === 0) dealDelays.current?.clear()
  if (!dealDelays.current) {
    dealDelays.current = new Map(board.map((c) => [c, -1]))
  } else {
    let batch = 0
    for (const code of board) {
      if (!dealDelays.current.has(code)) dealDelays.current.set(code, batch++ * DEAL_STAGGER_MS)
    }
  }

  const dealtCount = useRef(board.length)
  useEffect(() => {
    for (let i = dealtCount.current; i < board.length; i++) {
      playDeal(dealDelays.current?.get(board[i]) ?? 0)
    }
    dealtCount.current = board.length
  }, [board])

  // A chip clink whenever someone puts more in.
  const lastBets = useRef(new Map<string, number>())
  useEffect(() => {
    let raised = false
    for (const p of seats) {
      if (p.round_bet > (lastBets.current.get(p.name) ?? 0)) raised = true
      lastBets.current.set(p.name, p.round_bet)
    }
    if (raised) playChips()
  })

  // The deck is riffled as each new hand is dealt.
  const isPreflop = state.game_started && state.phase?.toLowerCase() === 'preflop'
  const wasPreflop = useRef(isPreflop)
  useEffect(() => {
    if (isPreflop && !wasPreflop.current) playShuffle()
    wasPreflop.current = isPreflop
  }, [isPreflop])

  // The payout. Keyed on the intermission so it plays once per hand — and not
  // at all for a hand that was already over when the table opened.
  const awardKey = state.intermission && state.winner ? String(state.intermission_ends_at_ms) : null
  const openedOnAward = useRef(awardKey)
  const freshAward = awardKey !== null && awardKey !== openedOnAward.current

  const avatarPoint = (name: string) => {
    const i = seats.findIndex((p) => p.name === name)
    if (i < 0) return { x: cx, y: cy }
    return seatPoint(anchors[i], metrics, 0, seats[i].name === heroName ? metrics.heroAvatarLateral : 0)
  }

  const award: TableAward | null =
    freshAward && state.winner
      ? {
          key: awardKey!,
          awards: state.pot_awards?.length
            ? state.pot_awards.map((a) => ({ amount: a.amount, to: avatarPoint(a.winner) }))
            : winners.map((n) => ({ amount: state.winner!.amount / winners.length, to: avatarPoint(n) })),
        }
      : null

  useEffect(() => {
    if (!awardKey || awardKey === openedOnAward.current) return
    playChips(120, 2)
    if (winners.includes(heroName)) playFanfare(250)
      // Once per payout; winners and heroName are read for that one moment.
  }, [awardKey])

  const betTotal = seats.reduce((sum, p) => sum + p.round_bet, 0)
  const potAt = { x: cx - H * 0.3, y: cy - H * 0.167 }
  const dealerIndex = seats.findIndex((p) => p.dealer)
  const dealerAt =
    dealerIndex >= 0 && state.game_started
      ? seats[dealerIndex].name === heroName
        ? seatPoint(anchors[dealerIndex], metrics, metrics.heroDealerInward, metrics.heroDealerLateral)
        : seatPoint(anchors[dealerIndex], metrics, metrics.dealerInward, metrics.dealerLateral)
      : null

  const bets = seats.flatMap((p, i) => {
    if (p.round_bet <= 0) return []
    const isHero = p.name === heroName
    const at = seatPoint(
      anchors[i],
      metrics,
      isHero ? metrics.heroBetInward : metrics.betInward,
      isHero ? metrics.heroBetLateral : metrics.betLateral,
    )
    return [{ id: p.name, amount: p.round_bet, at }]
  })

  // Board cards fly in from the deck, which sits just inside the far rail.
  const boardStep = metrics.boardCard + metrics.boardGap
  const deckOffset = (i: number) => ({ x: -(i - 2) * boardStep, y: metrics.top + H * 0.1 - (cy + H * 0.065) })

  return (
    <div ref={stageRef} className="absolute inset-0">
      {!ready ? null : (
        <>
          {webgl ? (
            <Suspense fallback={<FlatTable metrics={metrics} />}>
              <TableCanvas
                stage={stage}
                box={{ left: metrics.left, top: metrics.top, width: metrics.width, height: metrics.height }}
                bets={bets}
                pot={Math.max(0, state.pot - betTotal)}
                potAt={potAt}
                dealerAt={dealerAt}
                award={award}
                onFail={() => setWebgl(false)}
              />
            </Suspense>
          ) : (
            <FlatTable metrics={metrics} />
          )}

          {/* Pot and board, centred on the table rather than the viewport. */}
          <div
            className="pointer-events-none absolute flex flex-col items-center"
            style={{
              left: `${cx}px`,
              top: `${cy}px`,
              transform: 'translate(-50%, -50%)',
              gap: `${H * 0.045}px`,
            }}
          >
            {/* Kept in the layout but hidden while the result banner sits here. */}
            <div
              className={`flex items-center ${state.intermission ? 'invisible' : ''}`}
              style={{ gap: `${H * 0.04}px` }}
            >
              <span
                className="rounded-full bg-black/40 font-semibold tracking-[0.2em] text-white/45 uppercase"
                style={{
                  fontSize: `${metrics.tagSize}px`,
                  padding: `${metrics.tagSize * 0.15}px ${metrics.tagSize * 0.7}px`,
                }}
              >
                {state.phase}
              </span>
              <span
                className="flex items-center rounded-full border border-gold/30 bg-black/60"
                style={{
                  gap: `${metrics.chipsSize * 0.35}px`,
                  padding: `${metrics.chipsSize * 0.18}px ${metrics.chipsSize * 0.7}px`,
                }}
              >
                <span
                  className="rounded-full bg-linear-160 from-gold-bright to-gold-dark"
                  style={{ width: `${metrics.chipsSize * 0.6}px`, height: `${metrics.chipsSize * 0.6}px` }}
                />
                <span
                  className="font-heading font-bold text-gold-light"
                  style={{ fontSize: `${metrics.chipsSize * 1.2}px` }}
                >
                  {state.pot.toLocaleString()}
                </span>
              </span>
            </div>

            <div className="flex items-center" style={{ gap: `${metrics.boardGap}px` }}>
              {Array.from({ length: 5 }, (_, i) => {
                const code = board[i]
                if (!code) return <CardSlot key={`slot-${i}`} width={metrics.boardCard} />
                const winning = highlight.has(normalizeCardCode(code))
                const dimmed = dim.has(normalizeCardCode(code))
                const delay = dealDelays.current?.get(code) ?? -1
                return delay < 0 ? (
                  <PlayingCard key={code} code={code} width={metrics.boardCard} winning={winning} dim={dimmed} />
                ) : (
                  <DealtCard
                    key={code}
                    code={code}
                    width={metrics.boardCard}
                    delayMs={delay}
                    from={deckOffset(i)}
                    winning={winning}
                    dim={dimmed}
                  />
                )
              })}
            </div>
          </div>

          {anchors.map((anchor, i) => {
            const player = seats[i]
            if (!player) {
              return (
                <EmptySeat
                  key={`empty-${i}`}
                  anchor={anchor}
                  metrics={metrics}
                  onSit={onSit}
                  disabled={!canSit}
                />
              )
            }
            const share = freshAward && winners.includes(player.name) ? (state.winner?.amount ?? 0) / winners.length : 0
            return (
              <Seat
                key={player.name}
                player={player}
                anchor={anchor}
                metrics={metrics}
                isHero={player.name === heroName}
                isTurn={
                  player.name === state.current_turn_player &&
                  state.game_started &&
                  !state.intermission
                }
                turnFraction={turnFraction}
                revealed={revealed}
                isWinner={revealed && winners.includes(player.name)}
                highlightCards={player.name === heroName || winners.includes(player.name) ? highlight : EMPTY}
                dimCards={dim}
                winAmount={Math.round(share)}
                in3d={webgl}
              />
            )
          })}

          {state.intermission ? (
            <ShowdownOverlay
              state={state}
              metrics={metrics}
              heroName={heroName}
              stageHeight={stage.height}
              celebrate={freshAward}
            />
          ) : null}
        </>
      )}
    </div>
  )
}

/** The CSS felt: shown while the 3D table loads, and wherever WebGL is missing. */
function FlatTable({ metrics }: { metrics: TableMetrics }) {
  return (
    // Rail: wood surround, gold trim, then the playing surface.
    <div
      className="absolute rounded-full bg-linear-160 from-[#7a4d29] via-[#3d2213] to-[#20120a]"
      style={{
        left: `${metrics.left}px`,
        top: `${metrics.top}px`,
        width: `${metrics.width}px`,
        height: `${metrics.height}px`,
        padding: `${metrics.height * 0.055}px`,
        boxShadow: '0 12px 40px rgb(0 0 0 / 0.65)',
      }}
    >
      <div
        className="size-full rounded-full border-gold/25"
        style={{
          borderWidth: `${Math.max(1, metrics.height * 0.008)}px`,
          padding: `${metrics.height * 0.014}px`,
        }}
      >
        <div className="size-full rounded-full bg-[radial-gradient(ellipse_at_50%_36%,#12734e,#052c1c_74%)] shadow-[inset_0_0_60px_rgb(0_0_0/0.5)]" />
      </div>
    </div>
  )
}
