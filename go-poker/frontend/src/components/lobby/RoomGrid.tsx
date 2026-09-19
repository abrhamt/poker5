import { useEffect, useMemo, useState } from 'react'
import type { PublicRoom } from '../../lib/types'
import { useElementSize } from '../../lib/useElementSize'
import { RoomTile } from './RoomTile'

const TILE_MIN_WIDTH = 132
// Capacity is worked out at the minimum height; tiles then grow into whatever
// slack is left, up to the maximum. Most of the time there are only a couple of
// tables running, and a handful of small tiles stranded in a tall empty column
// reads as broken rather than as breathing room.
const TILE_MIN_HEIGHT = 84
const TILE_MAX_HEIGHT = 128
const GAP = 8

/**
 * Fills the space it is given and pages the overflow. The page size is measured
 * rather than fixed, which is what keeps the lobby free of a scrollbar on every
 * screen from a small phone to a desktop — the grid never asks for more room
 * than it has.
 */
export function RoomGrid({
  rooms,
  tierIndex,
  onQuickJoin,
  busy,
}: {
  rooms: PublicRoom[]
  tierIndex: number
  onQuickJoin: () => void
  busy: boolean
}) {
  const [ref, size] = useElementSize<HTMLDivElement>()
  const [page, setPage] = useState(0)

  const { columns, pageSize } = useMemo(() => {
    if (size.width === 0 || size.height === 0) return { columns: 2, pageSize: 4 }
    const cols = Math.max(1, Math.floor((size.width + GAP) / (TILE_MIN_WIDTH + GAP)))
    const rows = Math.max(1, Math.floor((size.height + GAP) / (TILE_MIN_HEIGHT + GAP)))
    return { columns: cols, pageSize: cols * rows }
  }, [size])

  const totalPages = Math.max(1, Math.ceil(rooms.length / pageSize))

  // A tier change, a table closing, or a narrower window can all put us past
  // the end; slide back rather than showing an empty grid.
  useEffect(() => {
    setPage((current) => Math.min(current, totalPages - 1))
  }, [totalPages])
  useEffect(() => {
    setPage(0)
  }, [tierIndex])

  const visible = rooms.slice(page * pageSize, page * pageSize + pageSize)

  // Grow the rows into any leftover height. Capacity was measured against the
  // minimum, so this can only ever take up slack — never cause an overflow.
  const rowsNeeded = Math.max(1, Math.ceil(visible.length / columns))
  const rowHeight = Math.min(
    TILE_MAX_HEIGHT,
    Math.max(TILE_MIN_HEIGHT, (size.height - (rowsNeeded - 1) * GAP) / rowsNeeded),
  )

  return (
    <div className="flex min-h-0 flex-1 flex-col gap-1.5">
      <div ref={ref} className="min-h-0 flex-1 overflow-hidden">
        {rooms.length === 0 ? (
          <div className="flex size-full flex-col items-center justify-center gap-3 rounded-xl border border-dashed border-white/10">
            <p className="text-[13px] text-muted">No tables running at this level.</p>
            <button type="button" onClick={onQuickJoin} disabled={busy} className="btn-gold px-4 py-2 text-xs">
              Start one
            </button>
          </div>
        ) : (
          <div
            className="grid size-full"
            style={{
              gap: `${GAP}px`,
              gridTemplateColumns: `repeat(${columns}, minmax(0, 1fr))`,
              gridAutoRows: `${rowHeight}px`,
              alignContent: 'center',
            }}
          >
            {visible.map((room) => (
              <RoomTile key={room.room_code} room={room} tierIndex={tierIndex} />
            ))}
          </div>
        )}
      </div>

      {totalPages > 1 ? (
        <div className="flex items-center justify-center gap-2">
          <PagerButton label="Previous tables" disabled={page === 0} onClick={() => setPage((p) => p - 1)}>
            <polyline points="15 18 9 12 15 6" />
          </PagerButton>
          <span className="text-[11px] text-muted tabular-nums">
            {page + 1} / {totalPages}
          </span>
          <PagerButton
            label="More tables"
            disabled={page >= totalPages - 1}
            onClick={() => setPage((p) => p + 1)}
          >
            <polyline points="9 18 15 12 9 6" />
          </PagerButton>
        </div>
      ) : null}
    </div>
  )
}

function PagerButton({
  label,
  disabled,
  onClick,
  children,
}: {
  label: string
  disabled: boolean
  onClick: () => void
  children: React.ReactNode
}) {
  return (
    <button
      type="button"
      aria-label={label}
      disabled={disabled}
      onClick={onClick}
      className="grid size-6 place-items-center rounded-md border border-white/10 bg-white/5 text-muted enabled:hover:text-chalk disabled:opacity-30"
    >
      <svg viewBox="0 0 24 24" width="12" height="12" fill="none" stroke="currentColor" strokeWidth="2.5" strokeLinecap="round" strokeLinejoin="round">
        {children}
      </svg>
    </button>
  )
}
