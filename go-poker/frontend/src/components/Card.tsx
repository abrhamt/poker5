/** Card art ships with this bundle, in public/cards/<CODE>.svg.
 *  "1B" is the face-down code the server sends; its art is the house back. */

const BACK = '1B'
const BACK_ART = '/cards/back.webp'

/** Canonicalises a loosely-written code onto a real filename in the deck:
 *  "10h" -> "TH", "as" -> "AS". The server sends canonical codes already, so
 *  this only guards against hand-written or legacy values. */
export function normalizeCardCode(code: string): string {
  if (code.length < 2) return code
  if (code.length === 3 && code.startsWith('10')) return `T${code[2].toUpperCase()}`
  return code.slice(0, 2).toUpperCase()
}

export function isFaceUp(code: string | undefined): code is string {
  return !!code && code !== BACK
}

/** Cards are sized in pixels off the measured table, so the felt scales as one. */
export function PlayingCard({
  code,
  width,
  winning = false,
  dim = false,
  className = '',
}: {
  code: string | undefined
  width: number
  /** Part of the made hand: lifted off the felt and ringed in gold. */
  winning?: boolean
  /** Played no part in the hand that won: pushed back. */
  dim?: boolean
  className?: string
}) {
  const face = isFaceUp(code) ? normalizeCardCode(code) : BACK

  return (
    <img
      src={face === BACK ? BACK_ART : `/cards/${face}.svg`}
      alt={face === BACK ? 'Face-down card' : face}
      draggable={false}
      style={{
        width: `${width}px`,
        borderRadius: `${Math.max(2, width * 0.1)}px`,
        boxShadow: winning
          ? '0 0 0 2px var(--color-gold-bright), 0 0 14px rgb(255 215 0 / 0.6), 0 6px 14px rgb(0 0 0 / 0.55)'
          : '0 2px 6px rgb(0 0 0 / 0.5)',
      }}
      className={`aspect-[5/7] shrink-0 select-none object-cover transition-[translate,filter] duration-300 ${
        winning ? '-translate-y-[14%]' : ''
      } ${dim ? 'brightness-50 saturate-50' : ''} ${className}`}
    />
  )
}

/**
 * A board card arriving: it flies in from the deck face-down and turns over
 * as it lands. `from` is the deck's offset from this card's slot.
 */
export function DealtCard({
  code,
  width,
  delayMs,
  from,
  winning,
  dim,
}: {
  code: string
  width: number
  delayMs: number
  from: { x: number; y: number }
  winning?: boolean
  dim?: boolean
}) {
  return (
    <div
      className="animate-deal-in relative shrink-0 perspective-[700px]"
      style={
        {
          width: `${width}px`,
          '--deal-x': `${from.x}px`,
          '--deal-y': `${from.y}px`,
          animationDelay: `${delayMs}ms`,
        } as React.CSSProperties
      }
    >
      <div className="animate-card-flip relative transform-3d" style={{ animationDelay: `${delayMs + 200}ms` }}>
        <PlayingCard code={code} width={width} winning={winning} dim={dim} className="backface-hidden" />
        <PlayingCard code={BACK} width={width} className="absolute inset-0 rotate-y-180 backface-hidden" />
      </div>
    </div>
  )
}

/** An undealt slot on the board, so the felt keeps a stable five-card layout. */
export function CardSlot({ width }: { width: number }) {
  return (
    <div
      style={{
        width: `${width}px`,
        borderRadius: `${Math.max(2, width * 0.1)}px`,
        fontSize: `${width * 0.45}px`,
      }}
      className="grid aspect-[5/7] shrink-0 place-items-center border border-white/8 bg-black/25 font-bold text-white/15"
    >
      ?
    </div>
  )
}
