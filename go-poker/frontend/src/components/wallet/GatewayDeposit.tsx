import { useState } from 'react'
import type { DepositInfo, DepositMethod } from '../../lib/types'

const GATEWAY_PRESETS = [10, 50, 100, 250, 500, 1000]

/** Offered when both deposit methods are on. Each card is one tap: there is
 *  nothing to configure before choosing, so a list of radios would only add a
 *  confirm step. */
export function DepositMethodChooser({ onPick }: { onPick: (method: DepositMethod) => void }) {
  return (
    <div className="flex flex-col gap-2.5">
      <p className="text-[11px] leading-relaxed text-muted">How would you like to add funds?</p>

      <button
        type="button"
        onClick={() => onPick('gateway')}
        className="flex items-center gap-3 rounded-xl border border-gold/40 bg-gold/10 p-3 text-left transition-colors active:scale-[0.99] hover:border-gold/70"
      >
        <span className="grid size-10 shrink-0 place-items-center rounded-lg bg-gold/20 text-gold-light">
          <svg viewBox="0 0 24 24" width="20" height="20" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
            <path d="M13 2 3 14h9l-1 8 10-12h-9l1-8z" />
          </svg>
        </span>
        <span className="min-w-0">
          <span className="block text-sm font-semibold text-chalk">Automatic deposit</span>
          <span className="block text-[11px] leading-snug text-muted">
            Pay online with your bank or mobile money. Credited as soon as the payment goes through.
          </span>
        </span>
      </button>

      <button
        type="button"
        onClick={() => onPick('receipt')}
        className="flex items-center gap-3 rounded-xl border border-white/12 bg-white/5 p-3 text-left transition-colors active:scale-[0.99] hover:border-gold/30"
      >
        <span className="grid size-10 shrink-0 place-items-center rounded-lg bg-white/10 text-chalk">
          <svg viewBox="0 0 24 24" width="20" height="20" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
            <path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z" />
            <polyline points="14 2 14 8 20 8" />
            <line x1="8" y1="13" x2="16" y2="13" />
            <line x1="8" y1="17" x2="16" y2="17" />
          </svg>
        </span>
        <span className="min-w-0">
          <span className="block text-sm font-semibold text-chalk">Send receipt</span>
          <span className="block text-[11px] leading-snug text-muted">
            Transfer to our CBE account, then paste the confirmation SMS.
          </span>
        </span>
      </button>
    </div>
  )
}

/** The automatic deposit form. Submitting hands the browser to the gateway's
 *  checkout page; the wallet is credited when the gateway reports back, not
 *  when the player returns. */
export function GatewayDepositForm({
  busy,
  info,
  onSubmit,
}: {
  busy: boolean
  info: DepositInfo
  onSubmit: (amount: number) => void | Promise<void>
}) {
  const min = info.gateway_min_amount ?? 10
  const max = info.gateway_max_amount ?? 100000
  const [amount, setAmount] = useState(100)

  const valid = Number.isInteger(amount) && amount >= min && amount <= max

  return (
    <form
      onSubmit={(event) => {
        event.preventDefault()
        if (valid) void onSubmit(amount)
      }}
      className="flex flex-col gap-3.5"
    >
      <div className="grid grid-cols-3 gap-1.5">
        {GATEWAY_PRESETS.map((preset) => (
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
        <label className="label" htmlFor="gateway-amount">
          Amount (ETB)
        </label>
        <input
          id="gateway-amount"
          name="amount"
          type="number"
          inputMode="numeric"
          min={min}
          max={max}
          step={1}
          required
          value={amount}
          onChange={(e) => setAmount(Number(e.target.value))}
          className="field tabular-nums"
        />
        {!valid && (
          <p className="mt-1 text-[11px] text-red-400">
            Enter a whole amount between {min.toLocaleString()} and {max.toLocaleString()} ETB.
          </p>
        )}
      </div>

      <p className="text-[11px] leading-relaxed text-muted">
        You will be taken to a secure payment page. Once the payment goes through you are brought
        back here and the amount is added to your balance automatically.
      </p>

      <button type="submit" className="btn-gold w-full" disabled={busy || !valid}>
        {busy ? 'Opening payment page…' : `Pay ${valid ? amount.toLocaleString() : ''} ETB`}
      </button>
    </form>
  )
}

/** Shown while a returning payment is still being confirmed. */
export function GatewayReturnBanner({ amount }: { amount: number | null }) {
  return (
    <div className="card flex shrink-0 items-center gap-3 px-4 py-3">
      <span className="size-4 shrink-0 animate-spin rounded-full border-2 border-gold/30 border-t-gold-light" />
      <div className="min-w-0">
        <p className="text-sm font-semibold text-chalk">Confirming your payment…</p>
        <p className="text-[11px] text-muted">
          {amount ? `${amount.toLocaleString()} ETB will appear in your balance shortly.` : 'This usually takes a few seconds.'}
        </p>
      </div>
    </div>
  )
}
