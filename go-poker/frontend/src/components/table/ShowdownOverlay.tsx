import { useMemo } from 'react'
import type { TableState } from '../../lib/types'
import type { TableMetrics } from './tableMetrics'
import { useCountdown } from '../../lib/useCountdown'
import { handCategory } from '../../lib/handStrength'
import { PlayerAvatar } from '../PlayerAvatar'

/** Everyone who takes a share of this hand's pot. */
export function winnerNames(state: TableState): string[] {
  const w = state.winner
  if (!w) return []
  return [w.name, ...(w.tied_with ?? []).filter((n) => n !== w.name)]
}

/**
 * The hand result, as a banner in the phase/pot row above the board (the
 * felt hides that row for the intermission — the pot is paid out by then).
 *
 * It deliberately leaves the board uncovered: the cards that made the winning
 * hand are lifted in place on the felt and at the winner's seat, which says
 * more than a second copy of them would.
 */
export function ShowdownOverlay({
  state,
  metrics,
  heroName,
  stageHeight,
  celebrate,
}: {
  state: TableState
  metrics: TableMetrics
  heroName: string
  stageHeight: number
  /** False when the table opened on a hand that was already over. */
  celebrate: boolean
}) {
  const nextIn = useCountdown(state.intermission_ends_at_ms, true)
  const winner = state.winner
  const names = winnerNames(state)
  const heroWon = names.includes(heroName)
  const H = metrics.height

  const headline = !winner
    ? 'Hand over'
    : names.length > 1
      ? `Split pot: ${names.map((n) => (n === heroName ? 'You' : n)).join(' & ')}`
      : `${winner.name === heroName ? 'You win' : `${winner.name} wins`}${
          winner.amount ? ` ${winner.amount.toLocaleString()}` : ''
        }`

  return (
    <>
      <div
        className="pointer-events-none absolute z-50"
        style={{
          left: `${metrics.left + metrics.width / 2}px`,
          top: `${metrics.top + metrics.height / 2 - H * 0.18}px`,
          transform: 'translate(-50%, -50%)',
        }}
      >
        <div
          className="animate-rise flex items-center rounded-full border border-gold/50 bg-ink/88 whitespace-nowrap shadow-gold backdrop-blur-md [animation-fill-mode:both]"
          style={{
            gap: `${H * 0.03}px`,
            padding: `${H * 0.014}px ${H * 0.05}px ${H * 0.014}px ${H * 0.016}px`,
            // When the hero wins, let "YOU WIN!" have the moment first.
            animationDelay: heroWon && celebrate ? '1.9s' : undefined,
          }}
        >
          {winner ? (
            <span
              className="shrink-0 overflow-hidden rounded-full ring-2 ring-gold"
              style={{ width: `${H * 0.085}px`, height: `${H * 0.085}px` }}
            >
              <PlayerAvatar
                name={winner.name}
                avatar={state.players?.find((p) => p.name === winner.name)?.avatar}
                className="block size-full"
              />
            </span>
          ) : null}
          <span className="flex flex-col leading-tight">
            <span className="font-heading font-bold text-gold-light" style={{ fontSize: `${H * 0.052}px` }}>
              {headline}
            </span>
            <span className="text-white/65" style={{ fontSize: `${H * 0.034}px` }}>
              {winner?.hand_name ? handCategory(winner.hand_name) : 'Everyone else folded'}
              {' · '}
              Next hand in {nextIn}s
            </span>
          </span>
        </div>
      </div>

      {heroWon && celebrate ? <Celebration metrics={metrics} stageHeight={stageHeight} amount={winner?.amount ?? 0} /> : null}
    </>
  )
}

const CONFETTI_COLOURS = ['#ffd700', '#f3e5ab', '#d4af37', '#ffffff', '#1b9564', '#dc2626', '#2563eb']

/** "You win!" over the table, with a burst of confetti. Mounted once per win. */
function Celebration({
  metrics,
  stageHeight,
  amount,
}: {
  metrics: TableMetrics
  stageHeight: number
  amount: number
}) {
  const H = metrics.height

  // Fixed per mount, so a re-render mid-fall never reshuffles the pieces.
  const pieces = useMemo(() => {
    let seed = 42
    const rand = () => {
      seed = (seed * 16807) % 2147483647
      return seed / 2147483647
    }
    return Array.from({ length: 42 }, (_, i) => {
      const top = metrics.top - H * 0.08 * rand()
      return {
        key: i,
        left: metrics.left + rand() * metrics.width,
        top,
        size: H * (0.018 + rand() * 0.02),
        colour: CONFETTI_COLOURS[i % CONFETTI_COLOURS.length],
        round: rand() > 0.6,
        drift: (rand() - 0.5) * H * 0.6,
        fall: stageHeight - top + H * 0.1,
        spin: 360 + rand() * 720,
        delay: rand() * 450,
        duration: 1700 + rand() * 900,
      }
    })
  }, [metrics.left, metrics.top, metrics.width, H, stageHeight])

  return (
    <div className="pointer-events-none absolute inset-0 z-60 overflow-hidden">
      {pieces.map((p) => (
        <span
          key={p.key}
          className="animate-confetti absolute"
          style={
            {
              left: `${p.left}px`,
              top: `${p.top}px`,
              width: `${p.size}px`,
              height: `${p.round ? p.size : p.size * 0.45}px`,
              borderRadius: p.round ? '9999px' : '2px',
              background: p.colour,
              '--drift': `${p.drift}px`,
              '--fall': `${p.fall}px`,
              '--spin': `${p.spin}deg`,
              animationDelay: `${p.delay}ms`,
              animationDuration: `${p.duration}ms`,
            } as React.CSSProperties
          }
        />
      ))}

      <div
        className="animate-win-pop absolute flex flex-col items-center"
        style={{ left: `${metrics.left + metrics.width / 2}px`, top: `${metrics.top + metrics.height * 0.42}px` }}
      >
        <span
          className="bg-linear-to-b from-[#fff6c8] via-gold-bright to-gold-dark bg-clip-text font-heading font-black tracking-wide text-transparent"
          style={{
            fontSize: `${H * 0.2}px`,
            lineHeight: 1,
            filter: 'drop-shadow(0 4px 10px rgb(0 0 0 / 0.65)) drop-shadow(0 0 18px rgb(255 215 0 / 0.45))',
          }}
        >
          YOU WIN!
        </span>
        {amount > 0 ? (
          <span
            className="font-heading font-bold text-gold-light"
            style={{ fontSize: `${H * 0.08}px`, textShadow: '0 2px 8px rgb(0 0 0 / 0.7)' }}
          >
            +{amount.toLocaleString()}
          </span>
        ) : null}
      </div>
    </div>
  )
}
