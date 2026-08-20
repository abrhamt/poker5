import { useMemo } from 'react'
import type { TableState } from '../../lib/types'
import { useElementSize } from '../../lib/useElementSize'
import { CardSlot, PlayingCard } from '../Card'
import { EmptySeat, Seat } from './Seat'
import { rotateToHero, seatAnchors } from './seatLayout'
import { tableMetrics } from './tableMetrics'
import { ShowdownOverlay } from './ShowdownOverlay'

interface FeltProps {
  state: TableState
  heroName: string
  winningCards: Set<string>
  /** 0–1 of the acting player's turn clock still left. */
  turnFraction: number
  onSit?: () => void
  canSit: boolean
}

export function Felt({ state, heroName, winningCards, turnFraction, onSit, canSit }: FeltProps) {
  const [stageRef, stage] = useElementSize<HTMLDivElement>()

  const seats = rotateToHero(state.players ?? [], heroName)
  const seatCount = Math.max(state.max_players || 6, seats.length, 2)

  const metrics = useMemo(() => tableMetrics(stage), [stage])
  const anchors = useMemo(
    () => seatAnchors(seatCount, metrics.width, metrics.height),
    [seatCount, metrics.width, metrics.height],
  )

  const board = state.community_cards ?? []
  const revealed = state.intermission || state.game_finished
  const winnerName = state.winner?.name ?? ''
  const ready = stage.width > 0

  return (
    <div ref={stageRef} className="absolute inset-0">
      {!ready ? null : (
        <>
          {/* Rail: wood surround, gold trim, then the playing surface. */}
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

          {/* Pot and board, centred on the table rather than the viewport. */}
          <div
            className="pointer-events-none absolute flex flex-col items-center"
            style={{
              left: `${metrics.left + metrics.width / 2}px`,
              top: `${metrics.top + metrics.height / 2}px`,
              transform: 'translate(-50%, -50%)',
              gap: `${metrics.height * 0.045}px`,
            }}
          >
            <div className="flex items-center" style={{ gap: `${metrics.height * 0.04}px` }}>
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
              {Array.from({ length: 5 }, (_, i) =>
                board[i] ? (
                  <PlayingCard
                    key={i}
                    code={board[i]}
                    width={metrics.boardCard}
                    winning={winningCards.has(board[i])}
                    className="animate-rise"
                  />
                ) : (
                  <CardSlot key={i} width={metrics.boardCard} />
                ),
              )}
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
                isWinner={revealed && player.name === winnerName}
                winningCards={winningCards}
              />
            )
          })}

          {state.intermission ? <ShowdownOverlay state={state} metrics={metrics} /> : null}
        </>
      )}
    </div>
  )
}
