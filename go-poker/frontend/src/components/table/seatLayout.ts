/**
 * Seats sit on the rail of a stadium-shaped table (a rectangle capped with a
 * semicircle at each end), spaced evenly by arc length starting at the bottom
 * centre and running counter-clockwise — so seat 0 is always the hero and the
 * rest fill out to their left, the way the felt reads on a phone in landscape.
 *
 * Everything here is in table-box pixels, which makes the walk isotropic: an
 * offset of N reads as the same distance whichever seat it is measured from.
 */

export interface SeatAnchor {
  /** Point on the rail, relative to the table box's top-left corner. */
  x: number
  y: number
  /** Unit vector pointing from the rail toward the middle of the felt. */
  inX: number
  inY: number
}

export function seatAnchors(count: number, width: number, height: number): SeatAnchor[] {
  const seats = Math.max(count, 1)
  const r = height / 2
  const straight = Math.max(0, width - height)
  const perimeter = 2 * straight + 2 * Math.PI * r

  return Array.from({ length: seats }, (_, i) => walk((i * perimeter) / seats))

  function walk(s: number): SeatAnchor {
    const halfStraight = straight / 2
    const arc = Math.PI * r

    // 1. bottom edge, centre -> left
    if (s < halfStraight) return { x: width / 2 - s, y: height, inX: 0, inY: -1 }
    s -= halfStraight

    // 2. left cap, bottom -> top
    if (s < arc) return pointOnCap(r, Math.PI / 2 + s / r)
    s -= arc

    // 3. top edge, left -> right
    if (s < straight) return { x: r + s, y: 0, inX: 0, inY: 1 }
    s -= straight

    // 4. right cap, top -> bottom
    if (s < arc) return pointOnCap(width - r, (3 * Math.PI) / 2 + s / r)
    s -= arc

    // 5. bottom edge, right -> centre
    return { x: width - r - s, y: height, inX: 0, inY: -1 }
  }

  function pointOnCap(centreX: number, angle: number): SeatAnchor {
    const dx = Math.cos(angle)
    const dy = Math.sin(angle)
    return { x: centreX + r * dx, y: r + r * dy, inX: -dx, inY: -dy }
  }
}

/**
 * Places a floating element against a seat: `inward` pushes it toward the
 * middle of the felt, `lateral` slides it along the rail. Both in pixels.
 */
export function anchorStyle(
  anchor: SeatAnchor,
  origin: { left: number; top: number },
  inward = 0,
  lateral = 0,
): React.CSSProperties {
  const dx = anchor.inX * inward + anchor.inY * lateral
  const dy = anchor.inY * inward - anchor.inX * lateral

  return {
    left: `${origin.left + anchor.x + dx}px`,
    top: `${origin.top + anchor.y + dy}px`,
    transform: 'translate(-50%, -50%)',
  }
}

/** Rotates a seat roster so `heroName` lands first, keeping table order intact. */
export function rotateToHero<T extends { name: string }>(seats: T[], heroName: string): T[] {
  const heroIndex = seats.findIndex((s) => s.name === heroName)
  if (heroIndex <= 0) return seats
  return [...seats.slice(heroIndex), ...seats.slice(0, heroIndex)]
}
