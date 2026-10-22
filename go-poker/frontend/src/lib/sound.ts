import { useSyncExternalStore } from 'react'
import cardUrl from './freesound_community-card-sounds-35956.mp3'
import chipsUrl from './oxidvideos-placing-poker-chips-522521.mp3'
import shuffleUrl from './freesound_community-riffle-card-shuffle-104313.mp3'

/**
 * Table sound effects. Cards, chips and the shuffle are recorded samples,
 * decoded once into Web Audio buffers so they start instantly and can
 * overlap; the win fanfare is synthesised.
 *
 * Browsers keep audio suspended until the page has had a user gesture, so the
 * context is created — and the samples fetched — on the first tap; a sound
 * requested before then is simply skipped.
 *
 * The on/off choice is a per-device preference, kept in localStorage.
 */

const STORAGE_KEY = 'poker.sound'
const listeners = new Set<() => void>()

let ctx: AudioContext | null = null
let enabled = readEnabled()

const SAMPLE_URLS = { card: cardUrl, chips: chipsUrl, shuffle: shuffleUrl }
type Sample = keyof typeof SAMPLE_URLS
const samples = new Map<Sample, AudioBuffer>()
let loading: Promise<void> | null = null

function readEnabled(): boolean {
  try {
    return localStorage.getItem(STORAGE_KEY) !== 'off'
  } catch {
    return true
  }
}

export function setSoundEnabled(on: boolean) {
  enabled = on
  try {
    localStorage.setItem(STORAGE_KEY, on ? 'on' : 'off')
  } catch {
    /* private mode — the choice just won't outlive the tab */
  }
  listeners.forEach((l) => l())
}

export function useSoundEnabled(): boolean {
  return useSyncExternalStore(
    (l) => {
      listeners.add(l)
      return () => listeners.delete(l)
    },
    () => enabled,
  )
}

/** Creates/resumes the audio context and loads the samples; call from a user gesture. */
export function unlockAudio() {
  try {
    ctx ??= new AudioContext()
    if (ctx.state === 'suspended') void ctx.resume()
    loadSamples(ctx)
  } catch {
    /* no Web Audio — every sound becomes a no-op */
  }
}

function loadSamples(ac: AudioContext) {
  loading ??= Promise.all(
    (Object.entries(SAMPLE_URLS) as [Sample, string][]).map(async ([name, url]) => {
      try {
        const res = await fetch(url)
        samples.set(name, await ac.decodeAudioData(await res.arrayBuffer()))
      } catch {
        /* that one sound stays silent */
      }
    }),
  ).then(() => undefined)
}

function audio(): AudioContext | null {
  if (!enabled || !ctx || ctx.state !== 'running') return null
  return ctx
}

function playSample(
  name: Sample,
  { delayMs = 0, gain = 1, rate = 1, maxSeconds }: { delayMs?: number; gain?: number; rate?: number; maxSeconds?: number } = {},
) {
  const ac = audio()
  const buffer = samples.get(name)
  if (!ac || !buffer) return

  const start = ac.currentTime + delayMs / 1000
  const src = ac.createBufferSource()
  const amp = ac.createGain()
  src.buffer = buffer
  src.playbackRate.value = rate
  amp.gain.setValueAtTime(gain, start)
  if (maxSeconds !== undefined) {
    // Cut long samples short with a quick fade rather than a click.
    amp.gain.setValueAtTime(gain, start + maxSeconds - 0.25)
    amp.gain.linearRampToValueAtTime(0, start + maxSeconds)
  }
  src.connect(amp).connect(ac.destination)
  src.start(start)
  if (maxSeconds !== undefined) src.stop(start + maxSeconds + 0.05)
}

/** A board card landing and turning over. */
export function playDeal(delayMs = 0) {
  // A touch of pitch variety so a flop doesn't sound like one sample three times.
  playSample('card', { delayMs, gain: 0.9, rate: 0.95 + Math.random() * 0.1 })
}

/**
 * Chips going down. `repeats` layers extra, slightly offset copies for a
 * bigger clatter — a whole pot being pushed to the winner.
 */
export function playChips(delayMs = 0, repeats = 1) {
  for (let i = 0; i < repeats; i++) {
    playSample('chips', { delayMs: delayMs + i * 170, gain: 0.85, rate: 0.96 + Math.random() * 0.08 })
  }
}

/** The deck being riffled for a new hand: the start of the recording, faded out. */
export function playShuffle(delayMs = 0) {
  playSample('shuffle', { delayMs, gain: 0.55, maxSeconds: 1.8 })
}

function tone(
  ac: AudioContext,
  { freq, start, duration, type = 'sine', gain = 0.2 }: {
    freq: number
    start: number
    duration: number
    type?: OscillatorType
    gain?: number
  },
) {
  const osc = ac.createOscillator()
  const amp = ac.createGain()
  osc.type = type
  osc.frequency.value = freq
  amp.gain.setValueAtTime(0, start)
  amp.gain.linearRampToValueAtTime(gain, start + 0.008)
  amp.gain.exponentialRampToValueAtTime(0.0001, start + duration)
  osc.connect(amp).connect(ac.destination)
  osc.start(start)
  osc.stop(start + duration + 0.02)
}

/** The hero won the pot: a rising arpeggio landing on a held major chord. */
export function playFanfare(delayMs = 0) {
  const ac = audio()
  if (!ac) return
  const t = ac.currentTime + delayMs / 1000
  const notes = [523.25, 659.25, 783.99, 1046.5] // C5 E5 G5 C6
  notes.forEach((freq, i) => {
    tone(ac, { freq, start: t + i * 0.11, duration: 0.3, type: 'triangle', gain: 0.18 })
  })
  const chordAt = t + notes.length * 0.11
  for (const freq of [523.25, 659.25, 783.99, 1046.5]) {
    tone(ac, { freq, start: chordAt, duration: 1.1, type: 'triangle', gain: 0.1 })
    tone(ac, { freq: freq * 2, start: chordAt, duration: 0.8, gain: 0.03 })
  }
  // A little sparkle on top.
  for (let i = 0; i < 6; i++) {
    tone(ac, { freq: 2093 + i * 260, start: chordAt + 0.05 + i * 0.06, duration: 0.18, gain: 0.04 })
  }
}
