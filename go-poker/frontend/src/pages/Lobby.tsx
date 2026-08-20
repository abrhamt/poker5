import { useCallback, useEffect, useState } from 'react'
import { Link, useNavigate } from 'react-router-dom'
import { api } from '../lib/api'
import type { BlindTier, PublicRoom } from '../lib/types'
import { useSSE } from '../lib/useSSE'
import { useAuth } from '../lib/auth'
import { useToast } from '../components/Toast'
import { Spinner } from '../components/Spinner'
import { DrawerLink, SideDrawer } from '../components/SideDrawer'
import { RoomGrid } from '../components/lobby/RoomGrid'
import { Sheet } from '../components/Sheet'
import { HelpPanel } from '../components/lobby/HelpPanel'
import { TierPiece } from '../components/lobby/TierPiece'

type SheetName = 'help' | 'private' | 'code' | null

export default function Lobby() {
  const { user, logout } = useAuth()
  const navigate = useNavigate()
  const toast = useToast()

  const [rooms, setRooms] = useState<PublicRoom[]>([])
  const [tiers, setTiers] = useState<BlindTier[]>([])
  const [activeRoom, setActiveRoom] = useState('')
  const [tierIndex, setTierIndex] = useState(0)
  const [loading, setLoading] = useState(true)
  const [busy, setBusy] = useState(false)
  const [menuOpen, setMenuOpen] = useState(false)
  const [sheet, setSheet] = useState<SheetName>(null)

  // The lobby owns the viewport the same way the table does — nothing scrolls.
  useEffect(() => {
    const previous = document.body.style.overflow
    document.body.style.overflow = 'hidden'
    return () => {
      document.body.style.overflow = previous
    }
  }, [])

  const load = useCallback(async () => {
    try {
      const data = await api.rooms.listPublic()
      setRooms(data.rooms ?? [])
      setTiers(data.tiers ?? [])
      setActiveRoom(data.active_room_code ?? '')
    } catch (err) {
      toast(err instanceof Error ? err.message : 'Could not load tables.', 'error')
    } finally {
      setLoading(false)
    }
  }, [toast])

  useEffect(() => {
    void load()
  }, [load])

  // Seat counts and start countdowns move on their own, so refresh on every
  // lobby broadcast and keep a slow poll as a safety net for missed events.
  useSSE('lobby', { 'lobby-update': load })
  useEffect(() => {
    const id = setInterval(load, 15000)
    return () => clearInterval(id)
  }, [load])

  const tier = tiers[tierIndex]
  const tierRooms = tier ? rooms.filter((r) => r.small_blind === tier.small_blind) : []

  const run = useCallback(
    async (fn: () => Promise<{ room_code: string }>) => {
      setBusy(true)
      try {
        const { room_code } = await fn()
        navigate(`/table/${room_code}`)
      } catch (err) {
        toast(err instanceof Error ? err.message : 'That did not work.', 'error')
        setBusy(false)
      }
    },
    [navigate, toast],
  )

  const quickJoin = useCallback(() => {
    if (!tier) return
    void run(() => api.rooms.quickJoin(tier.small_blind))
  }, [run, tier])

  if (loading) return <Spinner label="Loading tables…" />

  return (
    <div className="fixed inset-0 flex flex-col overflow-hidden">
      <TopBar
        wallet={user?.wallet ?? 0}
        onOpenMenu={() => setMenuOpen(true)}
        onOpenHelp={() => setSheet('help')}
      />

      {/* Capped: this is a phone-first screen, and left to fill a desktop the
          tiles stretch into unreadable letterboxes. */}
      <main className="mx-auto flex min-h-0 w-full max-w-2xl flex-1 flex-col gap-2 px-3 pt-2 pb-3">
        {activeRoom ? <ResumeStrip roomCode={activeRoom} /> : null}

        <TierPicker tiers={tiers} selected={tierIndex} onSelect={setTierIndex} />

        <RoomGrid rooms={tierRooms} tierIndex={tierIndex} onQuickJoin={quickJoin} busy={busy} />

        <div className="flex items-stretch gap-2">
          <button type="button" onClick={quickJoin} disabled={busy || !tier} className="btn-gold flex-1">
            {busy ? 'Seating…' : 'Quick Join'}
          </button>
          <IconButton label="Create a private table" onClick={() => setSheet('private')}>
            <line x1="12" y1="5" x2="12" y2="19" />
            <line x1="5" y1="12" x2="19" y2="12" />
          </IconButton>
          <IconButton label="Join with a room code" onClick={() => setSheet('code')}>
            <line x1="4" y1="9" x2="20" y2="9" />
            <line x1="4" y1="15" x2="20" y2="15" />
            <line x1="10" y1="3" x2="8" y2="21" />
            <line x1="16" y1="3" x2="14" y2="21" />
          </IconButton>
        </div>
      </main>

      <SideDrawer
        open={menuOpen}
        onClose={() => setMenuOpen(false)}
        user={user}
        onLogout={async () => {
          await logout()
          navigate('/login', { replace: true })
        }}
        footer={
          activeRoom ? (
            <Link to={`/table/${activeRoom}`} className="btn-gold w-full">
              Back to table #{activeRoom}
            </Link>
          ) : null
        }
      >
        <nav className="flex flex-col gap-1.5">
          <DrawerLink to="/wallet" label="Wallet" />
        </nav>

        <div className="rounded-xl border border-gold/20 bg-black/30 px-3 py-2.5">
          <span className="text-[10px] tracking-wide text-muted uppercase">Referral code</span>
          <p className="mt-0.5 font-mono text-base font-bold tracking-widest text-gold-light">
            {user?.referral_code}
          </p>
        </div>
      </SideDrawer>

      <Sheet open={sheet === 'help'} title="What the tables show" onClose={() => setSheet(null)}>
        <HelpPanel tiers={tiers} />
      </Sheet>

      <Sheet open={sheet === 'private'} title="Create a private table" onClose={() => setSheet(null)}>
        <CreatePrivateForm busy={busy} onSubmit={(input) => void run(() => api.rooms.createPrivate(input))} />
      </Sheet>

      <Sheet open={sheet === 'code'} title="Join with a code" onClose={() => setSheet(null)}>
        <JoinCodeForm busy={busy} onSubmit={(code) => void run(() => api.rooms.joinByCode(code))} />
      </Sheet>
    </div>
  )
}

function TopBar({
  wallet,
  onOpenMenu,
  onOpenHelp,
}: {
  wallet: number
  onOpenMenu: () => void
  onOpenHelp: () => void
}) {
  return (
    <header className="shrink-0 border-b border-gold/20 bg-ink/80 backdrop-blur-xl">
      <div className="mx-auto flex w-full max-w-2xl items-center justify-between gap-2 px-3 py-2">
      <div className="flex items-center gap-2">
        <button
          type="button"
          onClick={onOpenMenu}
          aria-label="Open menu"
          className="grid size-9 place-items-center rounded-xl border border-white/12 bg-white/5 text-white/85 active:scale-95"
        >
          <svg viewBox="0 0 24 24" width="17" height="17" fill="none" stroke="currentColor" strokeWidth="2.5" strokeLinecap="round">
            <line x1="3" y1="6" x2="21" y2="6" />
            <line x1="3" y1="12" x2="21" y2="12" />
            <line x1="3" y1="18" x2="21" y2="18" />
          </svg>
        </button>
        <span className="font-heading text-xs tracking-[0.2em] text-muted">
          GOLDEN <strong className="text-gold-light">POKER</strong>
        </span>
      </div>

      <div className="flex items-center gap-2">
        <Link
          to="/wallet"
          className="rounded-full border border-gold/25 bg-gold/10 px-3 py-1.5 text-xs font-semibold text-gold-light tabular-nums"
        >
          {wallet.toLocaleString()} ETB
        </Link>
        <button
          type="button"
          onClick={onOpenHelp}
          aria-label="What do these tables mean?"
          className="grid size-8 place-items-center rounded-full border border-white/12 bg-white/5 text-sm font-bold text-muted hover:text-gold-light"
        >
          ?
        </button>
      </div>
      </div>
    </header>
  )
}

function ResumeStrip({ roomCode }: { roomCode: string }) {
  return (
    <Link
      to={`/table/${roomCode}`}
      className="flex shrink-0 items-center justify-between gap-2 rounded-lg border border-gold/40 bg-gold/10 px-3 py-1.5 text-[11px] text-gold-light"
    >
      <span>
        Still seated at <strong className="font-mono">#{roomCode}</strong>
      </span>
      <span className="font-semibold">Resume →</span>
    </Link>
  )
}

function TierPicker({
  tiers,
  selected,
  onSelect,
}: {
  tiers: BlindTier[]
  selected: number
  onSelect: (index: number) => void
}) {
  return (
    <div className="flex shrink-0 gap-1.5" role="tablist" aria-label="Stake level">
      {tiers.map((tier, i) => (
        <button
          key={tier.small_blind}
          type="button"
          role="tab"
          aria-selected={i === selected}
          onClick={() => onSelect(i)}
          className={`flex flex-1 items-center justify-center gap-1.5 rounded-lg border py-1.5 transition-colors ${
            i === selected
              ? 'border-gold/60 bg-gold/15 text-gold-light'
              : 'border-white/10 bg-white/5 text-muted'
          }`}
        >
          <TierPiece index={i} size={16} />
          <span className="text-[11px] font-semibold tabular-nums">
            {tier.small_blind}/{tier.big_blind}
          </span>
        </button>
      ))}
    </div>
  )
}

function IconButton({
  label,
  onClick,
  children,
}: {
  label: string
  onClick: () => void
  children: React.ReactNode
}) {
  return (
    <button
      type="button"
      onClick={onClick}
      aria-label={label}
      title={label}
      className="btn-ghost aspect-square px-0"
    >
      <svg viewBox="0 0 24 24" width="18" height="18" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round">
        {children}
      </svg>
    </button>
  )
}

function CreatePrivateForm({
  busy,
  onSubmit,
}: {
  busy: boolean
  onSubmit: (input: { small_blind: number; max_players: number }) => void
}) {
  return (
    <form
      className="flex flex-col gap-3"
      onSubmit={(event) => {
        event.preventDefault()
        const form = new FormData(event.currentTarget)
        onSubmit({
          small_blind: Number(form.get('small_blind')),
          max_players: Number(form.get('max_players')),
        })
      }}
    >
      <p className="text-[13px] leading-relaxed text-muted">
        Your own table with a 5-digit code to share. Everyone who joins brings their whole wallet
        balance, so long as it clears the big blind.
      </p>

      <div className="grid grid-cols-2 gap-3">
        <div>
          <label className="label" htmlFor="small_blind">
            Small blind
          </label>
          <input
            id="small_blind"
            name="small_blind"
            type="number"
            min={10}
            step={5}
            defaultValue={10}
            required
            className="field tabular-nums"
          />
        </div>
        <div>
          <label className="label" htmlFor="max_players">
            Seats
          </label>
          <select id="max_players" name="max_players" defaultValue={6} className="field">
            <option value={2}>2</option>
            <option value={4}>4</option>
            <option value={6}>6</option>
            <option value={9}>9</option>
          </select>
        </div>
      </div>

      <button type="submit" className="btn-gold w-full" disabled={busy}>
        {busy ? 'Creating…' : 'Create table'}
      </button>
    </form>
  )
}

function JoinCodeForm({ busy, onSubmit }: { busy: boolean; onSubmit: (code: string) => void }) {
  return (
    <form
      className="flex flex-col gap-3"
      onSubmit={(event) => {
        event.preventDefault()
        onSubmit(String(new FormData(event.currentTarget).get('code')))
      }}
    >
      <input
        name="code"
        inputMode="numeric"
        maxLength={5}
        pattern="[0-9]{5}"
        required
        autoFocus
        placeholder="00000"
        className="field text-center font-heading text-3xl tracking-[0.35em] tabular-nums"
      />
      <button type="submit" className="btn-primary w-full" disabled={busy}>
        {busy ? 'Looking up…' : 'Join table'}
      </button>
    </form>
  )
}
