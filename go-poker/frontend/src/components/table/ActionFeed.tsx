import type { FeedLine } from '../../lib/useActionFeed'

/**
 * A running commentary of the hand, stacked up the left edge.
 *
 * Deliberately inert: absolutely positioned and pointer-events-none, so it can
 * never take a tap meant for the felt or shift a single seat.
 */
export function ActionFeed({ lines }: { lines: FeedLine[] }) {
  if (lines.length === 0) return null

  return (
    <div className="pointer-events-none absolute bottom-2 left-2 z-30 flex max-w-[46%] flex-col gap-1 sm:bottom-3 sm:left-3">
      {lines.map((line) => (
        <span
          key={line.seq}
          className={`animate-slide-up truncate rounded-lg border px-2 py-1 text-[11px] backdrop-blur-sm ${
            line.kind === 'result'
              ? 'border-gold/35 bg-black/65 font-semibold text-gold-light'
              : 'border-white/8 bg-black/55 text-white/70'
          }`}
        >
          {line.text}
        </span>
      ))}
    </div>
  )
}
