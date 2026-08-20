import { useCallback, useEffect, useRef, useState } from 'react'
import { Link, useNavigate } from 'react-router-dom'
import { api } from '../lib/api'
import type { TransactionPage } from '../lib/types'
import { useAuth } from '../lib/auth'
import { useToast } from '../components/Toast'
import { DrawerLink, SideDrawer } from '../components/SideDrawer'
import { Sheet } from '../components/Sheet'
import { TransactionLedger } from '../components/TransactionLedger'

const PRESETS = [50, 100, 250, 500, 1000, 2000]

export default function Wallet() {
  const { user, refresh, logout } = useAuth()
  const navigate = useNavigate()
  const toast = useToast()

  const [ledger, setLedger] = useState<TransactionPage | null>(null)
  const [loading, setLoading] = useState(true)
  const [busy, setBusy] = useState(false)
  const [menuOpen, setMenuOpen] = useState(false)
  const [depositOpen, setDepositOpen] = useState(false)

  // The ledger measures how many rows fit and asks for exactly that many, so
  // this screen owns the viewport the same way the lobby and table do.
  const pageSize = useRef(0)

  useEffect(() => {
    const previous = document.body.style.overflow
    document.body.style.overflow = 'hidden'
    return () => {
      document.body.style.overflow = previous
    }
  }, [])

  const load = useCallback(
    async (next: number) => {
      if (pageSize.current === 0) return
      setLoading(true)
      try {
        // The response reports the page the server actually served — it clamps
        // out-of-range requests — so the controls follow it, not our guess.
        setLedger(await api.wallet.transactions(next, pageSize.current))
      } catch (err) {
        toast(err instanceof Error ? err.message : 'Could not load transactions.', 'error')
      } finally {
        setLoading(false)
      }
    },
    [toast],
  )

  const onPageSizeChange = useCallback(
    (size: number) => {
      if (size === pageSize.current) return
      pageSize.current = size
      // A resize changes how much fits, so restart from the top rather than
      // landing on a page number that meant something else.
      void load(1)
    },
    [load],
  )

  async function deposit(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault()
    const amount = Number(new FormData(event.currentTarget).get('amount'))
    if (!Number.isFinite(amount) || amount <= 0) return
    setBusy(true)
    try {
      await api.wallet.deposit(amount)
      toast(`Deposited ${amount.toLocaleString()} ETB.`, 'success')
      setDepositOpen(false)
      // A deposit is the newest row, so jump back to the top of the ledger.
      await Promise.all([refresh(), load(1)])
    } catch (err) {
      toast(err instanceof Error ? err.message : 'Deposit failed.', 'error')
    } finally {
      setBusy(false)
    }
  }

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
            <span className="font-heading text-xs tracking-[0.2em] text-muted">WALLET</span>
          </div>

          <Link to="/lobby" className="btn-ghost px-3 py-1.5 text-xs">
            Lobby
          </Link>
        </div>
      </header>

      <main className="mx-auto flex min-h-0 w-full max-w-2xl flex-1 flex-col gap-2 px-3 pt-2 pb-3">
        <section className="card flex shrink-0 items-end justify-between gap-3 px-4 py-3">
          <div>
            <span className="text-[10px] tracking-wide text-muted uppercase">Balance</span>
            <p className="flex items-baseline gap-1.5">
              <span className="font-heading text-3xl font-extrabold text-gold-light tabular-nums">
                {(user?.wallet ?? 0).toLocaleString()}
              </span>
              <span className="text-xs font-semibold text-muted">ETB</span>
            </p>
          </div>

          <div className="text-right">
            <span className="text-[10px] tracking-wide text-muted uppercase">Referral</span>
            <p className="font-mono text-sm font-semibold text-gold-light">
              {user?.referral_code}
            </p>
          </div>
        </section>

        <section className="card flex min-h-0 flex-1 flex-col overflow-hidden">
          <h2 className="shrink-0 border-b border-white/8 px-3 py-2 text-[11px] font-semibold tracking-wide text-muted uppercase">
            Transactions
          </h2>
          <TransactionLedger
            page={ledger}
            loading={loading}
            onPageChange={(next) => void load(next)}
            onPageSizeChange={onPageSizeChange}
          />
        </section>

        <button
          type="button"
          onClick={() => setDepositOpen(true)}
          className="btn-gold w-full shrink-0"
        >
          Deposit
        </button>
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
        <nav className="flex flex-col gap-1.5">
          <DrawerLink to="/lobby" label="Lobby" />
        </nav>
      </SideDrawer>

      <Sheet open={depositOpen} title="Deposit ETB" onClose={() => setDepositOpen(false)}>
        <DepositForm busy={busy} onSubmit={deposit} />
      </Sheet>
    </div>
  )
}

function DepositForm({
  busy,
  onSubmit,
}: {
  busy: boolean
  onSubmit: (event: React.FormEvent<HTMLFormElement>) => void
}) {
  const [amount, setAmount] = useState(100)

  return (
    <form onSubmit={onSubmit} className="flex flex-col gap-3.5">
      <div className="grid grid-cols-3 gap-1.5">
        {PRESETS.map((preset) => (
          <button
            key={preset}
            type="button"
            onClick={() => setAmount(preset)}
            className={`rounded-lg border px-1 py-2 text-[11px] font-semibold tabular-nums transition-colors ${
              amount === preset
                ? 'border-gold/60 bg-gold/15 text-gold-light'
                : 'border-white/10 bg-white/5 text-muted hover:border-gold/30 hover:text-chalk'
            }`}
          >
            {preset.toLocaleString()}
          </button>
        ))}
      </div>

      <div>
        <label className="label" htmlFor="amount">
          Amount (ETB)
        </label>
        <input
          id="amount"
          name="amount"
          type="number"
          min={1}
          max={100000}
          step={1}
          required
          value={amount}
          onChange={(e) => setAmount(Number(e.target.value))}
          className="field tabular-nums"
        />
      </div>

      <p className="text-[11px] leading-relaxed text-muted">
        Funds convert to chips 1:1 when you take a seat, and settle back to your balance when you
        leave the table.
      </p>

      <button type="submit" className="btn-gold w-full" disabled={busy}>
        {busy ? 'Processing…' : 'Confirm deposit'}
      </button>
    </form>
  )
}
