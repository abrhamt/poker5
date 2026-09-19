/** Card art ships with this bundle, in public/cards/<CODE>.svg.
 *  "1B" is the face-down back. */

const BACK = '1B'

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
  className = '',
}: {
  code: string | undefined
  width: number
  winning?: boolean
  className?: string
}) {
  const face = isFaceUp(code) ? normalizeCardCode(code) : BACK

  return (
    <img
      src={`/cards/${face}.svg`}
      alt={face === BACK ? 'Face-down card' : face}
      draggable={false}
      style={{
        width: `${width}px`,
        borderRadius: `${Math.max(2, width * 0.1)}px`,
        boxShadow: winning
          ? '0 0 0 2px var(--color-gold), 0 4px 12px rgb(0 0 0 / 0.55)'
          : '0 2px 6px rgb(0 0 0 / 0.5)',
      }}
      className={`aspect-[5/7] shrink-0 select-none object-contain transition-transform duration-200 ${
        winning ? '-translate-y-1' : ''
      } ${className}`}
    />
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
