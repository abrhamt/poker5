import { createContext, use, useCallback, useEffect, useState } from 'react'
import type { ReactNode } from 'react'
import { NavLink, Outlet, useLocation, useNavigate } from 'react-router-dom'
import { useAuth } from '../lib/auth'
import { adminApi } from './adminApi'
import type { AdminOverview } from './adminApi'

/** The overview doubles as the source of the nav badges, so any page that
 *  changes a queue calls refresh() and the counts follow. */
const OverviewContext = createContext<{ overview: AdminOverview | null; refresh: () => Promise<void> } | null>(null)

export function useOverview() {
  const ctx = use(OverviewContext)
  if (!ctx) throw new Error('useOverview must be used inside <AdminLayout>')
  return ctx
}

const icon = (path: ReactNode) => (
  <svg viewBox="0 0 24 24" width="18" height="18" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
    {path}
  </svg>
)

const NAV: { to: string; label: string; end?: boolean; badge?: (o: AdminOverview) => number; icon: ReactNode }[] = [
  {
    to: '/admin',
    label: 'Overview',
    end: true,
    icon: icon(
      <>
        <rect x="3" y="3" width="7" height="9" rx="1" />
        <rect x="14" y="3" width="7" height="5" rx="1" />
        <rect x="14" y="12" width="7" height="9" rx="1" />
        <rect x="3" y="16" width="7" height="5" rx="1" />
      </>,
    ),
  },
  {
    to: '/admin/withdrawals',
    label: 'Withdrawals',
    badge: (o) => o.pending_withdrawals,
    icon: icon(
      <>
        <rect x="2" y="6" width="20" height="12" rx="2" />
        <line x1="12" y1="15" x2="12" y2="9" />
        <polyline points="9 12 12 9 15 12" />
      </>,
    ),
  },
  {
    to: '/admin/deposits',
    label: 'Deposits',
    badge: (o) => o.pending_deposits,
    icon: icon(
      <>
        <path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z" />
        <polyline points="14 2 14 8 20 8" />
        <line x1="8" y1="13" x2="16" y2="13" />
        <line x1="8" y1="17" x2="16" y2="17" />
      </>,
    ),
  },
  {
    to: '/admin/users',
    label: 'Users',
    icon: icon(
      <>
        <path d="M17 21v-2a4 4 0 0 0-4-4H5a4 4 0 0 0-4 4v2" />
        <circle cx="9" cy="7" r="4" />
        <path d="M23 21v-2a4 4 0 0 0-3-3.87" />
        <path d="M16 3.13a4 4 0 0 1 0 7.75" />
      </>,
    ),
  },
  {
    to: '/admin/transactions',
    label: 'Transactions',
    icon: icon(
      <>
        <line x1="8" y1="6" x2="21" y2="6" />
        <line x1="8" y1="12" x2="21" y2="12" />
        <line x1="8" y1="18" x2="21" y2="18" />
        <line x1="3" y1="6" x2="3.01" y2="6" />
        <line x1="3" y1="12" x2="3.01" y2="12" />
        <line x1="3" y1="18" x2="3.01" y2="18" />
      </>,
    ),
  },
  {
    to: '/admin/settings',
    label: 'Settings',
    icon: icon(
      <>
        <circle cx="12" cy="12" r="3" />
        <path d="M19.4 15a1.65 1.65 0 0 0 .33 1.82l.06.06a2 2 0 1 1-2.83 2.83l-.06-.06a1.65 1.65 0 0 0-1.82-.33 1.65 1.65 0 0 0-1 1.51V21a2 2 0 1 1-4 0v-.09A1.65 1.65 0 0 0 9 19.4a1.65 1.65 0 0 0-1.82.33l-.06.06a2 2 0 1 1-2.83-2.83l.06-.06a1.65 1.65 0 0 0 .33-1.82 1.65 1.65 0 0 0-1.51-1H3a2 2 0 1 1 0-4h.09A1.65 1.65 0 0 0 4.6 9a1.65 1.65 0 0 0-.33-1.82l-.06-.06a2 2 0 1 1 2.83-2.83l.06.06a1.65 1.65 0 0 0 1.82.33H9a1.65 1.65 0 0 0 1-1.51V3a2 2 0 1 1 4 0v.09a1.65 1.65 0 0 0 1 1.51 1.65 1.65 0 0 0 1.82-.33l.06-.06a2 2 0 1 1 2.83 2.83l-.06.06a1.65 1.65 0 0 0-.33 1.82V9a1.65 1.65 0 0 0 1.51 1H21a2 2 0 1 1 0 4h-.09a1.65 1.65 0 0 0-1.51 1z" />
      </>,
    ),
  },
]

/** How often the badges re-check on their own, so a request that arrives
 *  while an admin sits on one page still shows up in the nav. */
const OVERVIEW_POLL_MS = 30_000

export function AdminLayout() {
  const { user, logout } = useAuth()
  const navigate = useNavigate()
  const location = useLocation()
  const [overview, setOverview] = useState<AdminOverview | null>(null)
  const [menuOpen, setMenuOpen] = useState(false)

  const refresh = useCallback(async () => {
    try {
      setOverview(await adminApi.overview())
    } catch {
      // Badges are a convenience; every page loads its own data.
    }
  }, [])

  useEffect(() => {
    void refresh()
    const id = window.setInterval(() => void refresh(), OVERVIEW_POLL_MS)
    return () => window.clearInterval(id)
  }, [refresh])

  // Close the mobile menu whenever the route changes.
  useEffect(() => setMenuOpen(false), [location.pathname])

  const nav = (
    <nav className="flex flex-col gap-1">
      {NAV.map((item) => {
        const count = overview && item.badge ? item.badge(overview) : 0
        return (
          <NavLink
            key={item.to}
            to={item.to}
            end={item.end}
            className={({ isActive }) =>
              `flex items-center gap-3 rounded-xl px-3 py-2.5 text-sm font-medium transition-colors ${
                isActive ? 'bg-gold/15 text-gold-light' : 'text-muted hover:bg-white/5 hover:text-chalk'
              }`
            }
          >
            {item.icon}
            <span className="flex-1">{item.label}</span>
            {count > 0 && (
              <span className="min-w-5 rounded-full bg-gold px-1.5 py-0.5 text-center text-[10px] font-bold text-ink tabular-nums">
                {count > 99 ? '99+' : count}
              </span>
            )}
          </NavLink>
        )
      })}
    </nav>
  )

  const account = (
    <div className="flex items-center justify-between gap-2 border-t border-white/8 pt-4">
      <div className="flex min-w-0 items-center gap-2">
        <span className="grid size-8 shrink-0 place-items-center rounded-full bg-gold/20 font-heading text-sm font-bold text-gold-light uppercase">
          {user?.username.charAt(0) ?? '?'}
        </span>
        <span className="truncate text-sm font-medium">{user?.username}</span>
      </div>
      <button
        type="button"
        className="btn-ghost px-3 py-1.5 text-xs"
        onClick={async () => {
          await logout()
          navigate('/login', { replace: true })
        }}
      >
        Sign out
      </button>
    </div>
  )

  const brand = (
    <div className="flex items-center gap-2 px-1">
      <span className="grid size-8 place-items-center rounded-lg bg-linear-[135deg,#f6d365,var(--color-gold),#aa771c] font-heading text-sm font-extrabold text-ink">
        G
      </span>
      <div className="leading-tight">
        <p className="font-heading text-sm font-bold text-chalk">Golden Poker</p>
        <p className="text-[10px] tracking-[0.2em] text-muted uppercase">Admin</p>
      </div>
    </div>
  )

  return (
    <OverviewContext value={{ overview, refresh }}>
      <div className="min-h-screen lg:pl-64">
        {/* Desktop sidebar */}
        <aside className="fixed inset-y-0 left-0 hidden w-64 flex-col gap-6 border-r border-gold/15 bg-surface/90 p-4 backdrop-blur-xl lg:flex">
          {brand}
          {nav}
          <div className="mt-auto">{account}</div>
        </aside>

        {/* Mobile top bar */}
        <header className="sticky top-0 z-40 flex items-center justify-between gap-2 border-b border-gold/15 bg-ink/85 px-4 py-2.5 backdrop-blur-xl lg:hidden">
          {brand}
          <button
            type="button"
            onClick={() => setMenuOpen(true)}
            aria-label="Open menu"
            className="relative grid size-9 place-items-center rounded-xl border border-white/12 bg-white/5 text-white/85"
          >
            <svg viewBox="0 0 24 24" width="17" height="17" fill="none" stroke="currentColor" strokeWidth="2.5" strokeLinecap="round">
              <line x1="3" y1="6" x2="21" y2="6" />
              <line x1="3" y1="12" x2="21" y2="12" />
              <line x1="3" y1="18" x2="21" y2="18" />
            </svg>
            {overview && overview.pending_withdrawals + overview.pending_deposits > 0 && (
              <span className="absolute -top-1 -right-1 size-2.5 rounded-full bg-gold" />
            )}
          </button>
        </header>

        {menuOpen && (
          <div className="fixed inset-0 z-50 flex lg:hidden" role="dialog" aria-modal="true">
            <div className="absolute inset-0 bg-ink/70 backdrop-blur-sm" onClick={() => setMenuOpen(false)} />
            <aside className="animate-rise relative flex h-full w-[min(80vw,17rem)] flex-col gap-6 overflow-y-auto border-r border-gold/20 bg-surface p-4">
              {brand}
              {nav}
              <div className="mt-auto">{account}</div>
            </aside>
          </div>
        )}

        <main className="mx-auto w-full max-w-6xl px-4 py-6 lg:px-8 lg:py-8">
          <Outlet />
        </main>
      </div>
    </OverviewContext>
  )
}
