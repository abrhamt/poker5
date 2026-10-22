/**
 * Procedural models for the 3D table: the table itself, chip stacks, and the
 * dealer button. Everything is built from primitives and canvas-drawn
 * textures — no model or image files — so it scales to any stage size and
 * costs nothing to download beyond three.js itself.
 *
 * Units are stage pixels. The scene is drawn by an orthographic camera tilted
 * TILT away from straight-down, so a point on the table plane at (x, z) lands
 * on screen at (x, z·cos TILT). Every outline here is the on-screen stadium
 * the DOM seats are laid out against, stretched by 1/cos TILT in depth, which
 * is what keeps the 3D rail exactly under the seats.
 */
import * as THREE from 'three'

export const TILT = (28 * Math.PI) / 180

/** Heights above the table plane (y = 0 is the top of the padded rail). */
export function tableHeights(H: number) {
  return { felt: -0.02 * H }
}

export interface TableModelSpec {
  /** On-screen size of the rail's outer edge, in stage pixels. */
  width: number
  height: number
}

// ---------------------------------------------------------------------------
// Shared helpers

/** Deterministic PRNG so the felt grain is identical on every render. */
function mulberry32(seed: number) {
  return () => {
    seed |= 0
    seed = (seed + 0x6d2b79f5) | 0
    let t = Math.imul(seed ^ (seed >>> 15), 1 | seed)
    t = (t + Math.imul(t ^ (t >>> 7), 61 | t)) ^ t
    return ((t ^ (t >>> 14)) >>> 0) / 4294967296
  }
}

/**
 * Traces the screen stadium (W×H, caps of radius H/2), shrunk by `inset`, as
 * a shape. Shape y is screen-up (−s) so that after layFlat() the shape's +y
 * runs away from the camera and its face normal points up.
 */
function stadiumPath<T extends THREE.Path>(path: T, halfW: number, halfH: number, inset: number): T {
  const r = Math.max(1, halfH - inset)
  const cx = Math.max(0, halfW - halfH)
  path.moveTo(-cx, -r)
  path.lineTo(cx, -r)
  path.absarc(cx, 0, r, -Math.PI / 2, Math.PI / 2, false)
  path.lineTo(-cx, r)
  path.absarc(-cx, 0, r, Math.PI / 2, (3 * Math.PI) / 2, false)
  return path
}

function stadiumShape(halfW: number, halfH: number, inset: number, holeInset?: number) {
  const shape = stadiumPath(new THREE.Shape(), halfW, halfH, inset)
  if (holeInset !== undefined) shape.holes.push(stadiumPath(new THREE.Path(), halfW, halfH, holeInset))
  return shape
}

/** Lays a shape-space geometry onto the table plane, stretched for the tilt. */
function layFlat(geometry: THREE.BufferGeometry): THREE.BufferGeometry {
  geometry.rotateX(-Math.PI / 2)
  geometry.scale(1, 1, 1 / Math.cos(TILT))
  return geometry
}

/** Moves a geometry vertically so its highest point sits at `top`. */
function topAt(geometry: THREE.BufferGeometry, top: number) {
  geometry.computeBoundingBox()
  geometry.translate(0, top - geometry.boundingBox!.max.y, 0)
  return geometry
}

function canvasTexture(canvas: HTMLCanvasElement) {
  const texture = new THREE.CanvasTexture(canvas)
  texture.colorSpace = THREE.SRGBColorSpace
  texture.anisotropy = 4
  return texture
}

/** Same stadium as stadiumPath, drawn on a 2D canvas in box coordinates. */
function canvasStadium(ctx: CanvasRenderingContext2D, W: number, H: number, inset: number, k: number) {
  const r = (H / 2 - inset) * k
  const cy = (H / 2) * k
  const left = (H / 2) * k
  const right = (W - H / 2) * k
  ctx.beginPath()
  ctx.moveTo(left, cy - r)
  ctx.lineTo(right, cy - r)
  ctx.arc(right, cy, r, -Math.PI / 2, Math.PI / 2)
  ctx.lineTo(left, cy + r)
  ctx.arc(left, cy, r, Math.PI / 2, (3 * Math.PI) / 2)
  ctx.closePath()
}

/** Soft round shadow, shared by everything that sits on the felt. */
let shadowTexture: THREE.Texture | null = null
function contactShadowTexture() {
  if (shadowTexture) return shadowTexture
  const size = 128
  const canvas = document.createElement('canvas')
  canvas.width = canvas.height = size
  const ctx = canvas.getContext('2d')!
  const g = ctx.createRadialGradient(size / 2, size / 2, 0, size / 2, size / 2, size / 2)
  g.addColorStop(0, 'rgba(0,0,0,0.55)')
  g.addColorStop(0.55, 'rgba(0,0,0,0.25)')
  g.addColorStop(1, 'rgba(0,0,0,0)')
  ctx.fillStyle = g
  ctx.fillRect(0, 0, size, size)
  shadowTexture = new THREE.CanvasTexture(canvas)
  return shadowTexture
}

function contactShadow(width: number, depth: number) {
  const mesh = new THREE.Mesh(
    new THREE.PlaneGeometry(width, depth),
    new THREE.MeshBasicMaterial({ map: contactShadowTexture(), transparent: true, depthWrite: false }),
  )
  mesh.rotation.x = -Math.PI / 2
  mesh.renderOrder = -1
  return mesh
}

// ---------------------------------------------------------------------------
// The table

/** Rail width and the wood racetrack inside it, as ratios of table height. */
const RAIL = 0.075
const TRIM = 0.026

function drawFelt(W: number, H: number): HTMLCanvasElement {
  const cw = Math.min(2048, Math.ceil(W * 2))
  const k = cw / W
  const ch = Math.ceil(H * k)
  const canvas = document.createElement('canvas')
  canvas.width = cw
  canvas.height = ch
  const ctx = canvas.getContext('2d')!

  // Cloth: lit from a little above centre, falling off toward the rail.
  const glow = ctx.createRadialGradient(cw * 0.5, ch * 0.42, 0, cw * 0.5, ch * 0.5, cw * 0.55)
  glow.addColorStop(0, '#1b9564')
  glow.addColorStop(0.45, '#0f6a45')
  glow.addColorStop(1, '#063522')
  ctx.fillStyle = glow
  ctx.fillRect(0, 0, cw, ch)

  // Fibre grain: a fixed-seed speckle, so it never shimmers between renders.
  const img = ctx.getImageData(0, 0, cw, ch)
  const rand = mulberry32(1337)
  for (let i = 0; i < img.data.length; i += 4) {
    const n = (rand() - 0.5) * 14
    img.data[i] += n
    img.data[i + 1] += n
    img.data[i + 2] += n
  }
  ctx.putImageData(img, 0, 0)

  // Shade the cloth where it tucks under the rail.
  const feltInset = (RAIL + TRIM) * H
  ctx.save()
  canvasStadium(ctx, W, H, feltInset, k)
  ctx.clip()
  ctx.shadowColor = 'rgba(0,0,0,0.7)'
  ctx.shadowBlur = H * 0.06 * k
  ctx.lineWidth = H * 0.04 * k
  ctx.strokeStyle = 'rgba(0,0,0,0.6)'
  canvasStadium(ctx, W, H, feltInset - H * 0.02, k)
  ctx.stroke()
  ctx.restore()

  // The printed betting line, doubled like a real layout.
  ctx.strokeStyle = 'rgba(212,175,55,0.42)'
  ctx.lineWidth = Math.max(1, H * 0.0045 * k)
  canvasStadium(ctx, W, H, feltInset + H * 0.075, k)
  ctx.stroke()
  ctx.strokeStyle = 'rgba(212,175,55,0.18)'
  canvasStadium(ctx, W, H, feltInset + H * 0.09, k)
  ctx.stroke()

  return canvas
}

/**
 * The table: a padded leather rail, a wood racetrack, the felt, and the apron
 * under the rail. Each part is its own named mesh so it can be picked out or
 * restyled on its own.
 */
export function createPokerTableModel({ width: W, height: H }: TableModelSpec): THREE.Group {
  const group = new THREE.Group()
  group.name = 'poker-table'
  const halfW = W / 2
  const halfH = H / 2

  // Padded rail: an extruded ring with a deep, rounded bevel.
  const bevel = 0.026 * H
  const railGeo = new THREE.ExtrudeGeometry(stadiumShape(halfW, halfH, bevel, RAIL * H - bevel), {
    depth: 0.008 * H,
    bevelEnabled: true,
    bevelThickness: 0.024 * H,
    bevelSize: bevel,
    bevelSegments: 6,
    curveSegments: 64,
  })
  const rail = new THREE.Mesh(
    topAt(layFlat(railGeo), 0),
    new THREE.MeshStandardMaterial({ color: '#2b1a12', roughness: 0.48, metalness: 0.05 }),
  )
  rail.name = 'rail'
  group.add(rail)

  // Wood racetrack between rail and felt, with a gold inlay along its inner edge.
  const trimGeo = new THREE.ExtrudeGeometry(stadiumShape(halfW, halfH, RAIL * H - 0.004 * H, (RAIL + TRIM) * H), {
    depth: 0.01 * H,
    bevelEnabled: false,
    curveSegments: 64,
  })
  const trim = new THREE.Mesh(
    topAt(layFlat(trimGeo), -0.012 * H),
    new THREE.MeshStandardMaterial({ color: '#7a4a24', roughness: 0.32, metalness: 0.08 }),
  )
  trim.name = 'racetrack'
  group.add(trim)

  const inlayGeo = new THREE.ExtrudeGeometry(
    stadiumShape(halfW, halfH, (RAIL + TRIM) * H - 0.005 * H, (RAIL + TRIM) * H),
    { depth: 0.002 * H, bevelEnabled: false, curveSegments: 64 },
  )
  const inlay = new THREE.Mesh(
    topAt(layFlat(inlayGeo), -0.0115 * H),
    new THREE.MeshStandardMaterial({ color: '#d4af37', roughness: 0.3, metalness: 0.85 }),
  )
  inlay.name = 'inlay'
  group.add(inlay)

  // Felt: a flat shape whose UVs map the whole table box, so the texture is
  // drawn in exactly the screen proportions the player sees.
  const feltGeo = new THREE.ShapeGeometry(stadiumShape(halfW, halfH, (RAIL + TRIM) * H - 0.004 * H), 64)
  const pos = feltGeo.attributes.position
  const uv = new Float32Array(pos.count * 2)
  for (let i = 0; i < pos.count; i++) {
    uv[i * 2] = (pos.getX(i) + halfW) / W
    uv[i * 2 + 1] = (pos.getY(i) + halfH) / H
  }
  feltGeo.setAttribute('uv', new THREE.BufferAttribute(uv, 2))
  const felt = new THREE.Mesh(
    topAt(layFlat(feltGeo), tableHeights(H).felt),
    new THREE.MeshStandardMaterial({ map: canvasTexture(drawFelt(W, H)), roughness: 0.95, metalness: 0 }),
  )
  felt.name = 'felt'
  group.add(felt)

  // Apron: the table's side, seen below the near rail thanks to the tilt.
  const apronGeo = new THREE.ExtrudeGeometry(stadiumShape(halfW, halfH, 0.012 * H), {
    depth: 0.06 * H,
    bevelEnabled: false,
    curveSegments: 64,
  })
  const apron = new THREE.Mesh(
    topAt(layFlat(apronGeo), -0.03 * H),
    new THREE.MeshStandardMaterial({ color: '#1e110a', roughness: 0.6, metalness: 0.05 }),
  )
  apron.name = 'apron'
  group.add(apron)

  const shadow = contactShadow(W * 1.12, (H * 1.3) / Math.cos(TILT))
  shadow.position.y = -0.1 * H
  shadow.position.z = (0.04 * H) / Math.cos(TILT)
  shadow.name = 'floor-shadow'
  group.add(shadow)

  return group
}

// ---------------------------------------------------------------------------
// Chips

interface Denomination {
  value: number
  base: string
  spot: string
}

/** Casino colours, highest first. */
const DENOMINATIONS: Denomination[] = [
  { value: 5000, base: '#e8772e', spot: '#fff4e0' },
  { value: 1000, base: '#e2b43a', spot: '#3a2a08' },
  { value: 500, base: '#6c3fa6', spot: '#f1e8ff' },
  { value: 100, base: '#1c1c22', spot: '#d4af37' },
  { value: 25, base: '#1f8a4c', spot: '#ffffff' },
  { value: 5, base: '#c8283a', spot: '#ffffff' },
  { value: 1, base: '#e9e6dc', spot: '#2563eb' },
]

const MAX_PER_STACK = 10
const MAX_STACKS = 4

/** Breaks an amount into chips, largest first, capped to what reads on a phone. */
export function chipsFor(amount: number): Denomination[][] {
  const chips: Denomination[] = []
  let left = Math.max(0, Math.round(amount))
  for (const d of DENOMINATIONS) {
    while (left >= d.value && chips.length < MAX_PER_STACK * MAX_STACKS) {
      chips.push(d)
      left -= d.value
    }
  }
  if (chips.length === 0 && amount > 0) chips.push(DENOMINATIONS[DENOMINATIONS.length - 1])

  // One stack per denomination where possible, tallest at the back.
  const stacks: Denomination[][] = []
  for (const chip of chips) {
    const stack = stacks.find((s) => s[0].value === chip.value && s.length < MAX_PER_STACK)
    if (stack) stack.push(chip)
    else if (stacks.length < MAX_STACKS) stacks.push([chip])
    else stacks[stacks.length - 1].push(chip)
  }
  return stacks
}

const chipMaterials = new Map<number, THREE.Material[]>()

function chipMaterialsFor(d: Denomination): THREE.Material[] {
  const cached = chipMaterials.get(d.value)
  if (cached) return cached

  // Edge: base colour broken by six rectangular spots.
  const side = document.createElement('canvas')
  side.width = 256
  side.height = 16
  const sctx = side.getContext('2d')!
  sctx.fillStyle = d.base
  sctx.fillRect(0, 0, 256, 16)
  sctx.fillStyle = d.spot
  for (let i = 0; i < 6; i++) sctx.fillRect(i * (256 / 6) + 10, 0, 18, 16)

  // Face: spotted outer ring, a fine inner line, and a plain centre inlay.
  const face = document.createElement('canvas')
  face.width = face.height = 128
  const fctx = face.getContext('2d')!
  fctx.fillStyle = d.base
  fctx.beginPath()
  fctx.arc(64, 64, 64, 0, Math.PI * 2)
  fctx.fill()
  fctx.strokeStyle = d.spot
  fctx.lineWidth = 14
  for (let i = 0; i < 6; i++) {
    const a = (i / 6) * Math.PI * 2
    fctx.beginPath()
    fctx.arc(64, 64, 56, a, a + 0.38)
    fctx.stroke()
  }
  fctx.lineWidth = 2
  fctx.beginPath()
  fctx.arc(64, 64, 40, 0, Math.PI * 2)
  fctx.stroke()
  fctx.fillStyle = 'rgba(255,255,255,0.14)'
  fctx.beginPath()
  fctx.arc(64, 64, 36, 0, Math.PI * 2)
  fctx.fill()

  const materials = [
    new THREE.MeshStandardMaterial({ map: canvasTexture(side), roughness: 0.45, metalness: 0.05 }),
    new THREE.MeshStandardMaterial({ map: canvasTexture(face), roughness: 0.4, metalness: 0.05 }),
    new THREE.MeshStandardMaterial({ map: canvasTexture(face), roughness: 0.4, metalness: 0.05 }),
  ]
  chipMaterials.set(d.value, materials)
  return materials
}

const chipGeometries = new Map<number, THREE.CylinderGeometry>()

/** One chip, its base resting on y = 0. */
export function createChipModel(d: Denomination, radius: number): THREE.Mesh {
  const key = Math.round(radius * 100)
  let geometry = chipGeometries.get(key)
  if (!geometry) {
    geometry = new THREE.CylinderGeometry(radius, radius, radius * 0.24, 40, 1)
    geometry.translate(0, radius * 0.12, 0)
    chipGeometries.set(key, geometry)
  }
  const mesh = new THREE.Mesh(geometry, chipMaterialsFor(d))
  mesh.name = `chip-${d.value}`
  return mesh
}

/** Chip thickness for a given radius, matching createChipModel. */
export function chipThickness(radius: number) {
  return radius * 0.24
}

/**
 * A bet or a pot: up to four stacks in a tight cluster, each chip nudged by a
 * fixed amount so the stacks look hand-placed rather than machined.
 */
export function createChipStackModel(amount: number, radius: number): THREE.Group {
  const group = new THREE.Group()
  group.name = 'chip-stack'
  const stacks = chipsFor(amount)
  const rand = mulberry32(Math.round(amount) + 7)
  const step = radius * 1.95
  const offsets = [
    [0, 0],
    [step, 0],
    [step / 2, -step * 0.85],
    [-step, 0],
  ]
  const centre = offsets.slice(0, stacks.length).reduce((a, [x, z]) => [a[0] + x, a[1] + z], [0, 0])
  centre[0] /= stacks.length || 1
  centre[1] /= stacks.length || 1

  stacks.forEach((stack, s) => {
    const [ox, oz] = offsets[s]
    stack.forEach((d, i) => {
      const chip = createChipModel(d, radius)
      chip.position.set(
        ox - centre[0] + (rand() - 0.5) * radius * 0.12,
        i * chipThickness(radius),
        (oz - centre[1]) / Math.cos(TILT) + (rand() - 0.5) * radius * 0.12,
      )
      chip.rotation.y = rand() * Math.PI * 2
      group.add(chip)
    })
  })

  const shadow = contactShadow(radius * 2.4 * Math.max(1, stacks.length * 0.8), (radius * 2.4) / Math.cos(TILT))
  shadow.position.y = 0.2
  group.add(shadow)
  return group
}

// ---------------------------------------------------------------------------
// Dealer button

export function createDealerButtonModel(radius: number): THREE.Group {
  const face = document.createElement('canvas')
  face.width = face.height = 256
  const ctx = face.getContext('2d')!
  const g = ctx.createRadialGradient(110, 100, 10, 128, 128, 128)
  g.addColorStop(0, '#ffffff')
  g.addColorStop(1, '#dcd6c8')
  ctx.fillStyle = g
  ctx.beginPath()
  ctx.arc(128, 128, 128, 0, Math.PI * 2)
  ctx.fill()
  ctx.strokeStyle = '#111'
  ctx.lineWidth = 8
  ctx.beginPath()
  ctx.arc(128, 128, 112, 0, Math.PI * 2)
  ctx.stroke()
  ctx.fillStyle = '#111'
  ctx.textAlign = 'center'
  ctx.textBaseline = 'middle'
  ctx.font = '900 132px Outfit, ui-sans-serif, system-ui, sans-serif'
  ctx.fillText('D', 128, 122)
  ctx.font = '700 30px Outfit, ui-sans-serif, system-ui, sans-serif'
  ctx.fillText('DEALER', 128, 198)

  const top = new THREE.MeshStandardMaterial({ map: canvasTexture(face), roughness: 0.35 })
  const side = new THREE.MeshStandardMaterial({ color: '#efeadf', roughness: 0.4 })
  const height = radius * 0.32
  const geometry = new THREE.CylinderGeometry(radius, radius * 1.02, height, 48, 1)
  geometry.translate(0, height / 2, 0)

  const puck = new THREE.Mesh(geometry, [side, top, side])
  puck.name = 'dealer-puck'
  const group = new THREE.Group()
  group.name = 'dealer-button'
  group.add(puck)

  const shadow = contactShadow(radius * 2.8, (radius * 2.8) / Math.cos(TILT))
  shadow.position.y = 0.2
  group.add(shadow)
  return group
}

/** Frees the GPU resources a model created, except the shared chip caches. */
export function disposeModel(object: THREE.Object3D) {
  object.traverse((child) => {
    if (!(child instanceof THREE.Mesh)) return
    if (!(child.geometry instanceof THREE.CylinderGeometry) || !child.name.startsWith('chip-')) {
      child.geometry.dispose()
    }
    const materials = Array.isArray(child.material) ? child.material : [child.material]
    for (const m of materials) {
      if ([...chipMaterials.values()].some((set) => set.includes(m))) continue
      if (m instanceof THREE.MeshStandardMaterial || m instanceof THREE.MeshBasicMaterial) {
        if (m.map && m.map !== shadowTexture) m.map.dispose()
      }
      m.dispose()
    }
  })
}
