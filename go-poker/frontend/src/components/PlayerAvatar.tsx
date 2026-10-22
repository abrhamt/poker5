/**
 * Predefined player avatars, drawn as inline SVG so they stay sharp at every
 * seat size and need no image requests. A player has no avatar of their own on
 * the server yet, so each username is hashed onto one preset — the same name
 * always gets the same face, at every table and on every device.
 */

type Hair = 'short' | 'long' | 'curly' | 'bun' | 'spiky' | 'none'
type Extra = 'sunglasses' | 'glasses' | 'cap' | 'fedora' | 'beard' | 'headphones' | 'none'

interface Preset {
  bg: string
  skin: string
  hair: Hair
  hairColour: string
  shirt: string
  extra: Extra
  /** Colour of the cap, hat or headphones when `extra` needs one. */
  accent?: string
}

const PRESETS: Preset[] = [
  { bg: '#1f6f54', skin: '#f2c9a0', hair: 'short', hairColour: '#3b2416', shirt: '#d4af37', extra: 'sunglasses' },
  { bg: '#7a2e3a', skin: '#8d5a3b', hair: 'curly', hairColour: '#1b120d', shirt: '#f3e5ab', extra: 'none' },
  { bg: '#26457a', skin: '#e8b48a', hair: 'long', hairColour: '#a8642a', shirt: '#2a2a2a', extra: 'none' },
  { bg: '#5b3a82', skin: '#c68a5e', hair: 'none', hairColour: '#2b1b12', shirt: '#1f6f54', extra: 'cap', accent: '#dc2626' },
  { bg: '#8a5a17', skin: '#f5d0b0', hair: 'bun', hairColour: '#20140e', shirt: '#26457a', extra: 'glasses' },
  { bg: '#2f5e6e', skin: '#6b4329', hair: 'short', hairColour: '#120c08', shirt: '#7a2e3a', extra: 'beard' },
  { bg: '#3d3d46', skin: '#e9bf96', hair: 'none', hairColour: '#3b2416', shirt: '#0f0f12', extra: 'fedora', accent: '#1b1b20' },
  { bg: '#7a3b6b', skin: '#a8714c', hair: 'spiky', hairColour: '#d4af37', shirt: '#26457a', extra: 'none' },
  { bg: '#1d5a3c', skin: '#f1c7a5', hair: 'long', hairColour: '#1b120d', shirt: '#7a2e3a', extra: 'sunglasses' },
  { bg: '#6e3d1f', skin: '#7c4b2e', hair: 'short', hairColour: '#120c08', shirt: '#d4af37', extra: 'headphones', accent: '#2563eb' },
  { bg: '#2b4c8c', skin: '#d9a07a', hair: 'curly', hairColour: '#5a3418', shirt: '#f3e5ab', extra: 'glasses' },
  { bg: '#4a5a1f', skin: '#f6d6bc', hair: 'spiky', hairColour: '#7a3b1b', shirt: '#2a2a2a', extra: 'beard' },
]

/** FNV-1a: small, stable, and spreads similar names (dev1, dev2…) apart. */
function hashName(name: string): number {
  let hash = 0x811c9dc5
  for (let i = 0; i < name.length; i++) {
    hash ^= name.charCodeAt(i)
    hash = Math.imul(hash, 0x01000193)
  }
  return hash >>> 0
}

/** How many avatars a player can pick from (the server's AvatarCount). */
export const AVATAR_COUNT = PRESETS.length

/** The player's chosen avatar, or — with no choice (-1/absent) — one derived from their name. */
export function avatarPreset(name: string, avatar?: number): Preset {
  if (avatar !== undefined && avatar >= 0 && avatar < PRESETS.length) return PRESETS[avatar]
  return PRESETS[hashName(name.toLowerCase()) % PRESETS.length]
}

export function PlayerAvatar({ name, avatar, className }: { name: string; avatar?: number; className?: string }) {
  const p = avatarPreset(name, avatar)

  return (
    <svg viewBox="0 0 64 64" className={className} role="img" aria-label={`${name}'s avatar`}>
      <circle cx="32" cy="32" r="32" fill={p.bg} />
      <circle cx="22" cy="16" r="22" fill="white" opacity="0.07" />

      <HairBack hair={p.hair} colour={p.hairColour} />

      {/* Shoulders and neck. */}
      <path d="M8 64 C8 51 19 45 32 45 C45 45 56 51 56 64 Z" fill={p.shirt} />
      <rect x="27" y="36" width="10" height="11" rx="3" fill={p.skin} />
      <rect x="27" y="40" width="10" height="4" fill="black" opacity="0.12" />

      {/* Head. */}
      <circle cx="19.5" cy="30" r="3" fill={p.skin} />
      <circle cx="44.5" cy="30" r="3" fill={p.skin} />
      <ellipse cx="32" cy="28" rx="12.5" ry="13.5" fill={p.skin} />

      <HairFront hair={p.hair} colour={p.hairColour} />

      {/* Face. */}
      <circle cx="27" cy="29" r="1.7" fill="#1b1b1b" />
      <circle cx="37" cy="29" r="1.7" fill="#1b1b1b" />
      <path d="M27.5 35 Q32 38.5 36.5 35" fill="none" stroke="#5a2a1a" strokeWidth="1.6" strokeLinecap="round" />

      <ExtraLayer extra={p.extra} hairColour={p.hairColour} accent={p.accent} />
    </svg>
  )
}

/** Hair that falls behind the head, drawn before it. */
function HairBack({ hair, colour }: { hair: Hair; colour: string }) {
  if (hair === 'long') {
    return <path d="M17 28 C17 10 47 10 47 28 L48 46 C42 49 22 49 16 46 Z" fill={colour} />
  }
  if (hair === 'curly') {
    return (
      <g fill={colour}>
        <circle cx="32" cy="20" r="15" />
        <circle cx="19" cy="25" r="6" />
        <circle cx="45" cy="25" r="6" />
      </g>
    )
  }
  return null
}

/** Hair over the forehead, drawn after the head. */
function HairFront({ hair, colour }: { hair: Hair; colour: string }) {
  switch (hair) {
    case 'short':
    case 'long':
      return <path d="M19.5 27 C19 13 45 13 44.5 27 C41 21 36 19 32 19 C27 19 23 21 19.5 27 Z" fill={colour} />
    case 'curly':
      return (
        <g fill={colour}>
          <circle cx="24" cy="17" r="5" />
          <circle cx="32" cy="15" r="5.5" />
          <circle cx="40" cy="17" r="5" />
        </g>
      )
    case 'bun':
      return (
        <g fill={colour}>
          <circle cx="32" cy="10" r="5.5" />
          <path d="M19.5 26 C19 13 45 13 44.5 26 C40 19 24 19 19.5 26 Z" />
        </g>
      )
    case 'spiky':
      return <path d="M19.5 25 L20 15 L25 19 L27 11 L31 18 L34 10 L37 18 L41 12 L42 19 L45 16 L44.5 25 C40 20 24 20 19.5 25 Z" fill={colour} />
    default:
      return null
  }
}

function ExtraLayer({ extra, hairColour, accent = '#1b1b20' }: { extra: Extra; hairColour: string; accent?: string }) {
  switch (extra) {
    case 'sunglasses':
      return (
        <g>
          <rect x="22" y="26" width="9" height="6" rx="2.5" fill="#111" />
          <rect x="33" y="26" width="9" height="6" rx="2.5" fill="#111" />
          <path d="M31 28 H33" stroke="#111" strokeWidth="1.5" />
          <rect x="23.5" y="27" width="3" height="1.5" rx="0.75" fill="white" opacity="0.35" />
          <rect x="34.5" y="27" width="3" height="1.5" rx="0.75" fill="white" opacity="0.35" />
        </g>
      )
    case 'glasses':
      return (
        <g fill="none" stroke="#1b1b1b" strokeWidth="1.4">
          <circle cx="27" cy="29" r="4" />
          <circle cx="37" cy="29" r="4" />
          <path d="M31 29 H33" />
        </g>
      )
    case 'cap':
      return (
        <g fill={accent}>
          <path d="M19 24 C19 11 45 11 45 24 Z" />
          <path d="M17 24 H47 C47 26 44 26.5 32 26.5 C20 26.5 17 26 17 24 Z" />
          <circle cx="32" cy="12.5" r="1.5" fill="white" opacity="0.5" />
        </g>
      )
    case 'fedora':
      return (
        <g fill={accent}>
          <ellipse cx="32" cy="20" rx="20" ry="3.5" />
          <path d="M22 20 C22 9 42 9 42 20 Z" />
          <rect x="22" y="16" width="20" height="2.5" fill="#d4af37" />
        </g>
      )
    case 'beard':
      return <path d="M19.5 29 C20 40 26 43 32 43 C38 43 44 40 44.5 29 C42 35 39 36 32 36 C25 36 22 35 19.5 29 Z M28 36.5 Q32 39 36 36.5" fill={hairColour} />
    case 'headphones':
      return (
        <g>
          <path d="M18 30 C18 10 46 10 46 30" fill="none" stroke={accent} strokeWidth="3" />
          <rect x="15" y="26" width="6" height="10" rx="3" fill={accent} />
          <rect x="43" y="26" width="6" height="10" rx="3" fill={accent} />
        </g>
      )
    default:
      return null
  }
}
