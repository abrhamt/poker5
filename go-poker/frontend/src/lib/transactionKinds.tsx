import type { ReactNode } from 'react'

/**
 * Ledger rows arrive as raw type codes from the Go wallet service
 * (services/wallet_service.go). Players should never see "winner_payout", so
 * every known code maps to a label, an icon and a tone here — and anything
 * unrecognised degrades to a title-cased version of the code rather than
 * vanishing from the statement.
 */
interface TransactionKind {
  label: string
  icon: ReactNode
  tone: string
}

const icon = (path: ReactNode) => (
  <svg viewBox="0 0 24 24" width="13" height="13" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
    {path}
  </svg>
)

const KINDS: Record<string, TransactionKind> = {
  deposit: {
    label: 'Deposit',
    tone: 'bg-emerald-500/12 text-emerald-300',
    icon: icon(
      <>
        <line x1="12" y1="5" x2="12" y2="19" />
        <polyline points="19 12 12 19 5 12" />
      </>,
    ),
  },
  buy_in: {
    label: 'Table buy-in',
    tone: 'bg-amber-500/12 text-amber-300',
    icon: icon(
      <>
        <ellipse cx="12" cy="6" rx="8" ry="3" />
        <path d="M4 6v6c0 1.66 3.58 3 8 3s8-1.34 8-3V6" />
        <path d="M4 12v6c0 1.66 3.58 3 8 3s8-1.34 8-3v-6" />
      </>,
    ),
  },
  cash_out: {
    label: 'Cash out',
    tone: 'bg-sky-500/12 text-sky-300',
    icon: icon(
      <>
        <line x1="12" y1="19" x2="12" y2="5" />
        <polyline points="5 12 12 5 19 12" />
      </>,
    ),
  },
  winner_payout: {
    label: 'Hand winnings',
    tone: 'bg-gold/15 text-gold-light',
    icon: icon(
      <>
        <path d="M6 9H4.5a2.5 2.5 0 0 1 0-5H6" />
        <path d="M18 9h1.5a2.5 2.5 0 0 0 0-5H18" />
        <path d="M4 22h16" />
        <path d="M10 14.66V17c0 .55-.45 1-1 1H8c-.55 0-1 .45-1 1v1c0 .55.45 1 1 1h8c.55 0 1-.45 1-1v-1c0-.55-.45-1-1-1h-1c-.55 0-1-.45-1-1v-2.34" />
        <path d="M18 2H6v7a6 6 0 0 0 12 0V2z" />
      </>,
    ),
  },
  referral_commission: {
    label: 'Referral bonus',
    tone: 'bg-violet-500/12 text-violet-300',
    icon: icon(
      <>
        <path d="M16 21v-2a4 4 0 0 0-4-4H6a4 4 0 0 0-4 4v2" />
        <circle cx="9" cy="7" r="4" />
        <path d="M22 21v-2a4 4 0 0 0-3-3.87" />
        <path d="M16 3.13a4 4 0 0 1 0 7.75" />
      </>,
    ),
  },
  site_rake: {
    label: 'Rake',
    tone: 'bg-white/8 text-muted',
    icon: icon(
      <>
        <line x1="19" y1="5" x2="5" y2="19" />
        <circle cx="6.5" cy="6.5" r="2.5" />
        <circle cx="17.5" cy="17.5" r="2.5" />
      </>,
    ),
  },
}

const FALLBACK: TransactionKind = {
  label: 'Adjustment',
  tone: 'bg-white/8 text-muted',
  icon: icon(
    <>
      <circle cx="12" cy="12" r="9" />
      <line x1="12" y1="8" x2="12" y2="12" />
      <line x1="12" y1="16" x2="12.01" y2="16" />
    </>,
  ),
}

export function transactionKind(type: string): TransactionKind {
  const known = KINDS[type]
  if (known) return known
  // Unknown codes still read as words rather than snake_case.
  const label = type.replace(/_/g, ' ').replace(/^./, (c) => c.toUpperCase())
  return { ...FALLBACK, label: label || FALLBACK.label }
}
