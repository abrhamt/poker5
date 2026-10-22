import type { User } from '../../lib/types'
import { setSoundEnabled, unlockAudio, useSoundEnabled } from '../../lib/sound'
import { SideDrawer } from '../SideDrawer'
import { AccountNav } from '../PageShell'

/** Table sounds on or off, remembered on this device. */
function SoundToggle() {
  const on = useSoundEnabled()
  return (
    <button
      type="button"
      role="switch"
      aria-checked={on}
      onClick={() => {
        unlockAudio()
        setSoundEnabled(!on)
      }}
      className="flex items-center justify-between rounded-xl border border-white/10 bg-black/30 px-3 py-2.5 text-sm"
    >
      <span className="font-semibold">Sound effects</span>
      <span
        className={`relative h-5 w-9 rounded-full transition-colors ${on ? 'bg-gold' : 'bg-white/15'}`}
      >
        <span
          className={`absolute top-0.5 size-4 rounded-full bg-white shadow transition-[left] ${on ? 'left-4.5' : 'left-0.5'}`}
        />
      </span>
    </button>
  )
}

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

      <SoundToggle />

      <AccountNav />
    </SideDrawer>
  )
}
