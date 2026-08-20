import type { User } from '../../lib/types'
import { DrawerLink, SideDrawer } from '../SideDrawer'

/** The table's slice of the shared drawer: the room code, and the way out. */
export function TableMenu({
  open,
  onClose,
  roomCode,
  user,
  seated,
  onLeave,
  onLogout,
}: {
  open: boolean
  onClose: () => void
  roomCode: string
  user: User | null
  seated: boolean
  onLeave: () => void
  onLogout: () => void
}) {
  return (
    <SideDrawer
      open={open}
      onClose={onClose}
      user={user}
      onLogout={onLogout}
      footer={
        seated ? (
          <button type="button" onClick={onLeave} className="btn-danger w-full">
            Leave &amp; Cash Out
          </button>
        ) : null
      }
    >
      <div className="rounded-xl border border-gold/20 bg-black/30 px-3 py-2.5">
        <span className="text-[10px] tracking-wide text-muted uppercase">Room code</span>
        <div className="mt-0.5 flex items-center justify-between gap-2">
          <span className="font-mono text-lg font-bold tracking-[0.2em] text-gold-light">
            {roomCode}
          </span>
          <button
            type="button"
            onClick={() => void navigator.clipboard?.writeText(roomCode)}
            className="rounded-lg px-2 py-1 text-[11px] font-semibold text-muted hover:text-gold-light"
          >
            Copy
          </button>
        </div>
      </div>

      <nav className="flex flex-col gap-1.5">
        <DrawerLink to="/lobby" label="Lobby" />
        <DrawerLink to="/wallet" label="Wallet" />
      </nav>
    </SideDrawer>
  )
}
