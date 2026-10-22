import { useState } from 'react'
import type { ReactNode } from 'react'
import { Link, useNavigate } from 'react-router-dom'
import { useAuth } from '../lib/auth'
import { DrawerLink, SideDrawer } from './SideDrawer'

const ACCOUNT_PAGES = [
  { to: '/lobby', label: 'Lobby' },
  { to: '/wallet', label: 'Wallet' },
  { to: '/history', label: 'Game history' },
  { to: '/analytics', label: 'Analytics' },
  { to: '/profile', label: 'Profile' },
]

/** The drawer's page links, shared by every screen, minus the one you're on. */
export function AccountNav({ current }: { current?: string }) {
  return (
    <nav className="flex flex-col gap-1.5">
      {ACCOUNT_PAGES.filter((p) => p.to !== current).map((p) => (
        <DrawerLink key={p.to} to={p.to} label={p.label} />
      ))}
    </nav>
  )
}

/**
 * The frame for the account pages: the same header and drawer as the wallet,
 * with a body that scrolls, since a history can run long.
 */
export function PageShell({ title, path, children }: { title: string; path: string; children: ReactNode }) {
  const { user, logout } = useAuth()
  const navigate = useNavigate()
  const [menuOpen, setMenuOpen] = useState(false)

  return (
    <div className="fixed inset-0 flex flex-col overflow-hidden">
      <header className="shrink-0 border-b border-gold/20 bg-ink/80 backdrop-blur-xl">
        <div className="mx-auto flex w-full max-w-2xl items-center justify-between gap-2 px-3 py-2">
          <div className="flex items-center gap-2">
            <button
              type="button"
              onClick={() => setMenuOpen(true)}
              aria-label="Open menu"
              className="grid size-9 place-items-center rounded-xl border border-white/12 bg-white/5 text-white/85 active:scale-95"
            >
              <svg viewBox="0 0 24 24" width="17" height="17" fill="none" stroke="currentColor" strokeWidth="2.5" strokeLinecap="round">
                <line x1="3" y1="6" x2="21" y2="6" />
                <line x1="3" y1="12" x2="21" y2="12" />
                <line x1="3" y1="18" x2="21" y2="18" />
              </svg>
            </button>
            <span className="font-heading text-xs tracking-[0.2em] text-muted uppercase">{title}</span>
          </div>

          <Link to="/lobby" className="btn-ghost px-3 py-1.5 text-xs">
            Lobby
          </Link>
        </div>
      </header>

      <main className="min-h-0 flex-1 overflow-y-auto overscroll-contain">
        <div className="mx-auto flex w-full max-w-2xl flex-col gap-3 px-3 pt-3 pb-8">{children}</div>
      </main>

      <SideDrawer
        open={menuOpen}
        onClose={() => setMenuOpen(false)}
        user={user}
        onLogout={async () => {
          await logout()
          navigate('/login', { replace: true })
        }}
      >
        <AccountNav current={path} />
      </SideDrawer>
    </div>
  )
}

/** Signed chip amount for display: "+1,250" / "−400" / "0". */
export function formatNet(n: number): string {
  if (n === 0) return '0'
  return `${n > 0 ? '+' : '−'}${Math.abs(n).toLocaleString()}`
}

/** A signed result: the sign carries the meaning, the colour only backs it up. */
export function NetValue({ net, className = '' }: { net: number; className?: string }) {
  const tone = net > 0 ? 'text-emerald-400' : net < 0 ? 'text-red-400' : 'text-muted'
  return <span className={`${tone} ${className}`}>{formatNet(net)}</span>
}
