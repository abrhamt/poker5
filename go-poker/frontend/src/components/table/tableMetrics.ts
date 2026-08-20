/**
 * Every dimension on the felt is derived from one master unit: the height of
 * the table itself. That keeps the proportions between avatars, cards and the
 * rail identical on a 2:1 phone and a 4:3 tablet, and means the whole game
 * fits the viewport without a single media query — or a scrollbar.
 *
 * The ratios are lifted from the reference table art.
 */
import type { Size } from '../../lib/useElementSize'

export interface TableMetrics {
  /** Table box in stage pixels. */
  left: number
  top: number
  width: number
  height: number

  avatar: number
  heroAvatar: number
  card: number
  heroCard: number
  boardCard: number
  boardGap: number

  /** Distances off the rail, along/perpendicular to the inward normal. */
  cardInward: number
  cardLateral: number
  heroCardInward: number
  heroCardLateral: number
  betInward: number
  betLateral: number

  nameSize: number
  chipsSize: number
  tagSize: number
}

/** Ratios of the table's height. */
const R = {
  avatar: 0.21,
  heroAvatar: 0.23,
  card: 0.098,
  heroCard: 0.19,
  boardCard: 0.2,
  boardGap: 0.018,
  cardInward: 0.16,
  cardLateral: 0.13,
  heroCardInward: 0.17,
  heroCardLateral: 0.28,
  betInward: 0.24,
  betLateral: -0.17,
  nameSize: 0.036,
  chipsSize: 0.04,
  tagSize: 0.032,
} as const

/**
 * How far the seat furniture sticks out past the rail, as a ratio of table
 * height. A seat is an avatar with its name plate stacked beneath, centred on
 * the rail, so half of that assembly hangs outside the table — and *that*, not
 * the viewport, is what really caps how tall the table can be.
 *
 *   avatar 0.210 + gap 0.034 + one-line plate 0.060 = 0.304, halved.
 */
const SEAT_OVERHANG = 0.152

/** Breathing room at the edges of the stage. */
const PAD = 0.02

/**
 * Height of the band along the bottom of the screen that the action bar owns,
 * as a ratio of stage height. Seats sit on the rail all the way round, so
 * without reserving this the bottom-corner seats end up underneath the Fold /
 * Call / Raise buttons.
 */
const ACTION_BAND = 0.12

/**
 * Widest the table is allowed to get relative to its height. Landscape phones
 * are far wider than a poker table, so without this the felt stretches into a
 * running track — which is exactly what the old fixed-percentage sizing did,
 * producing a 3.4:1 table on a 844x390 phone.
 */
const MAX_ASPECT = 2.8

/** Narrowest, so a squarish tablet still reads as a table rather than a pond. */
const MIN_ASPECT = 1.7

export function tableMetrics(stage: Size): TableMetrics {
  const padX = stage.width * PAD
  const padY = stage.height * PAD

  // Take the tallest table whose seats still fit the stage vertically, then
  // let width follow within the aspect band. Height drives everything else, so
  // maximising it is what makes the cards and avatars big.
  const band = stage.height * ACTION_BAND
  const usable = stage.height - padY - band
  const byHeight = usable / (1 + 2 * SEAT_OVERHANG)
  const byWidth = (stage.width - 2 * padX) / MIN_ASPECT
  const height = Math.max(1, Math.min(byHeight, byWidth))
  const width = Math.max(height * MIN_ASPECT, Math.min(stage.width * 0.92, height * MAX_ASPECT))

  return {
    left: (stage.width - width) / 2,
    // Centred in what is left once the action band is spoken for.
    top: padY + (usable - height) / 2,
    width,
    height,

    avatar: height * R.avatar,
    heroAvatar: height * R.heroAvatar,
    card: height * R.card,
    heroCard: height * R.heroCard,
    boardCard: height * R.boardCard,
    boardGap: height * R.boardGap,

    cardInward: height * R.cardInward,
    cardLateral: height * R.cardLateral,
    heroCardInward: height * R.heroCardInward,
    heroCardLateral: height * R.heroCardLateral,
    betInward: height * R.betInward,
    betLateral: height * R.betLateral,

    nameSize: height * R.nameSize,
    chipsSize: height * R.chipsSize,
    tagSize: height * R.tagSize,
  }
}
