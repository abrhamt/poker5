/**
 * Which cards actually make a hand — the pair, the two pair, the straight —
 * so the table can lift exactly those off the felt. The server only sends the
 * winner's five cards at showdown; during play the hero's best five are worked
 * out here from their hole cards and the board.
 *
 * Card codes are the deck's canonical two characters: rank then suit ("TH").
 */
import { normalizeCardCode } from '../components/Card'

const RANK_ORDER = '23456789TJQKA'

interface Parsed {
  code: string
  rank: number
  suit: string
}

interface Evaluated {
  /** 1 = high card … 9 = straight flush. */
  category: number
  /** Rank values compared in order to break ties within a category. */
  tiebreak: number[]
  /** The cards that form the category itself; kickers are left out. */
  core: string[]
}

function parse(code: string): Parsed | null {
  const c = normalizeCardCode(code)
  const rank = RANK_ORDER.indexOf(c[0]) + 2
  if (rank < 2 || c.length < 2) return null
  return { code: c, rank, suit: c[1] }
}

function evaluateFive(cards: Parsed[]): Evaluated {
  const byRank = new Map<number, Parsed[]>()
  for (const c of cards) byRank.set(c.rank, [...(byRank.get(c.rank) ?? []), c])
  // Bigger groups first, then higher ranks: [[K,K,K],[8,8]] for a full house.
  const groups = [...byRank.values()].sort((a, b) => b.length - a.length || b[0].rank - a[0].rank)

  const flush = cards.every((c) => c.suit === cards[0].suit)
  const ranks = [...new Set(cards.map((c) => c.rank))].sort((a, b) => b - a)
  let straightHigh = 0
  if (ranks.length === 5) {
    if (ranks[0] - ranks[4] === 4) straightHigh = ranks[0]
    else if (ranks.join() === '14,5,4,3,2') straightHigh = 5 // the wheel, A-2-3-4-5
  }

  const all = cards.map((c) => c.code)
  const groupRanks = groups.map((g) => g[0].rank)
  const codesOf = (n: number) => groups.slice(0, n).flat().map((c) => c.code)

  if (straightHigh && flush) return { category: 9, tiebreak: [straightHigh], core: all }
  if (groups[0].length === 4) return { category: 8, tiebreak: groupRanks, core: codesOf(1) }
  if (groups[0].length === 3 && groups[1]?.length === 2) return { category: 7, tiebreak: groupRanks, core: all }
  if (flush) return { category: 6, tiebreak: ranks, core: all }
  if (straightHigh) return { category: 5, tiebreak: [straightHigh], core: all }
  if (groups[0].length === 3) return { category: 4, tiebreak: groupRanks, core: codesOf(1) }
  if (groups[0].length === 2 && groups[1]?.length === 2) return { category: 3, tiebreak: groupRanks, core: codesOf(2) }
  if (groups[0].length === 2) return { category: 2, tiebreak: groupRanks, core: codesOf(1) }
  return { category: 1, tiebreak: ranks, core: [] }
}

function compare(a: Evaluated, b: Evaluated): number {
  if (a.category !== b.category) return a.category - b.category
  for (let i = 0; i < Math.max(a.tiebreak.length, b.tiebreak.length); i++) {
    const d = (a.tiebreak[i] ?? 0) - (b.tiebreak[i] ?? 0)
    if (d !== 0) return d
  }
  return 0
}

function combinations<T>(items: T[], k: number): T[][] {
  if (k === 0) return [[]]
  if (items.length < k) return []
  const [head, ...rest] = items
  return [...combinations(rest, k - 1).map((c) => [head, ...c]), ...combinations(rest, k)]
}

/**
 * The cards forming the best hand that can be made from `codes` (any mix of
 * hole and board cards). Empty for a bare high card, where nothing is "made",
 * and for fewer than two cards.
 */
export function madeHandCards(codes: (string | undefined)[]): Set<string> {
  const cards = codes.map((c) => (c && c !== '1B' ? parse(c) : null)).filter((c): c is Parsed => !!c)
  if (cards.length < 2) return new Set()

  // Preflop there are only two cards: the one thing worth showing is a pair.
  if (cards.length < 5) {
    const paired = cards.filter((c) => cards.some((o) => o !== c && o.rank === c.rank))
    return new Set(paired.map((c) => c.code))
  }

  let best: Evaluated | null = null
  for (const five of combinations(cards, 5)) {
    const e = evaluateFive(five)
    if (!best || compare(e, best) > 0) best = e
  }
  return new Set(best?.core ?? [])
}
