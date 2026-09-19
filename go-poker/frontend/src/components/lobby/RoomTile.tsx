import { Link } from 'react-router-dom'
import type { PublicRoom } from '../../lib/types'
import { useCountdown } from '../../lib/useCountdown'
import { TierPiece } from './TierPiece'

/**
 * One table, at a glance and without a word of prose: the tier piece, the room
 * code, a dot per seat, and the buy-in. Anything that needs explaining lives in
 * the "?" panel instead of being repeated on every tile.
 */
export function RoomTile({ room, tierIndex }: { room: PublicRoom; tierIndex: number }) {
  const startsIn = useCountdown(room.countdown_ends_at_ms, room.countdown_active)
  const seats = Math.max(room.max_players, 1)
  const full = room.player_count >= seats

  return (
    <Link
      to={`/table/${room.room_code}`}
      className="group flex flex-col justify-between rounded-xl border border-white/10 bg-panel/60 p-2.5 transition-colors hover:border-gold/50 hover:bg-panel"
    >
      <div className="flex items-center justify-between gap-1">
        <span className="text-gold-light" title={`${room.small_blind}/${room.big_blind} blinds`}>
          <TierPiece index={tierIndex} size={15} />
        </span>
        <span className="font-mono text-[11px] tracking-wider text-muted">{room.room_code}</span>
      </div>

      <div className="flex items-center gap-[3px] py-1.5" aria-label={`${room.player_count} of ${seats} seats taken`}>
        {Array.from({ length: seats }, (_, i) => (
          <span
            key={i}
            className={`h-1.5 flex-1 rounded-full ${
              i < room.player_count ? (full ? 'bg-danger' : 'bg-emerald-400') : 'bg-white/12'
            }`}
          />
        ))}
      </div>

      <div className="flex items-center justify-between gap-1">
        <span className="text-[13px] font-bold text-chalk tabular-nums">
          {room.buy_in.toLocaleString()}
          <span className="ml-0.5 text-[9px] font-medium text-muted">ETB</span>
        </span>
        <Status room={room} startsIn={startsIn} />
      </div>
    </Link>
  )
}

function Status({ room, startsIn }: { room: PublicRoom; startsIn: number }) {
  if (room.countdown_active) {
    return (
      <span className="rounded-full bg-gold/20 px-1.5 py-px text-[10px] font-bold text-gold-light tabular-nums">
        {startsIn}s
      </span>
    )
  }
  if (room.game_started) {
    return <span className="size-1.5 animate-pulse rounded-full bg-emerald-400" title="Hand in progress" />
  }
  return <span className="size-1.5 rounded-full bg-white/20" title="Waiting for players" />
}
