import { useCallback, useEffect, useRef, useState } from 'react'
import type { ReactNode } from 'react'

// --- Data -----------------------------------------------------------------

/** Loads whatever `fetcher` returns and re-loads when `deps` change. Stale
 *  responses are dropped, so typing quickly into a search box cannot leave an
 *  older result on screen after a newer one. */
export function useAsync<T>(fetcher: () => Promise<T>, deps: unknown[]) {
  const [data, setData] = useState<T | null>(null)
  const [error, setError] = useState('')
  const [loading, setLoading] = useState(true)
  const seq = useRef(0)

  const run = useCallback(fetcher, deps)

  const reload = useCallback(async () => {
    const id = ++seq.current
    setLoading(true)
    try {
      const result = await run()
      if (id === seq.current) {
        setData(result)
        setError('')
      }
    } catch (err) {
      if (id === seq.current) setError(err instanceof Error ? err.message : 'Request failed.')
    } finally {
      if (id === seq.current) setLoading(false)
    }
  }, [run])

  useEffect(() => {
    void reload()
  }, [reload])

  return { data, error, loading, reload }
}

/** Holds back a value until it has stopped changing for `ms`. */
export function useDebounced<T>(value: T, ms = 300): T {
  const [debounced, setDebounced] = useState(value)
  useEffect(() => {
    const id = window.setTimeout(() => setDebounced(value), ms)
    return () => window.clearTimeout(id)
  }, [value, ms])
  return debounced
}

// --- Formatting -----------------------------------------------------------

export const etb = (amount: number) => `${amount.toLocaleString()} ETB`

export function signedEtb(amount: number) {
  return `${amount > 0 ? '+' : amount < 0 ? '−' : ''}${Math.abs(amount).toLocaleString()}`
}

export function formatDate(iso: string | null | undefined, withTime = true) {
  if (!iso) return '—'
  return new Date(iso).toLocaleString(undefined, {
    year: 'numeric',
    month: 'short',
    day: 'numeric',
    ...(withTime ? { hour: '2-digit', minute: '2-digit' } : {}),
  })
}

export function timeAgo(iso: string) {
  const seconds = Math.max(0, (Date.now() - new Date(iso).getTime()) / 1000)
  if (seconds < 60) return 'just now'
  const minutes = Math.floor(seconds / 60)
  if (minutes < 60) return `${minutes}m ago`
  const hours = Math.floor(minutes / 60)
  if (hours < 24) return `${hours}h ago`
  const days = Math.floor(hours / 24)
  return `${days}d ago`
}

// --- Layout pieces --------------------------------------------------------

export function PageHeader({ title, description, actions }: { title: string; description?: ReactNode; actions?: ReactNode }) {
  return (
    <div className="mb-5 flex flex-wrap items-end justify-between gap-3">
      <div className="min-w-0">
        <h1 className="text-2xl font-bold tracking-tight text-chalk">{title}</h1>
        {description && <p className="mt-1 max-w-2xl text-sm leading-relaxed text-muted">{description}</p>}
      </div>
      {actions && <div className="flex items-center gap-2">{actions}</div>}
    </div>
  )
}

export function Panel({ title, actions, children, className = '' }: { title?: ReactNode; actions?: ReactNode; children: ReactNode; className?: string }) {
  return (
    <section className={`card overflow-hidden ${className}`}>
      {(title || actions) && (
        <div className="flex items-center justify-between gap-3 border-b border-white/8 px-4 py-3">
          {title && <h2 className="text-sm font-semibold tracking-tight text-chalk">{title}</h2>}
          {actions}
        </div>
      )}
      {children}
    </section>
  )
}

export function StatCard({ label, value, hint, tone = 'default' }: { label: string; value: ReactNode; hint?: ReactNode; tone?: 'default' | 'gold' | 'muted' }) {
  const toneClass =
    tone === 'gold' ? 'border-gold/40 bg-gold/10' : tone === 'muted' ? 'border-white/8 bg-white/3' : 'border-white/10 bg-panel/60'
  return (
    <div className={`rounded-2xl border p-4 ${toneClass}`}>
      <p className="text-[11px] font-semibold tracking-wide text-muted uppercase">{label}</p>
      <p className={`mt-1.5 font-heading text-2xl font-bold tabular-nums ${tone === 'gold' ? 'text-gold-light' : 'text-chalk'}`}>{value}</p>
      {hint && <p className="mt-1 text-xs text-muted">{hint}</p>}
    </div>
  )
}

export function Badge({ children, tone = 'default' }: { children: ReactNode; tone?: 'default' | 'gold' | 'green' | 'red' | 'amber' | 'blue' }) {
  const tones = {
    default: 'border-white/12 bg-white/5 text-muted',
    gold: 'border-gold/40 bg-gold/15 text-gold-light',
    green: 'border-emerald-400/30 bg-emerald-400/10 text-emerald-200',
    red: 'border-red-400/30 bg-red-400/10 text-red-200',
    amber: 'border-amber-400/30 bg-amber-400/10 text-amber-200',
    blue: 'border-sky-400/30 bg-sky-400/10 text-sky-200',
  }
  return (
    <span className={`inline-flex items-center rounded-full border px-2 py-0.5 text-[10px] font-semibold tracking-wide whitespace-nowrap uppercase ${tones[tone]}`}>
      {children}
    </span>
  )
}

export function Pager({ page, pageSize, total, onPage }: { page: number; pageSize: number; total: number; onPage: (page: number) => void }) {
  const pages = Math.max(1, Math.ceil(total / pageSize))
  if (total === 0) return null
  const from = (page - 1) * pageSize + 1
  const to = Math.min(total, page * pageSize)
  return (
    <div className="flex items-center justify-between gap-3 border-t border-white/8 px-4 py-2.5 text-xs text-muted">
      <span className="tabular-nums">
        {from.toLocaleString()}–{to.toLocaleString()} of {total.toLocaleString()}
      </span>
      <div className="flex items-center gap-1.5">
        <button type="button" className="btn-ghost px-3 py-1.5 text-xs" disabled={page <= 1} onClick={() => onPage(page - 1)}>
          Previous
        </button>
        <span className="px-1 tabular-nums">
          {page} / {pages}
        </span>
        <button type="button" className="btn-ghost px-3 py-1.5 text-xs" disabled={page >= pages} onClick={() => onPage(page + 1)}>
          Next
        </button>
      </div>
    </div>
  )
}

export function EmptyState({ title, text }: { title: string; text?: string }) {
  return (
    <div className="flex flex-col items-center gap-1 px-4 py-12 text-center">
      <p className="text-sm font-semibold text-chalk">{title}</p>
      {text && <p className="max-w-sm text-xs text-muted">{text}</p>}
    </div>
  )
}

export function ErrorState({ message, onRetry }: { message: string; onRetry?: () => void }) {
  return (
    <div className="flex flex-col items-center gap-3 px-4 py-10 text-center">
      <p className="text-sm text-red-300">{message}</p>
      {onRetry && (
        <button type="button" className="btn-ghost px-3 py-1.5 text-xs" onClick={onRetry}>
          Try again
        </button>
      )}
    </div>
  )
}

export function LoadingBlock() {
  return (
    <div className="flex justify-center py-12">
      <span className="size-6 animate-spin rounded-full border-2 border-white/10 border-t-gold" />
    </div>
  )
}

export function CopyButton({ value, label = 'Copy' }: { value: string; label?: string }) {
  const [copied, setCopied] = useState(false)
  return (
    <button
      type="button"
      onClick={async () => {
        try {
          await navigator.clipboard.writeText(value)
          setCopied(true)
          window.setTimeout(() => setCopied(false), 1500)
        } catch {
          // Clipboard access is often denied on plain HTTP; the value is on
          // screen to be selected by hand either way.
        }
      }}
      className="rounded-md border border-white/10 bg-white/5 px-1.5 py-0.5 text-[10px] font-semibold text-muted hover:border-gold/40 hover:text-chalk"
    >
      {copied ? 'Copied' : label}
    </button>
  )
}

export function Toggle({ checked, onChange, label, description }: { checked: boolean; onChange: (next: boolean) => void; label: string; description?: ReactNode }) {
  return (
    <label className="flex cursor-pointer items-start justify-between gap-4 py-1">
      <span className="min-w-0">
        <span className="block text-sm font-medium text-chalk">{label}</span>
        {description && <span className="mt-0.5 block text-xs leading-relaxed text-muted">{description}</span>}
      </span>
      <span className="relative mt-0.5 shrink-0">
        <input type="checkbox" className="peer sr-only" checked={checked} onChange={(e) => onChange(e.target.checked)} />
        <span className="block h-6 w-11 rounded-full border border-white/15 bg-white/10 transition-colors peer-checked:border-gold/60 peer-checked:bg-gold/70 peer-focus-visible:ring-2 peer-focus-visible:ring-gold/40" />
        <span className="absolute top-1 left-1 size-4 rounded-full bg-chalk shadow transition-transform peer-checked:translate-x-5" />
      </span>
    </label>
  )
}

/** Tables scroll sideways inside their panel on a phone rather than pushing
 *  the page wider than the screen. */
export function TableWrap({ children }: { children: ReactNode }) {
  return <div className="overflow-x-auto">{children}</div>
}

export const th = 'px-4 py-2.5 text-left text-[11px] font-semibold tracking-wide whitespace-nowrap text-muted uppercase'
export const td = 'px-4 py-2.5 text-sm whitespace-nowrap'
