import { useCallback, useEffect, useRef, useState } from 'react'
import { Link, useNavigate } from 'react-router-dom'
import { api } from '../lib/api'
import { extractCbeReceiptUrl } from '../lib/cbeReceipt'
import type { DepositInfo, TransactionPage } from '../lib/types'
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
  const [depositInfo, setDepositInfo] = useState<DepositInfo | null>(null)

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

  // Which deposit form to show is the server's call, and it can change while
  // someone has the app open — so it is read when the sheet opens rather than
  // once at mount.
  useEffect(() => {
    if (!depositOpen) return
    let cancelled = false
    api.wallet
      .depositInfo()
      .then((info) => {
        if (!cancelled) setDepositInfo(info)
      })
      .catch(() => {
        // Leave whatever was last known: a failed config read should not
        // silently downgrade a real-money site to the play-money form.
      })
    return () => {
      cancelled = true
    }
  }, [depositOpen])

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

  async function submitReceipt(message: string) {
    setBusy(true)
    try {
      const outcome = await api.wallet.depositReceipt(message)
      if (outcome.status === 'credited') {
        toast(outcome.message, 'success')
        setDepositOpen(false)
        await Promise.all([refresh(), load(1)])
      } else {
        // Queued for review. The sheet stays closed and the message stays on
        // screen: there is nothing more for the player to do, and re-sending
        // the money is the one thing they must not do.
        toast(outcome.message, 'success')
        setDepositOpen(false)
      }
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
        {depositInfo?.real_deposits_enabled ? (
          <ReceiptDepositForm busy={busy} info={depositInfo} onSubmit={submitReceipt} />
        ) : (
          <DepositForm busy={busy} onSubmit={deposit} />
        )}
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

/** The real-money deposit form: transfer first, then paste the SMS CBE sends
 *  back. The link inside that message is the receipt, and the bank is what
 *  confirms it — so this form's job is to make sure a link is actually in
 *  there before anyone waits on a request. */
function ReceiptDepositForm({
  busy,
  info,
  onSubmit,
}: {
  busy: boolean
  info: DepositInfo
  onSubmit: (message: string) => void | Promise<void>
}) {
  const [message, setMessage] = useState('')
  const [copied, setCopied] = useState<'name' | 'number' | null>(null)

  const trimmed = message.trim()
  const parsed = trimmed === '' ? null : extractCbeReceiptUrl(trimmed)

  async function copy(value: string, which: 'name' | 'number') {
    try {
      await navigator.clipboard.writeText(value)
      setCopied(which)
      window.setTimeout(() => setCopied(null), 1500)
    } catch {
      // Clipboard access is denied often enough (insecure origin, permission
      // prompt) that failing quietly is better than an error toast — the value
      // is on screen to be read either way.
    }
  }

  return (
    <form
      onSubmit={(event) => {
        event.preventDefault()
        if (parsed?.ok) void onSubmit(trimmed)
      }}
      className="flex flex-col gap-3.5"
    >
      <div className="rounded-xl border border-gold/25 bg-gold/5 p-3">
        <p className="text-[10px] tracking-wide text-muted uppercase">Send your transfer to</p>
        <button
          type="button"
          onClick={() => void copy(info.account_number ?? '', 'number')}
          className="mt-1 block w-full text-left font-mono text-lg font-bold tracking-wide text-gold-light tabular-nums"
        >
          {info.account_number}
        </button>
        <button
          type="button"
          onClick={() => void copy(info.account_name ?? '', 'name')}
          className="block w-full text-left text-sm font-semibold text-chalk"
        >
          {info.account_name}
        </button>
        <p className="mt-1 text-[11px] text-muted">
          {copied ? 'Copied.' : 'Tap either line to copy. Commercial Bank of Ethiopia.'}
        </p>
      </div>

      <div>
        <label className="label" htmlFor="receipt">
          Paste the CBE confirmation SMS
        </label>
        <textarea
          id="receipt"
          name="receipt"
          rows={5}
          required
          value={message}
          onChange={(e) => setMessage(e.target.value)}
          placeholder="Dear ... You have successfully transferred ETB50.00 ... https://mbreciept.cbe.com.et/..."
          className="field resize-none text-xs leading-relaxed"
        />
      </div>

      {parsed?.ok === false && <p className="text-[11px] text-red-400">{parsed.reason}</p>}
      {parsed?.ok && (
        <p className="text-[11px] break-all text-emerald-400">Receipt link found: {parsed.url}</p>
      )}

      <p className="text-[11px] leading-relaxed text-muted">
        Paste the whole message — the link at the end is the receipt. We check it with the bank, so
        it only counts once and only if it was sent to the account above.
      </p>

      <button type="submit" className="btn-gold w-full" disabled={busy || !parsed?.ok}>
        {busy ? 'Checking with the bank…' : 'Submit receipt'}
      </button>
    </form>
  )
}
