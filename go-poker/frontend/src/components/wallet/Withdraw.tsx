import { useState } from 'react'
import type { Withdrawal, WithdrawalMethod, WithdrawInfo } from '../../lib/types'

export const METHOD_LABEL: Record<WithdrawalMethod, string> = {
  cbe: 'CBE',
  telebirr: 'Telebirr',
}

const STATUS_STYLE: Record<Withdrawal['status'], { label: string; tone: string }> = {
  pending: { label: 'Pending', tone: 'border-amber-400/30 bg-amber-400/10 text-amber-200' },
  completed: { label: 'Paid', tone: 'border-emerald-400/30 bg-emerald-400/10 text-emerald-200' },
  rejected: { label: 'Rejected', tone: 'border-red-400/30 bg-red-400/10 text-red-200' },
  cancelled: { label: 'Cancelled', tone: 'border-white/15 bg-white/5 text-muted' },
}

export function WithdrawalStatusChip({ status }: { status: Withdrawal['status'] }) {
  const style = STATUS_STYLE[status]
  return (
    <span className={`rounded-full border px-2 py-0.5 text-[10px] font-semibold tracking-wide uppercase ${style.tone}`}>
      {style.label}
    </span>
  )
}

/** Mirrors the server's checks so a typo is caught before a round-trip; the
 *  server re-checks and is the one that decides. */
export function normalizeAccount(method: WithdrawalMethod, raw: string): string | null {
  const cleaned = raw.replace(/[\s\-()]/g, '')
  if (method === 'cbe') return /^\d{13}$/.test(cleaned) ? cleaned : null
  const match = cleaned.match(/^(?:\+251|251|0)?(9\d{8})$/)
  return match ? `0${match[1]}` : null
}

const ACCOUNT_HINT: Record<WithdrawalMethod, { label: string; placeholder: string; error: string; maxLength: number }> = {
  cbe: {
    label: 'CBE account number',
    placeholder: '1000XXXXXXXXX',
    error: 'A CBE account number is 13 digits.',
    maxLength: 17,
  },
  telebirr: {
    label: 'Telebirr phone number',
    placeholder: '09XXXXXXXX',
    error: 'Enter the Telebirr number as 09XXXXXXXX.',
    maxLength: 16,
  },
}

/**
 * Everything the withdraw sheet shows, decided by the server's view of this
 * player: closed, paused after a password reset, already waiting on a request,
 * or open for a new one.
 */
export function WithdrawPanel({
  info,
  balance,
  busy,
  onSubmit,
  onCancel,
}: {
  info: WithdrawInfo | null
  balance: number
  busy: boolean
  onSubmit: (input: { method: WithdrawalMethod; account_number: string; account_name: string; amount: number }) => void
  onCancel: (withdrawal: Withdrawal) => void
}) {
  if (!info) {
    return (
      <div className="flex justify-center py-8">
        <span className="size-6 animate-spin rounded-full border-2 border-white/10 border-t-gold" />
      </div>
    )
  }

  const pending = info.withdrawals.find((w) => w.status === 'pending')
  const history = info.withdrawals.filter((w) => w !== pending)

  let body
  if (pending) {
    body = <PendingWithdrawal withdrawal={pending} canCancel={info.cancel_enabled} busy={busy} onCancel={onCancel} />
  } else if (!info.enabled) {
    body = <Notice title="Withdrawals are closed" text="Withdrawals are not available right now. Please check back later." />
  } else if (info.cooldown_until) {
    body = (
      <Notice
        title="Withdrawals paused"
        text={`For your security, withdrawals are paused for a while after a password reset. You can withdraw again after ${formatDateTime(info.cooldown_until)}.`}
      />
    )
  } else {
    body = <WithdrawForm info={info} balance={balance} busy={busy} onSubmit={onSubmit} />
  }

  return (
    <div className="flex flex-col gap-4">
      {body}
      {history.length > 0 && (
        <div>
          <h3 className="mb-1.5 text-[10px] font-semibold tracking-wide text-muted uppercase">Recent requests</h3>
          <ul className="flex flex-col divide-y divide-white/6 rounded-xl border border-white/8 bg-black/20">
            {history.map((w) => (
              <li key={w.id} className="flex flex-col gap-0.5 px-3 py-2">
                <div className="flex items-center justify-between gap-2">
                  <span className="text-sm font-semibold tabular-nums">{w.amount.toLocaleString()} ETB</span>
                  <WithdrawalStatusChip status={w.status} />
                </div>
                <span className="text-[11px] text-muted">
                  {METHOD_LABEL[w.method]} · {w.account_number} · {formatDateTime(w.created_at)}
                </span>
                {w.status === 'rejected' && w.note && (
                  <span className="text-[11px] text-red-300">Reason: {w.note}</span>
                )}
              </li>
            ))}
          </ul>
        </div>
      )}
    </div>
  )
}

function Notice({ title, text }: { title: string; text: string }) {
  return (
    <div className="rounded-xl border border-white/10 bg-white/5 p-3">
      <p className="text-sm font-semibold text-chalk">{title}</p>
      <p className="mt-1 text-[12px] leading-relaxed text-muted">{text}</p>
    </div>
  )
}

function PendingWithdrawal({
  withdrawal,
  canCancel,
  busy,
  onCancel,
}: {
  withdrawal: Withdrawal
  canCancel: boolean
  busy: boolean
  onCancel: (withdrawal: Withdrawal) => void
}) {
  return (
    <div className="flex flex-col gap-3">
      <div className="rounded-xl border border-amber-400/25 bg-amber-400/5 p-3">
        <div className="flex items-center justify-between gap-2">
          <span className="font-heading text-2xl font-extrabold text-gold-light tabular-nums">
            {withdrawal.amount.toLocaleString()} <span className="text-xs font-semibold text-muted">ETB</span>
          </span>
          <WithdrawalStatusChip status="pending" />
        </div>
        <dl className="mt-2 grid grid-cols-[auto_1fr] gap-x-3 gap-y-0.5 text-[12px]">
          <dt className="text-muted">To</dt>
          <dd className="text-chalk">{METHOD_LABEL[withdrawal.method]}</dd>
          <dt className="text-muted">Account</dt>
          <dd className="font-mono text-chalk">{withdrawal.account_number}</dd>
          <dt className="text-muted">Name</dt>
          <dd className="text-chalk">{withdrawal.account_name}</dd>
          <dt className="text-muted">Requested</dt>
          <dd className="text-chalk">{formatDateTime(withdrawal.created_at)}</dd>
        </dl>
      </div>
      <p className="text-[11px] leading-relaxed text-muted">
        The amount has been set aside from your balance and will be sent to the account above.
        {canCancel ? ' Changed your mind? Cancel it and the amount returns to your balance.' : ''}
      </p>
      {canCancel && (
        <button type="button" className="btn-ghost w-full" disabled={busy} onClick={() => onCancel(withdrawal)}>
          {busy ? 'Cancelling…' : 'Cancel request'}
        </button>
      )}
    </div>
  )
}

function WithdrawForm({
  info,
  balance,
  busy,
  onSubmit,
}: {
  info: WithdrawInfo
  balance: number
  busy: boolean
  onSubmit: (input: { method: WithdrawalMethod; account_number: string; account_name: string; amount: number }) => void
}) {
  const methods = info.methods.length > 0 ? info.methods : (['cbe', 'telebirr'] as WithdrawalMethod[])
  const [method, setMethod] = useState<WithdrawalMethod>(methods[0])
  const [account, setAccount] = useState('')
  const [name, setName] = useState('')
  const [amountText, setAmountText] = useState('')
  const [touched, setTouched] = useState(false)

  const hint = ACCOUNT_HINT[method]
  const normalized = normalizeAccount(method, account)
  const amount = Number(amountText)
  const min = info.min_amount

  const amountError =
    amountText === ''
      ? 'Enter an amount.'
      : !Number.isInteger(amount) || amount <= 0
        ? 'Enter a whole amount in ETB.'
        : amount < min
          ? `The minimum withdrawal is ${min.toLocaleString()} ETB.`
          : amount > balance
            ? `You have ${balance.toLocaleString()} ETB available.`
            : ''
  const accountError = normalized ? '' : hint.error
  const nameError = name.trim() ? '' : 'Enter the name on the account.'
  const valid = !amountError && !accountError && !nameError
  const notEnough = balance < min

  return (
    // noValidate: the browser's own bubbles would block submit before these
    // messages, which say what the rule actually is, ever appear.
    <form
      noValidate
      onSubmit={(event) => {
        event.preventDefault()
        setTouched(true)
        if (valid && normalized) {
          onSubmit({ method, account_number: normalized, account_name: name.trim(), amount })
        }
      }}
      className="flex flex-col gap-3.5"
    >
      <div className="grid grid-cols-2 gap-1.5" role="radiogroup" aria-label="Payout method">
        {methods.map((m) => (
          <button
            key={m}
            type="button"
            role="radio"
            aria-checked={method === m}
            onClick={() => setMethod(m)}
            className={`rounded-xl border px-3 py-2.5 text-sm font-semibold transition-colors ${
              method === m
                ? 'border-gold/60 bg-gold/15 text-gold-light'
                : 'border-white/10 bg-white/5 text-muted hover:border-gold/30 hover:text-chalk'
            }`}
          >
            {METHOD_LABEL[m]}
          </button>
        ))}
      </div>

      <div>
        <label className="label" htmlFor="withdraw-account">
          {hint.label}
        </label>
        <input
          id="withdraw-account"
          inputMode={method === 'cbe' ? 'numeric' : 'tel'}
          autoComplete="off"
          maxLength={hint.maxLength}
          value={account}
          onChange={(e) => setAccount(e.target.value)}
          placeholder={hint.placeholder}
          className="field font-mono tabular-nums"
        />
        {touched && accountError && <p className="mt-1 text-[11px] text-red-400">{accountError}</p>}
      </div>

      <div>
        <label className="label" htmlFor="withdraw-name">
          Account holder name
        </label>
        <input
          id="withdraw-name"
          autoComplete="name"
          maxLength={128}
          value={name}
          onChange={(e) => setName(e.target.value)}
          placeholder="As registered on the account"
          className="field"
        />
        {touched && nameError && <p className="mt-1 text-[11px] text-red-400">{nameError}</p>}
      </div>

      <div>
        <div className="mb-1.5 flex items-baseline justify-between">
          <label className="label mb-0" htmlFor="withdraw-amount">
            Amount (ETB)
          </label>
          <button
            type="button"
            onClick={() => setAmountText(String(balance))}
            className="text-[11px] font-semibold text-gold-light hover:underline"
          >
            Max {balance.toLocaleString()}
          </button>
        </div>
        <input
          id="withdraw-amount"
          type="number"
          inputMode="numeric"
          min={min}
          max={balance}
          step={1}
          value={amountText}
          onChange={(e) => setAmountText(e.target.value)}
          placeholder={`At least ${min.toLocaleString()}`}
          className="field tabular-nums"
        />
        {touched && amountError && <p className="mt-1 text-[11px] text-red-400">{amountError}</p>}
      </div>

      <p className="text-[11px] leading-relaxed text-muted">
        {notEnough
          ? `You need at least ${min.toLocaleString()} ETB in your balance to withdraw. Chips at a table count once you leave it.`
          : 'The amount leaves your balance now and is sent to you once the request is processed. Check the account details — payouts go exactly where you tell us.'}
      </p>

      <button type="submit" className="btn-gold w-full" disabled={busy || notEnough}>
        {busy ? 'Submitting…' : valid ? `Withdraw ${amount.toLocaleString()} ETB` : 'Withdraw'}
      </button>
    </form>
  )
}

/** A one-line reminder on the wallet screen while a request is waiting. */
export function PendingWithdrawalBanner({ withdrawal, onOpen }: { withdrawal: Withdrawal; onOpen: () => void }) {
  return (
    <button
      type="button"
      onClick={onOpen}
      className="card flex w-full shrink-0 items-center justify-between gap-3 px-4 py-2.5 text-left"
    >
      <div className="min-w-0">
        <p className="text-sm font-semibold text-chalk">
          Withdrawal of {withdrawal.amount.toLocaleString()} ETB pending
        </p>
        <p className="truncate text-[11px] text-muted">
          To {METHOD_LABEL[withdrawal.method]} {withdrawal.account_number}
        </p>
      </div>
      <WithdrawalStatusChip status="pending" />
    </button>
  )
}

export function formatDateTime(iso: string): string {
  return new Date(iso).toLocaleString(undefined, {
    month: 'short',
    day: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
  })
}
