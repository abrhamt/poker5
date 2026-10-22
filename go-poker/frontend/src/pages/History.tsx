import { useCallback, useEffect, useState } from 'react'
import { api } from '../lib/api'
import type { HistoryHand, HistoryRoom } from '../lib/types'
import { useToast } from '../components/Toast'
import { Spinner } from '../components/Spinner'
import { NetValue, PageShell } from '../components/PageShell'
import { PlayingCard, isFaceUp } from '../components/Card'
import { handCategory } from '../lib/handStrength'

const PAGE_SIZE = 10

/**
 * Every room the player has been dealt into, and under each one the hands
 * they played there — theirs only, newest first.
 */
export default function History() {
  const toast = useToast()
  const [rooms, setRooms] = useState<HistoryRoom[] | null>(null)
  const [open, setOpen] = useState<string | null>(null)

  useEffect(() => {
    api.me
      .historyRooms()
      .then((r) => setRooms(r.rooms ?? []))
      .catch((err) => {
        toast(err instanceof Error ? err.message : 'Could not load your history.', 'error')
        setRooms([])
      })
  }, [toast])

  return (
    <PageShell title="Game history" path="/history">
      {rooms === null ? (
        <Spinner label="Loading your history…" />
      ) : rooms.length === 0 ? (
        <section className="card px-4 py-8 text-center">
          <p className="font-heading text-lg font-semibold">No hands yet</p>
          <p className="mt-1 text-sm text-muted">Every hand you're dealt into shows up here, grouped by table.</p>
        </section>
      ) : (
        rooms.map((room) => (
          <RoomSection
            key={room.room_code}
            room={room}
            open={open === room.room_code}
            onToggle={() => setOpen((o) => (o === room.room_code ? null : room.room_code))}
          />
        ))
      )}
    </PageShell>
  )
}

function RoomSection({ room, open, onToggle }: { room: HistoryRoom; open: boolean; onToggle: () => void }) {
  const toast = useToast()
  const [hands, setHands] = useState<HistoryHand[]>([])
  const [page, setPage] = useState(0)
  const [totalPages, setTotalPages] = useState(1)
  const [loading, setLoading] = useState(false)
  const [started, setStarted] = useState(false)

  const loadPage = useCallback(
    async (next: number) => {
      setLoading(true)
      try {
        const res = await api.me.historyHands(room.room_code, next, PAGE_SIZE)
        setHands((h) => (next === 1 ? res.hands : [...h, ...res.hands]))
        setPage(res.page)
        setTotalPages(res.total_pages)
      } catch (err) {
        toast(err instanceof Error ? err.message : 'Could not load these hands.', 'error')
      } finally {
        setLoading(false)
      }
    },
    [room.room_code, toast],
  )

  // Hands load the first time the room is opened, and only then.
  useEffect(() => {
    if (!open || started) return
    setStarted(true)
    void loadPage(1)
  }, [open, started, loadPage])

  const name = room.room_name || (room.room_type === 'private' ? 'Private table' : 'Table')

  return (
    <section className="card overflow-hidden">
      <button
        type="button"
        onClick={onToggle}
        aria-expanded={open}
        className="flex w-full items-center justify-between gap-3 px-4 py-3 text-left"
      >
        <div className="min-w-0">
          <p className="truncate font-heading font-semibold">{name}</p>
          <p className="mt-0.5 text-xs text-muted">
            #{room.room_code}
            {room.big_blind ? ` · ${room.small_blind}/${room.big_blind} blinds` : ''} · last played{' '}
            {formatWhen(room.last_played)}
          </p>
        </div>
        <div className="flex shrink-0 items-center gap-3">
          <div className="text-right">
            <NetValue net={room.net} className="font-heading text-lg font-bold" />
            <p className="text-[11px] text-muted">
              {room.hands.toLocaleString()} hand{room.hands === 1 ? '' : 's'} · {room.hands_won} won
            </p>
          </div>
          <svg
            viewBox="0 0 24 24"
            width="16"
            height="16"
            className={`text-muted transition-transform ${open ? 'rotate-180' : ''}`}
            fill="none"
            stroke="currentColor"
            strokeWidth="2.5"
            strokeLinecap="round"
            strokeLinejoin="round"
          >
            <path d="m6 9 6 6 6-6" />
          </svg>
        </div>
      </button>

      {open ? (
        <div className="border-t border-white/8">
          {hands.map((hand) => (
            <HandRow key={hand.id} hand={hand} />
          ))}
          {loading ? (
            <p className="px-4 py-3 text-center text-xs text-muted">Loading…</p>
          ) : page > 0 && page < totalPages ? (
            <button
              type="button"
              onClick={() => void loadPage(page + 1)}
              className="w-full px-4 py-3 text-center text-sm font-semibold text-gold-light hover:bg-white/5"
            >
              Show more hands
            </button>
          ) : null}
        </div>
      ) : null}
    </section>
  )
}

function HandRow({ hand }: { hand: HistoryHand }) {
  const [expanded, setExpanded] = useState(false)
  const you = hand.you

  const outcome = you.folded
    ? 'Folded'
    : you.net > 0
      ? `Won with ${handCategory(you.hand_name) || 'the pot'}`
      : you.showed_down
        ? `Lost · ${hand.winner_name} won with ${handCategory(hand.winning_hand) || 'a better hand'}`
        : `Lost to ${hand.winner_name}`

  return (
    <div className="border-b border-white/6 last:border-b-0">
      <button
        type="button"
        onClick={() => setExpanded((e) => !e)}
        aria-expanded={expanded}
        className="flex w-full items-center gap-3 px-4 py-2.5 text-left hover:bg-white/3"
      >
        <div className="flex shrink-0 gap-0.5">
          {you.cards.map((c, i) => (
            <PlayingCard key={i} code={isFaceUp(c) ? c : '1B'} width={24} />
          ))}
        </div>
        <div className="min-w-0 flex-1">
          <p className="truncate text-sm">{outcome}</p>
          <p className="text-[11px] text-muted">
            Hand #{hand.hand_number} · {formatWhen(hand.played_at)} · pot {hand.pot.toLocaleString()}
          </p>
        </div>
        <NetValue net={you.net} className="shrink-0 font-semibold tabular-nums" />
      </button>

      {expanded ? (
        <div className="flex flex-col gap-2.5 bg-black/20 px-4 py-3">
          <div>
            <p className="mb-1 text-[10px] tracking-wide text-muted uppercase">Board</p>
            {hand.board.length ? (
              <div className="flex gap-1">
                {hand.board.map((c) => (
                  <PlayingCard key={c} code={c} width={30} />
                ))}
              </div>
            ) : (
              <p className="text-xs text-muted">Over before the flop.</p>
            )}
          </div>

          <div>
            <p className="mb-1 text-[10px] tracking-wide text-muted uppercase">Players</p>
            <ul className="flex flex-col gap-1">
              <SeatLine name={`${you.name} (you)`} cards={you.cards} handName={you.hand_name} folded={you.folded} net={you.net} />
              {hand.players.map((p) => (
                <SeatLine
                  key={p.name}
                  name={p.name}
                  cards={p.cards}
                  handName={p.hand_name}
                  folded={p.folded}
                  net={p.net}
                />
              ))}
            </ul>
          </div>

          <p className="text-[11px] text-muted">
            You put in {you.invested.toLocaleString()}
            {you.won ? `, took ${you.won.toLocaleString()}` : ''}. Rake {hand.rake.toLocaleString()} · blinds{' '}
            {hand.small_blind}/{hand.big_blind}.
          </p>
        </div>
      ) : null}
    </div>
  )
}

function SeatLine({
  name,
  cards,
  handName,
  folded,
  net,
}: {
  name: string
  cards?: string[]
  handName?: string
  folded: boolean
  net: number
}) {
  return (
    <li className="flex items-center gap-2 text-xs">
      <span className="flex w-[50px] shrink-0 gap-0.5">
        {cards?.length
          ? cards.map((c, i) => <PlayingCard key={i} code={c} width={22} />)
          : [0, 1].map((i) => <PlayingCard key={i} code="1B" width={22} />)}
      </span>
      <span className="min-w-0 flex-1 truncate">
        <span className="font-medium text-chalk">{name}</span>
        <span className="text-muted"> · {folded ? 'folded' : handCategory(handName) || 'mucked'}</span>
      </span>
      <NetValue net={net} className="shrink-0 tabular-nums" />
    </li>
  )
}

function formatWhen(iso: string): string {
  const then = new Date(iso)
  const seconds = (Date.now() - then.getTime()) / 1000
  if (seconds < 60) return 'just now'
  if (seconds < 3600) return `${Math.floor(seconds / 60)} min ago`
  if (seconds < 86400) return `${Math.floor(seconds / 3600)} h ago`
  return then.toLocaleDateString(undefined, { day: 'numeric', month: 'short' })
}
