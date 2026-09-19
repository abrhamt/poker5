import type { SeatPlayer } from '../../lib/types'
import type { SeatAnchor } from './seatLayout'
import { anchorStyle } from './seatLayout'
import type { TableMetrics } from './tableMetrics'
import { PlayingCard, isFaceUp } from '../Card'
import { handCategory, handStrength } from '../../lib/handStrength'

interface SeatProps {
  player: SeatPlayer
  anchor: SeatAnchor
  metrics: TableMetrics
  isHero: boolean
  isTurn: boolean
  /** 0–1 of the turn clock still left, for the ring around the avatar. */
  turnFraction: number
  revealed: boolean
  isWinner: boolean
  winningCards: Set<string>
}

export function Seat({
  player,
  anchor,
  metrics,
  isHero,
  isTurn,
  turnFraction,
  revealed,
  isWinner,
  winningCards,
}: SeatProps) {
  const showCards = (isHero || revealed) && !player.folded && isFaceUp(player.cards?.[0])
  const cardWidth = isHero ? metrics.heroCard : metrics.card
  const avatarSize = isHero ? metrics.heroAvatar : metrics.avatar
  // Seats on the far side of the table read top-down, so their plate belongs
  // above the avatar rather than below it, hanging off the felt either way.
  const plateAbove = anchor.inY > 0

  return (
    <>
      {/* Hole cards lie on the felt beside the avatar, hero's included — no
          part of anyone's hand lives outside the table. */}
      {!player.folded ? (
        <div
          className="absolute z-10 flex"
          style={{
            ...anchorStyle(
              anchor,
              metrics,
              isHero ? metrics.heroCardInward : metrics.cardInward,
              isHero ? metrics.heroCardLateral : metrics.cardLateral,
            ),
            gap: `${cardWidth * 0.1}px`,
          }}
        >
          {player.cards?.map((code, i) => (
            <PlayingCard
              key={i}
              code={showCards ? code : '1B'}
              width={cardWidth}
              winning={showCards && winningCards.has(code)}
            />
          ))}
        </div>
      ) : null}

      {player.round_bet > 0 ? (
        <div
          className="absolute z-20 flex items-center gap-1 rounded-full border border-gold/40 bg-black/75 font-bold text-gold-light"
          style={{
            ...anchorStyle(anchor, metrics, metrics.betInward, metrics.betLateral),
            fontSize: `${metrics.tagSize}px`,
            padding: `${metrics.tagSize * 0.25}px ${metrics.tagSize * 0.6}px`,
          }}
        >
          <span
            className="rounded-full bg-gold"
            style={{ width: `${metrics.tagSize * 0.5}px`, height: `${metrics.tagSize * 0.5}px` }}
          />
          {player.round_bet.toLocaleString()}
        </div>
      ) : null}

      <PositionBadge
        player={player}
        anchor={anchor}
        metrics={metrics}
        avatarSize={avatarSize}
      />

      <div
        className={`absolute z-30 flex flex-col items-center ${player.folded ? 'opacity-45' : ''} ${
          plateAbove ? 'flex-col-reverse' : ''
        }`}
        style={anchorStyle(anchor, metrics)}
      >
        <Avatar
          player={player}
          size={avatarSize}
          isTurn={isTurn}
          isWinner={isWinner}
          turnFraction={turnFraction}
        />

        {/* Name and stack share one pill. Two stacked pills read fine on a
            desktop, but the outer half of this plate is what caps the table's
            height (see SEAT_OVERHANG) — collapsing it to a single line is what
            lets the felt, and so every card on it, grow.

            It sits on whichever side of the avatar faces away from the felt, so
            it never covers cards, the rail, or the turn ring. */}
        <div
          className="relative flex flex-col items-center"
          style={{ [plateAbove ? 'marginBottom' : 'marginTop']: `${avatarSize * 0.16}px` }}
        >
          <span
            className="flex max-w-full items-baseline rounded-full bg-black/75"
            style={{
              gap: `${metrics.nameSize * 0.45}px`,
              padding: `${metrics.nameSize * 0.16}px ${metrics.nameSize * 0.6}px`,
            }}
          >
            <span
              className="truncate font-semibold text-white/85"
              style={{ fontSize: `${metrics.nameSize}px`, maxWidth: `${avatarSize * 1.5}px` }}
            >
              {isHero ? 'You' : player.name}
            </span>
            <span
              className="font-bold text-gold-light tabular-nums"
              style={{ fontSize: `${metrics.chipsSize}px` }}
            >
              {player.chips.toLocaleString()}
            </span>
          </span>

          {/* Absolutely placed so a showdown tag never grows the plate — the
              table height is computed from the plate's resting size. */}
          <div
            className={`absolute inset-x-0 flex justify-center ${plateAbove ? 'bottom-full' : 'top-full'}`}
            style={{ [plateAbove ? 'marginBottom' : 'marginTop']: `${metrics.tagSize * 0.3}px` }}
          >
            <SeatTag
              player={player}
              metrics={metrics}
              revealed={revealed}
              isWinner={isWinner}
              isHero={isHero}
            />
          </div>
        </div>
      </div>
    </>
  )
}

function Avatar({
  player,
  size,
  isTurn,
  isWinner,
  turnFraction,
}: {
  player: SeatPlayer
  size: number
  isTurn: boolean
  isWinner: boolean
  turnFraction: number
}) {
  const ringColour = isWinner
    ? 'var(--color-gold)'
    : isTurn
      ? 'var(--color-gold-bright)'
      : 'rgb(0 0 0 / 0.5)'

  return (
    <div className="relative" style={{ width: `${size}px`, height: `${size}px` }}>
      {isTurn ? <TurnRing fraction={turnFraction} /> : null}

      <div
        className="grid size-full place-items-center rounded-full bg-linear-160 from-[#33513f] to-[#111f19] font-heading font-bold text-gold-light uppercase"
        style={{
          fontSize: `${size * 0.42}px`,
          boxShadow: `0 0 0 ${Math.max(2, size * 0.045)}px ${ringColour}, 0 3px 10px rgb(0 0 0 / 0.55)`,
        }}
      >
        {player.name.charAt(0)}
      </div>
    </div>
  )
}

/** Countdown ring drawn around the acting player's avatar. */
function TurnRing({ fraction }: { fraction: number }) {
  const radius = 45
  const circumference = 2 * Math.PI * radius

  return (
    <svg
      viewBox="0 0 100 100"
      className="pointer-events-none absolute -inset-[14%] size-[128%] -rotate-90"
    >
      <circle cx="50" cy="50" r={radius} fill="none" stroke="rgb(0 0 0 / 0.5)" strokeWidth="7" />
      <circle
        cx="50"
        cy="50"
        r={radius}
        fill="none"
        stroke={fraction < 0.25 ? 'var(--color-danger)' : 'var(--color-gold-bright)'}
        strokeWidth="7"
        strokeLinecap="round"
        strokeDasharray={circumference}
        strokeDashoffset={circumference * (1 - Math.max(0, Math.min(1, fraction)))}
        className="transition-[stroke-dashoffset] duration-300 ease-linear"
      />
    </svg>
  )
}

/** Dealer button / blind marker, parked on the opposite side of the avatar
 *  from the hole cards so the two never stack. */
function PositionBadge({
  player,
  anchor,
  metrics,
  avatarSize,
}: {
  player: SeatPlayer
  anchor: SeatAnchor
  metrics: TableMetrics
  avatarSize: number
}) {
  const badge = player.dealer
    ? { text: 'D', tone: 'bg-white text-ink' }
    : player.small_blind
      ? { text: 'SB', tone: 'bg-brand text-white' }
      : player.big_blind
        ? { text: 'BB', tone: 'bg-gold text-ink' }
        : null

  if (!badge) return null

  const badgeSize = avatarSize * 0.38

  return (
    <span
      className={`absolute z-40 grid place-items-center rounded-full font-black shadow-[0_1px_4px_rgb(0_0_0/0.7)] ${badge.tone}`}
      style={{
        ...anchorStyle(anchor, metrics, avatarSize * 0.52, -avatarSize * 0.58),
        minWidth: `${badgeSize}px`,
        height: `${badgeSize}px`,
        fontSize: `${badgeSize * 0.5}px`,
        paddingInline: `${badgeSize * 0.15}px`,
      }}
    >
      {badge.text}
    </span>
  )
}

function SeatTag({
  player,
  metrics,
  revealed,
  isWinner,
  isHero,
}: {
  player: SeatPlayer
  metrics: TableMetrics
  revealed: boolean
  isWinner: boolean
  isHero: boolean
}) {
  const pill = {
    fontSize: `${metrics.tagSize}px`,
    padding: `${metrics.tagSize * 0.12}px ${metrics.tagSize * 0.5}px`,
  }

  // Sitting out reads as folded to the engine, so check it first — "Away" is
  // the honest label for a seat that is not folding, just absent.
  if (player.sitting_out) {
    return (
      <span className="rounded-full bg-black/70 font-semibold text-amber-300/90" style={pill}>
        Away
      </span>
    )
  }

  if (player.folded) {
    return (
      <span className="rounded-full bg-black/70 text-white/50" style={pill}>
        Folded
      </span>
    )
  }

  if (player.all_in) {
    return (
      <span className="rounded-full bg-danger/85 font-bold text-white" style={pill}>
        ALL IN
      </span>
    )
  }

  if (isWinner) {
    return (
      <span className="animate-rise rounded-full bg-gold font-bold text-ink" style={pill}>
        {handCategory(player.hand_name) || 'Winner'}
      </span>
    )
  }

  // The hero always sees their own hand read out with a strength meter; other
  // seats only get one once the hand is revealed at showdown.
  if (isHero && player.hand_name) return <HandMeter handName={player.hand_name} metrics={metrics} />
  if (revealed && player.hand_name) {
    return (
      <span className="rounded-full bg-black/70 text-white/70" style={pill}>
        {handCategory(player.hand_name)}
      </span>
    )
  }

  return null
}

function HandMeter({ handName, metrics }: { handName: string; metrics: TableMetrics }) {
  return (
    <span
      className="flex flex-col items-center rounded-md bg-black/75"
      style={{
        gap: `${metrics.tagSize * 0.25}px`,
        padding: `${metrics.tagSize * 0.25}px ${metrics.tagSize * 0.6}px`,
      }}
    >
      <span className="font-semibold text-white/80" style={{ fontSize: `${metrics.tagSize}px` }}>
        {handCategory(handName)}
      </span>
      <span
        className="overflow-hidden rounded-full bg-white/15"
        style={{ height: `${metrics.tagSize * 0.32}px`, width: `${metrics.tagSize * 6}px` }}
      >
        <span
          className="block h-full rounded-full bg-linear-to-r from-gold-dark to-gold-bright transition-[width] duration-500"
          style={{ width: `${handStrength(handName) * 100}%` }}
        />
      </span>
    </span>
  )
}

/** An open seat on the rail — tapping it is how a spectator sits down. */
export function EmptySeat({
  anchor,
  metrics,
  onSit,
  disabled,
}: {
  anchor: SeatAnchor
  metrics: TableMetrics
  onSit?: () => void
  disabled?: boolean
}) {
  const interactive = !!onSit && !disabled

  return (
    <button
      type="button"
      disabled={!interactive}
      onClick={onSit}
      style={{
        ...anchorStyle(anchor, metrics),
        width: `${metrics.avatar}px`,
        height: `${metrics.avatar}px`,
      }}
      className={`absolute z-30 grid place-items-center rounded-full border border-white/20 bg-black/45 text-white/50 shadow-[0_0_0_3px_rgb(0_0_0/0.45)] transition-colors ${
        interactive ? 'cursor-pointer hover:border-gold/60 hover:bg-gold/20 hover:text-gold' : ''
      }`}
      aria-label={interactive ? 'Sit down at this seat' : 'Empty seat'}
    >
      <svg
        viewBox="0 0 24 24"
        style={{ width: `${metrics.avatar * 0.45}px` }}
        fill="none"
        stroke="currentColor"
        strokeWidth="2"
        strokeLinecap="round"
        strokeLinejoin="round"
      >
        <path d="M16 21v-2a4 4 0 0 0-4-4H6a4 4 0 0 0-4 4v2" />
        <circle cx="9" cy="7" r="4" />
        <line x1="19" y1="8" x2="19" y2="14" />
        <line x1="22" y1="11" x2="16" y2="11" />
      </svg>
    </button>
  )
}
