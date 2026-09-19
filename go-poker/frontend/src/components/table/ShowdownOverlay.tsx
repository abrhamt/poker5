import type { TableState } from '../../lib/types'
import type { TableMetrics } from './tableMetrics'
import { PlayingCard } from '../Card'
import { useCountdown } from '../../lib/useCountdown'

/**
 * The hand result, centred on the felt during the intermission.
 *
 * Sized off the table like everything else rather than in fixed pixels: this
 * is the moment the player most wants to read, so the winning cards are drawn
 * at roughly two-thirds of board size instead of the thumbnails they were.
 */
export function ShowdownOverlay({
  state,
  metrics,
}: {
  state: TableState
  metrics: TableMetrics
}) {
  const nextIn = useCountdown(state.intermission_ends_at_ms, true)
  const winner = state.winner
  const cards = (winner?.winning_cards ?? []).filter((c) => c && c !== '1B')

  const H = metrics.height
  const cardWidth = H * 0.145

  return (
    <div
      className="pointer-events-none absolute z-50 flex justify-center"
      style={{
        left: `${metrics.left + metrics.width / 2}px`,
        top: `${metrics.top + metrics.height / 2}px`,
        // Wide enough to cover the board it sits on: five board cards plus
        // their gaps come to a little over one table-height across, and a
        // panel narrower than that leaves cards poking out either side.
        width: `${Math.max(metrics.width * 0.62, H * 1.45)}px`,
        transform: 'translate(-50%, -50%)',
      }}
    >
      <div
        className="animate-rise flex w-full flex-col items-center rounded-2xl border border-gold/45 bg-ink/92 text-center shadow-gold backdrop-blur-md"
        style={{
          gap: `${H * 0.022}px`,
          padding: `${H * 0.05}px ${H * 0.09}px`,
        }}
      >
        <span
          className="font-heading font-bold text-gold-light"
          style={{ fontSize: `${H * 0.075}px`, lineHeight: 1.15 }}
        >
          {winner?.name ?? 'Winner'} wins
          {winner?.amount ? ` ${winner.amount.toLocaleString()}` : ''}
        </span>

        <span className="text-white/70" style={{ fontSize: `${H * 0.045}px` }}>
          {winner?.hand_name || 'Opponents folded'}
        </span>

        {cards.length > 0 ? (
          <div className="flex" style={{ gap: `${cardWidth * 0.12}px`, marginTop: `${H * 0.015}px` }}>
            {cards.map((code, i) => (
              <PlayingCard key={`${code}-${i}`} code={code} width={cardWidth} winning />
            ))}
          </div>
        ) : null}

        <span className="text-muted" style={{ fontSize: `${H * 0.038}px` }}>
          Next hand in {nextIn}s
        </span>
      </div>
    </div>
  )
}
