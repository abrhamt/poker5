/**
 * Stake tiers are marked with a chess piece rather than a text label — rank
 * maps to stake, it reads instantly, and it costs a glyph instead of a line of
 * copy. The "?" panel spells the mapping out for anyone who wants it.
 *
 * These are drawn rather than typed. The Unicode chess characters are not
 * dependable: several platforms have no text glyph for ♞/♜ and render tofu,
 * and others substitute a colour emoji that ignores `color` entirely, which on
 * a dark theme comes out unreadable either way.
 */
const PIECES = [
  // Pawn
  <>
    <circle cx="12" cy="6" r="3.2" />
    <path d="M9.2 9.4c-.6 1.6-1.7 2.4-1.7 4.1 0 1.9 1.3 2.6 1.3 4.5h6.4c0-1.9 1.3-2.6 1.3-4.5 0-1.7-1.1-2.5-1.7-4.1z" />
    <rect x="5.5" y="18" width="13" height="3.5" rx="1.2" />
  </>,
  // Knight
  <>
    <path d="M8.5 3.2c.4 1.3.3 2.2-.4 3.1L6 8.7c-1 1.4-1.2 2.8-.6 4l1.9-1.4.6 1.5 2.4-1.6c-1.3 2-3.4 3.4-4.1 5.6-.3 1-.4 1.9-.4 2.9h11c0-4.4-.4-7.6-1.6-10.1-.9-1.9-2.7-3.6-5.1-5.2z" />
    <rect x="4.8" y="18.2" width="14.4" height="3.4" rx="1.2" />
  </>,
  // Rook
  <>
    <path d="M5.5 3h2.6v2h2.6V3h2.6v2h2.6V3h2.6v5l-1.8 1.6.7 8.4H6.6l.7-8.4L5.5 8z" />
    <rect x="4.6" y="18.2" width="14.8" height="3.4" rx="1.2" />
  </>,
  // Queen
  <>
    <circle cx="12" cy="3.4" r="1.7" />
    <circle cx="4.6" cy="7.2" r="1.6" />
    <circle cx="19.4" cy="7.2" r="1.6" />
    <path d="M5.2 8.8 7.5 15.4h9L18.8 8.8l-3.7 2.7L12 5.8l-3.1 5.7z" />
    <path d="M7.1 16.6h9.8l.4 1.6H6.7z" />
    <rect x="5" y="18.4" width="14" height="3.2" rx="1.2" />
  </>,
  // King
  <>
    <path d="M11 2h2v1.8h1.8v2H13V8h-2V5.8H9.2v-2H11z" />
    <path d="M12 8.4c3.4 0 6 1.9 6 4.4 0 1.8-1.2 3.3-3 4.1l.4 1.5H8.6l.4-1.5c-1.8-.8-3-2.3-3-4.1 0-2.5 2.6-4.4 6-4.4z" />
    <rect x="5" y="18.4" width="14" height="3.2" rx="1.2" />
  </>,
]

const NAMES = ['Pawn', 'Knight', 'Rook', 'Queen', 'King'] as const

const clamp = (index: number) => Math.min(Math.max(index, 0), PIECES.length - 1)

export function TierPiece({ index, size = 16 }: { index: number; size?: number }) {
  return (
    <svg
      viewBox="0 0 24 24"
      width={size}
      height={size}
      fill="currentColor"
      aria-hidden="true"
      className="shrink-0"
    >
      {PIECES[clamp(index)]}
    </svg>
  )
}

export function tierName(index: number): string {
  return NAMES[clamp(index)]
}
