import type { ReactNode } from 'react'
import { Link } from 'react-router-dom'
import type { User } from '../lib/types'

/**
 * The one menu surface for the app's full-screen pages. Both the lobby and the
 * table hide their navigation behind this so the screen itself stays entirely
 * given over to content — the pages differ only in what they slot into it.
 */
export function SideDrawer({
  open,
  onClose,
  user,
  children,
  footer,
  onLogout,
}: {
  open: boolean
  onClose: () => void
  user: User | null
  children?: ReactNode
  footer?: ReactNode
  onLogout: () => void
}) {
  if (!open) return null

  return (
    <div className="fixed inset-0 z-70 flex" role="dialog" aria-modal="true">
      <div className="absolute inset-0 bg-ink/70 backdrop-blur-sm" onClick={onClose} />

      <aside className="animate-rise relative flex h-full w-[min(80vw,17rem)] flex-col gap-4 overflow-y-auto border-r border-gold/20 bg-surface/95 p-4">
        <div className="flex items-center justify-between gap-2">
          <div className="flex min-w-0 items-center gap-2">
            <span className="grid size-9 shrink-0 place-items-center rounded-full bg-gold/20 font-heading font-bold text-gold-light uppercase">
              {user?.username.charAt(0) ?? '?'}
            </span>
            <div className="flex min-w-0 flex-col">
              <span className="truncate text-sm font-semibold">{user?.username}</span>
              <span className="text-xs text-gold-light tabular-nums">
                {(user?.wallet ?? 0).toLocaleString()} ETB
              </span>
            </div>
          </div>
          <button
            type="button"
            onClick={onClose}
            aria-label="Close menu"
            className="btn-ghost shrink-0 px-2.5 py-2"
          >
            <svg viewBox="0 0 24 24" width="16" height="16" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round">
              <line x1="18" y1="6" x2="6" y2="18" />
              <line x1="6" y1="6" x2="18" y2="18" />
            </svg>
          </button>
        </div>

        {children}

        <div className="mt-auto flex flex-col gap-2">
          {footer}
          <button type="button" onClick={onLogout} className="btn-ghost w-full">
            Log out
          </button>
        </div>
      </aside>
    </div>
  )
}

export function DrawerLink({ to, label }: { to: string; label: string }) {
  return (
    <Link
      to={to}
      className="rounded-xl border border-white/8 bg-white/5 px-3.5 py-2.5 text-sm font-medium text-chalk transition-colors hover:border-gold/30 hover:bg-white/10"
    >
      {label}
    </Link>
  )
}
