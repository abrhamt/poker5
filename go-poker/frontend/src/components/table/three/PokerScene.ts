/**
 * Owns the WebGL side of the table: renderer, camera, lights, the table model,
 * and every chip and button that sits on it. React hands it stage-pixel
 * positions (the same ones the DOM seats use) and it keeps the 3D in step.
 *
 * It renders on demand — a frame when something changed, a loop only while
 * an animation is running — so an idle table costs nothing on a phone.
 */
import * as THREE from 'three'
import {
  TILT,
  chipThickness,
  chipsFor,
  createChipModel,
  createChipStackModel,
  createDealerButtonModel,
  createPokerTableModel,
  disposeModel,
  tableHeights,
} from './tableModels'

export interface StagePoint {
  x: number
  y: number
}

export interface TableBox {
  left: number
  top: number
  width: number
  height: number
}

export interface SceneBet {
  id: string
  amount: number
  at: StagePoint
}

export interface SceneAward {
  amount: number
  to: StagePoint
}

interface Tween {
  start: number
  delay: number
  duration: number
  update: (t: number) => void
  done?: () => void
}

const easeOut = (t: number) => 1 - (1 - t) ** 3
const easeInOut = (t: number) => (t < 0.5 ? 4 * t ** 3 : 1 - (-2 * t + 2) ** 3 / 2)

const BET_COLLECT_MS = 380
const AWARD_FLIGHT_MS = 700
const AWARD_STAGGER_MS = 40

export class PokerScene {
  private renderer: THREE.WebGLRenderer
  private scene = new THREE.Scene()
  private camera = new THREE.OrthographicCamera()
  private table: THREE.Group | null = null
  private box: TableBox = { left: 0, top: 0, width: 0, height: 0 }

  private bets = new Map<string, { group: THREE.Group; amount: number }>()
  private pot: { group: THREE.Group; amount: number } | null = null
  /** Amount the pot is about to show once in-flight bets land, if any. */
  private potPending: number | null = null
  private potAt: StagePoint = { x: 0, y: 0 }
  private dealer: { group: THREE.Group; at: StagePoint } | null = null
  private collectingUntil = 0

  private last: { bets: SceneBet[]; pot: number; potAt: StagePoint; dealer: StagePoint | null } = {
    bets: [],
    pot: 0,
    potAt: { x: 0, y: 0 },
    dealer: null,
  }

  /**
   * A second, transparent layer drawn above the DOM cards, for the payout's
   * flying chips: on the table layer they would pass underneath the board.
   */
  private overlay: { renderer: THREE.WebGLRenderer; scene: THREE.Scene } | null = null

  private tweens: Tween[] = []
  private frame = 0

  constructor(canvas: HTMLCanvasElement, overlayCanvas?: HTMLCanvasElement) {
    this.renderer = createRenderer(canvas)
    addLights(this.scene)
    if (overlayCanvas) {
      const scene = new THREE.Scene()
      addLights(scene)
      this.overlay = { renderer: createRenderer(overlayCanvas), scene }
    }
  }

  private get H() {
    return this.box.height
  }

  private get chipRadius() {
    return this.H * 0.036
  }

  /** Stage pixels → a point on the felt (or `lift` above it). */
  private toWorld(p: StagePoint, lift = 0): THREE.Vector3 {
    const y = tableHeights(this.H).felt + lift
    const cx = this.box.left + this.box.width / 2
    const cy = this.box.top + this.box.height / 2
    return new THREE.Vector3(p.x - cx, y, (p.y - cy + y * Math.sin(TILT)) / Math.cos(TILT))
  }

  layout(stage: { width: number; height: number }, box: TableBox) {
    const resized = box.width !== this.box.width || box.height !== this.box.height
    this.box = box
    this.renderer.setSize(stage.width, stage.height, false)
    this.overlay?.renderer.setSize(stage.width, stage.height, false)

    // Frame the camera so the world origin lands on the table box's centre.
    const cx = box.left + box.width / 2
    const cy = box.top + box.height / 2
    const cam = this.camera
    cam.left = -cx
    cam.right = stage.width - cx
    cam.top = cy
    cam.bottom = cy - stage.height
    cam.near = 1
    cam.far = 20000
    const distance = 5000
    cam.position.set(0, distance * Math.cos(TILT), distance * Math.sin(TILT))
    cam.lookAt(0, 0, 0)
    cam.updateProjectionMatrix()

    if (resized) {
      if (this.table) {
        this.scene.remove(this.table)
        disposeModel(this.table)
      }
      this.table = createPokerTableModel({ width: box.width, height: box.height })
      this.scene.add(this.table)

      // Chip and button sizes follow the table, so rebuild them at the new scale.
      for (const { group } of this.bets.values()) this.removeObject(group)
      this.bets.clear()
      if (this.pot) this.removeObject(this.pot.group)
      this.pot = null
      this.potPending = null
      if (this.dealer) this.removeObject(this.dealer.group)
      this.dealer = null
    }

    this.setBets(this.last.bets, false)
    this.setPot(this.last.pot, this.last.potAt)
    this.setDealer(this.last.dealer)
    this.requestRender()
  }

  /** Chips in front of each seat for the current betting round. */
  setBets(bets: SceneBet[], animate = true) {
    this.last.bets = bets
    if (!this.table) return
    const seen = new Set<string>()

    for (const bet of bets) {
      seen.add(bet.id)
      const current = this.bets.get(bet.id)
      const target = this.toWorld(bet.at)
      if (current && current.amount === bet.amount) {
        current.group.position.copy(target)
        continue
      }
      if (current) this.removeObject(current.group)

      const group = createChipStackModel(bet.amount, this.chipRadius)
      group.position.copy(target)
      this.scene.add(group)
      this.bets.set(bet.id, { group, amount: bet.amount })

      if (animate) {
        const drop = this.H * 0.08
        this.tween(0, 240, (t) => {
          group.position.y = target.y + drop * (1 - easeOut(t))
        })
      }
    }

    // A bet that vanished was swept into the pot: slide it there, then drop it.
    for (const [id, { group }] of this.bets) {
      if (seen.has(id)) continue
      this.bets.delete(id)
      if (!animate) {
        this.removeObject(group)
        continue
      }
      const from = group.position.clone()
      const to = this.toWorld(this.potAt)
      this.collectingUntil = performance.now() + BET_COLLECT_MS
      this.tween(
        0,
        BET_COLLECT_MS,
        (t) => group.position.lerpVectors(from, to, easeInOut(t)),
        () => this.removeObject(group),
      )
    }
    this.requestRender()
  }

  /** Chips already collected into the middle. */
  setPot(amount: number, at: StagePoint) {
    this.last.pot = amount
    this.last.potAt = at
    this.potAt = at
    if (!this.table) return

    const showing = this.potPending ?? this.pot?.amount ?? 0
    if (showing === amount) {
      this.pot?.group.position.copy(this.toWorld(at))
      this.requestRender()
      return
    }

    // Let any bets still sliding in arrive before the pot grows.
    this.potPending = amount
    const wait = Math.max(0, this.collectingUntil - performance.now())
    this.tween(wait, 0, () => {}, () => {
      if (this.potPending !== amount) return
      this.potPending = null
      if (this.pot) this.removeObject(this.pot.group)
      this.pot = null
      if (amount <= 0) return
      const group = createChipStackModel(amount, this.chipRadius)
      group.position.copy(this.toWorld(this.potAt))
      this.scene.add(group)
      this.pot = { group, amount }
    })
    this.requestRender()
  }

  /** The dealer button, sliding to its new seat each hand. */
  setDealer(at: StagePoint | null) {
    this.last.dealer = at
    if (!this.table) return

    if (!at) {
      if (this.dealer) this.removeObject(this.dealer.group)
      this.dealer = null
      this.requestRender()
      return
    }

    const target = this.toWorld(at)
    if (!this.dealer) {
      const group = createDealerButtonModel(this.H * 0.047)
      group.position.copy(target)
      this.scene.add(group)
      this.dealer = { group, at }
    } else if (this.dealer.at.x !== at.x || this.dealer.at.y !== at.y) {
      const { group } = this.dealer
      const from = group.position.clone()
      this.dealer.at = at
      this.tween(0, 550, (t) => {
        const e = easeInOut(t)
        group.position.lerpVectors(from, target, e)
        group.position.y = target.y + Math.sin(Math.PI * e) * this.H * 0.05
      })
    }
    this.requestRender()
  }

  /**
   * Pays a pot out: its chips arc from the middle to the winner one after
   * another. Returns how long the flight takes, so the caller can time the
   * balance count-up to the chips landing.
   */
  award(awards: SceneAward[]): number {
    if (!this.table) return 0
    if (this.pot) {
      this.removeObject(this.pot.group)
      this.pot = null
    }
    this.potPending = null

    const r = this.chipRadius
    const from = this.toWorld(this.potAt)
    const layer = this.overlay?.scene ?? this.scene
    let longest = 0

    for (const award of awards) {
      const to = this.toWorld(award.to)
      const chips = chipsFor(award.amount).flat().slice(0, 24)
      chips.forEach((d, i) => {
        const chip = createChipModel(d, r)
        const start = from.clone().add(
          new THREE.Vector3((((i * 37) % 7) - 3) * r * 0.35, i * chipThickness(r) * 0.6, (((i * 53) % 5) - 2) * r * 0.3),
        )
        chip.position.copy(start)
        layer.add(chip)

        const delay = i * AWARD_STAGGER_MS
        longest = Math.max(longest, delay + AWARD_FLIGHT_MS)
        const arc = this.H * (0.22 + (i % 3) * 0.04)
        this.tween(
          delay,
          AWARD_FLIGHT_MS,
          (t) => {
            const e = easeInOut(t)
            chip.position.lerpVectors(start, to, e)
            chip.position.y += Math.sin(Math.PI * e) * arc
            chip.rotation.x = e * Math.PI * 2
            const s = t > 0.85 ? 1 - (t - 0.85) / 0.15 : 1
            chip.scale.setScalar(Math.max(0.01, s))
          },
          () => layer.remove(chip),
        )
      })
    }
    this.requestRender()
    return longest
  }

  private removeObject(object: THREE.Object3D) {
    this.scene.remove(object)
    disposeModel(object)
  }

  private tween(delay: number, duration: number, update: (t: number) => void, done?: () => void) {
    this.tweens.push({ start: performance.now(), delay, duration, update, done })
    this.requestRender()
  }

  requestRender() {
    if (!this.frame) this.frame = requestAnimationFrame(this.tick)
  }

  private tick = (now: number) => {
    this.frame = 0
    const running: Tween[] = []
    for (const tw of this.tweens) {
      const elapsed = now - tw.start - tw.delay
      if (elapsed < 0) {
        running.push(tw)
        continue
      }
      const t = tw.duration <= 0 ? 1 : Math.min(1, elapsed / tw.duration)
      tw.update(t)
      if (t < 1) running.push(tw)
      else tw.done?.()
    }
    this.tweens = running
    this.renderer.render(this.scene, this.camera)
    // Also once after the last chip lands, which clears the layer.
    this.overlay?.renderer.render(this.overlay.scene, this.camera)
    if (this.tweens.length) this.requestRender()
  }

  dispose() {
    cancelAnimationFrame(this.frame)
    this.frame = 0
    this.tweens = []
    disposeModel(this.scene)
    this.scene.clear()
    this.renderer.dispose()
    if (this.overlay) {
      this.overlay.scene.clear()
      this.overlay.renderer.dispose()
    }
  }
}

function createRenderer(canvas: HTMLCanvasElement) {
  const renderer = new THREE.WebGLRenderer({ canvas, antialias: true, alpha: true, powerPreference: 'low-power' })
  renderer.setPixelRatio(Math.min(window.devicePixelRatio, 2))
  renderer.setClearColor(0x000000, 0)
  return renderer
}

function addLights(scene: THREE.Scene) {
  scene.add(new THREE.HemisphereLight('#fff6e4', '#0b1c14', 2.2))
  const key = new THREE.DirectionalLight('#fff0d4', 2.4)
  key.position.set(-0.35, 1, 0.55)
  scene.add(key)
  const rim = new THREE.DirectionalLight('#bfe6ff', 0.5)
  rim.position.set(0.6, 0.5, -0.8)
  scene.add(rim)
}
