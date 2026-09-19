import { useCallback, useEffect, useMemo, useState } from 'react'
import { useNavigate, useParams } from 'react-router-dom'
import { api } from '../lib/api'
import type { PokerAction, TableState } from '../lib/types'
import { emptyTableState } from '../lib/types'
import { useSSE } from '../lib/useSSE'
import type { SSEStatus } from '../lib/useSSE'
import { useCountdown } from '../lib/useCountdown'
import { useActionFeed } from '../lib/useActionFeed'
import { useAuth } from '../lib/auth'
import { useImmersive, exitImmersive } from '../lib/useImmersive'
import { useToast } from '../components/Toast'
import { Spinner } from '../components/Spinner'
import { ConfirmDialog } from '../components/ConfirmDialog'
import { Felt } from '../components/table/Felt'
import { ActionBar } from '../components/table/ActionBar'
import { ActionFeed } from '../components/table/ActionFeed'
import { SitOutPanel } from '../components/table/SitOutPanel'
import { OrientationGate } from '../components/table/OrientationGate'
import { TableMenu } from '../components/table/TableMenu'

export default function Table() {
  const { code = '' } = useParams()
  const { user, refresh, logout } = useAuth()
  const navigate = useNavigate()
  const toast = useToast()
  const immersive = useImmersive()

  const [state, setState] = useState<TableState>(() => emptyTableState(code))
  const [loading, setLoading] = useState(true)
  const [busy, setBusy] = useState(false)
  const [menuOpen, setMenuOpen] = useState(false)
  const [confirmLeave, setConfirmLeave] = useState(false)

  // The game screen owns the whole viewport: no page scroll, no rubber-banding
  // over the felt, and fullscreen released on the way out.
  useEffect(() => {
    const previous = document.body.style.overflow
    document.body.style.overflow = 'hidden'
    return () => {
      document.body.style.overflow = previous
      void exitImmersive()
    }
  }, [])

  // The broadcast payload is not personalised (it hides every hole card), so a
  // broadcast is treated purely as "something changed" and we re-read the
  // per-player view — the same thing the htmx build did with hx-get.
  const load = useCallback(async () => {
    try {
      setState(await api.table.state(code))
    } catch (err) {
      toast(err instanceof Error ? err.message : 'Could not load the table.', 'error')
    } finally {
      setLoading(false)
    }
  }, [code, toast])

  useEffect(() => {
    void load()
  }, [load])

  const sseStatus = useSSE(code, {
    'game-state': useCallback(() => {
      void load()
      void refresh()
    }, [load, refresh]),
  })

  const heroName = user?.username ?? ''
  const hero = useMemo(
    () => state.players?.find((p) => p.name === heroName),
    [state.players, heroName],
  )

  const winningCards = useMemo(() => {
    const revealed = state.intermission || state.game_finished
    if (!revealed) return new Set<string>()
    return new Set(state.winner?.winning_cards ?? [])
  }, [state.intermission, state.game_finished, state.winner])

  const turnActive = state.game_started && !state.intermission
  const turnRemaining = useCountdown(state.turn_ends_at_ms, turnActive)
  const turnFraction = turnRemaining / (state.turn_total_seconds || 25)

  const startsIn = useCountdown(state.countdown_ends_at_ms, state.countdown_active)
  const feed = useActionFeed(state.notifications)

  const run = useCallback(
    async (fn: () => Promise<unknown>, successMessage?: string) => {
      setBusy(true)
      try {
        await fn()
        if (successMessage) toast(successMessage, 'success')
        await Promise.all([load(), refresh()])
      } catch (err) {
        toast(err instanceof Error ? err.message : 'That action failed.', 'error')
      } finally {
        setBusy(false)
      }
    },
    [load, refresh, toast],
  )

  const onAct = useCallback(
    (action: PokerAction, amount = 0) => void run(() => api.table.act(code, action, amount)),
    [run, code],
  )
  const onJoin = useCallback(
    () => void run(() => api.table.join(code), 'Seated. Good luck!'),
    [run, code],
  )
  const onStart = useCallback(() => void run(() => api.table.start(code)), [run, code])
  const onSitIn = useCallback(() => void run(() => api.table.sitIn(code)), [run, code])

  async function leave() {
    setConfirmLeave(false)
    setBusy(true)
    try {
      await api.table.leave(code)
      await refresh()
      navigate('/lobby', { replace: true })
    } catch (err) {
      toast(err instanceof Error ? err.message : 'Could not leave the table.', 'error')
      setBusy(false)
    }
  }

  // A spectator can only take a seat between hands.
  const canSit = !hero && (!state.game_started || state.intermission)

  return (
    <div className="room-backdrop fixed inset-0 overflow-hidden select-none">
      {loading ? (
        <Spinner label="Taking a seat…" />
      ) : (
        <>
          {/* The felt measures itself and fills the viewport — there is no
              fixed aspect to letterbox around, and nothing ever scrolls. */}
          <Felt
            state={state}
            heroName={heroName}
            winningCards={winningCards}
            turnFraction={turnFraction}
            onSit={onJoin}
            canSit={canSit && !busy}
          />

          <TopChrome
            roomCode={code}
            sseStatus={sseStatus}
            startsIn={state.countdown_active ? startsIn : null}
            onOpenMenu={() => setMenuOpen(true)}
          />

          <ActionFeed lines={feed} />

          <ActionBar
            state={state}
            hero={hero}
            busy={busy}
            onAct={onAct}
            onJoin={onJoin}
            onStart={onStart}
          />
        </>
      )}

      <TableMenu
        open={menuOpen}
        onClose={() => setMenuOpen(false)}
        roomCode={code}
        user={user}
        seated={!!hero}
        onLeave={() => {
          setMenuOpen(false)
          setConfirmLeave(true)
        }}
        onLogout={async () => {
          await logout()
          navigate('/login', { replace: true })
        }}
      />

      <ConfirmDialog
        open={confirmLeave}
        message="Leave the table and cash out your remaining chips?"
        confirmLabel="Leave & Cash Out"
        onConfirm={leave}
        onCancel={() => setConfirmLeave(false)}
      />

      {hero?.sitting_out ? <SitOutPanel busy={busy} onSitIn={onSitIn} /> : null}

      {immersive.needsPrompt ? (
        <OrientationGate isPortrait={immersive.isPortrait} onEnter={() => void immersive.enter()} />
      ) : null}
    </div>
  )
}

/** Floating chrome: the menu button, the room code, and the live-status dot. */
function TopChrome({
  roomCode,
  sseStatus,
  startsIn,
  onOpenMenu,
}: {
  roomCode: string
  sseStatus: SSEStatus
  startsIn: number | null
  onOpenMenu: () => void
}) {
  return (
    <>
      <div className="absolute top-2 left-2 z-40 flex flex-col items-start gap-1.5 sm:top-3 sm:left-3">
        <button
          type="button"
          onClick={onOpenMenu}
          aria-label="Open menu"
          className="grid size-10 place-items-center rounded-xl border border-white/15 bg-black/45 text-white/85 backdrop-blur-md transition-colors active:scale-95"
        >
          <svg viewBox="0 0 24 24" width="18" height="18" fill="none" stroke="currentColor" strokeWidth="2.5" strokeLinecap="round">
            <line x1="3" y1="6" x2="21" y2="6" />
            <line x1="3" y1="12" x2="21" y2="12" />
            <line x1="3" y1="18" x2="21" y2="18" />
          </svg>
        </button>
        <span className="rounded-lg bg-black/40 px-1.5 py-0.5 font-mono text-[9px] tracking-[0.15em] text-white/50 backdrop-blur-md">
          #{roomCode}
        </span>
      </div>

      <div className="absolute top-2 right-2 z-40 flex items-center gap-2 sm:top-3 sm:right-3">
        {startsIn !== null ? (
          <span className="rounded-full border border-gold/30 bg-black/45 px-3 py-1 text-[11px] font-semibold text-gold-light backdrop-blur-md">
            Starts in {startsIn}s
          </span>
        ) : null}
        <span
          title={`Live updates: ${sseStatus}`}
          className={`size-2.5 rounded-full ${
            sseStatus === 'live'
              ? 'bg-success'
              : sseStatus === 'connecting'
                ? 'animate-pulse bg-amber-400'
                : 'bg-danger'
          }`}
        />
      </div>
    </>
  )
}
