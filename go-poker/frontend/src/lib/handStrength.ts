/**
 * Turns the engine's hand description into a 0–1 strength, for the meter under
 * the hero's avatar. The descriptions come from the Go solver and always start
 * with the category name ("Two Pair (Kings & Eights)", "Pair of Sevens", …).
 * Longer names are matched first so "Straight Flush" never reads as "Straight".
 */
const RANKS: Array<[prefix: string, rank: number]> = [
  ['Royal Flush', 10],
  ['Straight Flush', 9],
  ['Four of a Kind', 8],
  ['Full House', 7],
  ['Flush', 6],
  ['Straight', 5],
  ['Three of a Kind', 4],
  ['Two Pair', 3],
  ['Pair', 2],
  ['High Card', 1],
]

const MAX_RANK = 10

export function handStrength(handName: string | undefined): number {
  if (!handName) return 0
  for (const [prefix, rank] of RANKS) {
    if (handName.startsWith(prefix)) return rank / MAX_RANK
  }
  return 0
}

/** The category on its own — the parenthetical kickers don't fit on a phone. */
export function handCategory(handName: string | undefined): string {
  if (!handName) return ''
  const open = handName.indexOf('(')
  return (open === -1 ? handName : handName.slice(0, open)).trim()
}
