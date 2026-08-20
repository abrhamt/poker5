import { useEffect, useMemo, useState } from 'react'
import type { PokerAction, SeatPlayer, TableState } from '../../lib/types'

interface ActionBarProps {
  state: TableState
  hero: SeatPlayer | undefined
  busy: boolean
  onAct: (action: PokerAction, amount?: number) => void
  onJoin: () => void
  onStart: () => void
}

/**
 * The only permanent chrome over the felt: three thumb-sized buttons pinned to
 * the bottom-right. Raising expands a panel upward rather than opening a
 * dialog, so the table never scrolls and never gets covered.
 */
export function ActionBar(props: ActionBarProps) {
  const { hero } = props

  return (
    <>
      {/* Status sits with the rest of the floating chrome on the left, below
          the menu button: the top centre and both bottom corners are seats. */}
      <div className="pointer-events-none absolute top-20 left-2 z-40 sm:top-22 sm:left-3">
        <WaitingNotice {...props} />
      </div>

      <div className="pointer-events-none absolute inset-x-2 bottom-2 z-40 flex items-end justify-end sm:inset-x-4 sm:bottom-3">
        <div className="pointer-events-auto">
          {hero ? <PlayerActions {...props} hero={hero} /> : <SitDown {...props} />}
        </div>
      </div>
    </>
  )
}

function PlayerActions({ state, hero, busy, onAct }: ActionBarProps & { hero: SeatPlayer }) {
  const toCall = Math.max(0, state.current_bet - hero.round_bet)
  const allInTo = hero.round_bet + hero.chips
  // The engine's minimum re-raise is what it costs to call plus the size of the
  // last raise; the panel speaks in "raise to" totals the way players think.
  const minRaiseTo = Math.min(allInTo, state.current_bet + Math.max(state.last_raise, state.big_blind))
  const canRaise = hero.chips > toCall && allInTo > state.current_bet
  const inHand = state.game_started && !state.intermission && !hero.folded && !hero.all_in
  const myTurn = inHand && state.is_my_turn === true
  const locked = busy || !myTurn

  const [raiseOpen, setRaiseOpen] = useState(false)
  const [raiseTo, setRaiseTo] = useState(minRaiseTo)

  // A new betting decision resets the panel — both the amount and whether it
  // is even open, so a stale slider can never be confirmed on the next street.
  useEffect(() => {
    setRaiseTo(minRaiseTo)
    if (!myTurn) setRaiseOpen(false)
  }, [minRaiseTo, myTurn, state.version])

  const presets = useMemo(
    () => [
      { label: 'Min', to: minRaiseTo },
      { label: '½ Pot', to: state.current_bet + Math.floor(state.pot / 2) },
      { label: 'Pot', to: state.current_bet + state.pot },
      { label: 'All In', to: allInTo },
    ],
    [minRaiseTo, allInTo, state.current_bet, state.pot],
  )

  if (!inHand) return null

  const clamp = (value: number) => Math.max(minRaiseTo, Math.min(allInTo, value))
  const amount = clamp(raiseTo)
  const isAllIn = amount >= allInTo

  return (
    <div className="flex flex-col items-end gap-2">
      {raiseOpen && canRaise ? (
        <div className="animate-slide-up w-[min(78vw,20rem)] rounded-2xl border border-gold/25 bg-ink/92 p-2.5 shadow-deep backdrop-blur-xl">
          <div className="flex items-baseline justify-between">
            <span className="text-[11px] tracking-wide text-muted uppercase">Raise to</span>
            <span className="font-heading text-lg font-bold text-gold-light">
              {amount.toLocaleString()}
            </span>
          </div>

          <input
            type="range"
            min={minRaiseTo}
            max={allInTo}
            step={Math.max(1, state.small_blind)}
            value={amount}
            disabled={locked}
            onChange={(e) => setRaiseTo(clamp(Number(e.target.value)))}
            className="mt-2 h-2 w-full cursor-pointer appearance-none rounded-full bg-white/12 accent-gold"
          />

          <div className="mt-2.5 grid grid-cols-4 gap-1.5">
            {presets.map((preset) => (
              <button
                key={preset.label}
                type="button"
                disabled={locked}
                onClick={() => setRaiseTo(clamp(preset.to))}
                className={`rounded-lg border px-1 py-1.5 text-[11px] font-semibold transition-colors ${
                  amount === clamp(preset.to)
                    ? 'border-gold/60 bg-gold/20 text-gold-light'
                    : 'border-white/10 bg-white/5 text-muted'
                }`}
              >
                {preset.label}
              </button>
            ))}
          </div>

          <div className="mt-2.5 flex gap-2">
            <button
              type="button"
              className="btn-ghost flex-1 py-2 text-xs"
              onClick={() => setRaiseOpen(false)}
            >
              Cancel
            </button>
            <button
              type="button"
              disabled={locked}
              // The engine takes chips-to-commit, not a target — convert here.
              onClick={() => {
                setRaiseOpen(false)
                onAct('raise', amount - hero.round_bet)
              }}
              className="btn-gold flex-2 py-2 text-xs"
            >
              {isAllIn ? 'All In' : `Raise ${amount.toLocaleString()}`}
            </button>
          </div>
        </div>
      ) : null}

      <div className="flex items-end gap-2">
        <ActionButton
          label="Fold"
          tone="fold"
          disabled={locked}
          onClick={() => onAct('fold')}
          icon={
            <>
              <line x1="18" y1="6" x2="6" y2="18" />
              <line x1="6" y1="6" x2="18" y2="18" />
            </>
          }
        />

        <ActionButton
          label={toCall > 0 ? `Call ${toCall.toLocaleString()}` : 'Check'}
          tone="primary"
          disabled={locked}
          onClick={() => onAct(toCall > 0 ? 'call' : 'check', toCall)}
          icon={<polyline points="20 6 9 17 4 12" />}
        />

        <ActionButton
          label={canRaise ? (toCall > 0 ? 'Raise' : 'Bet') : 'All In'}
          tone="raise"
          disabled={locked || (!canRaise && hero.chips <= 0)}
          onClick={() => (canRaise ? setRaiseOpen((open) => !open) : onAct('raise', hero.chips))}
          icon={
            <>
              <line x1="12" y1="19" x2="12" y2="5" />
              <polyline points="5 12 12 5 19 12" />
            </>
          }
        />
      </div>
    </div>
  )
}

const TONES = {
  fold: 'border-white/15 bg-black/55 text-white/75',
  primary: 'border-emerald-300/50 bg-emerald-500/25 text-emerald-100',
  raise: 'border-gold/50 bg-gold/25 text-gold-light',
} as const

function ActionButton({
  label,
  tone,
  icon,
  disabled,
  onClick,
}: {
  label: string
  tone: keyof typeof TONES
  icon: React.ReactNode
  disabled: boolean
  onClick: () => void
}) {
  return (
    <button
      type="button"
      disabled={disabled}
      onClick={onClick}
      className={`flex min-w-20 flex-col items-center gap-1 rounded-2xl border px-3 py-2 font-heading text-xs font-bold tracking-wide backdrop-blur-md transition-all active:scale-95 disabled:opacity-40 ${TONES[tone]}`}
    >
      <svg viewBox="0 0 24 24" width="16" height="16" fill="none" stroke="currentColor" strokeWidth="2.5" strokeLinecap="round" strokeLinejoin="round">
        {icon}
      </svg>
      <span className="whitespace-nowrap">{label}</span>
    </button>
  )
}

function SitDown({ state, busy, onJoin }: ActionBarProps) {
  if (state.game_started && !state.intermission) {
    return (
      <span className="rounded-2xl border border-white/12 bg-black/55 px-3.5 py-2.5 text-xs text-white/70 backdrop-blur-md">
        Hand in progress — you can sit down when it finishes
      </span>
    )
  }

  return (
    <button type="button" onClick={onJoin} disabled={busy} className="btn-gold px-5 py-3 text-sm">
      {state.room_type === 'private'
        ? 'Sit Down · Wallet Balance'
        : `Sit Down · ${state.buy_in.toLocaleString()} ETB`}
    </button>
  )
}

/** Left-hand status line: what the table is waiting on, and the host's early start. */
function WaitingNotice({ state, hero, busy, onStart }: ActionBarProps) {
  const seated = state.players?.length ?? 0
  const canStart = state.room_type === 'private' && state.is_host && seated >= 2 && !state.game_started

  let message = ''
  if (state.intermission) message = ''
  else if (!state.game_started) message = seated < 2 ? 'Waiting for players…' : 'Starting shortly…'
  else if (hero?.folded) message = 'Folded — waiting for the next deal'
  else if (hero?.all_in) message = 'All in'
  else if (hero && !state.is_my_turn) message = `${state.current_turn_player ?? 'Opponent'} to act`

  if (!message && !canStart) return null

  return (
    <div className="flex items-center gap-2">
      {message ? (
        <span className="rounded-full border border-white/10 bg-black/55 px-3 py-1.5 text-[11px] text-white/70 backdrop-blur-md">
          {message}
        </span>
      ) : null}
      {canStart ? (
        <button type="button" onClick={onStart} disabled={busy} className="btn-gold px-3 py-1.5 text-[11px]">
          Start Now
        </button>
      ) : null}
    </div>
  )
}
