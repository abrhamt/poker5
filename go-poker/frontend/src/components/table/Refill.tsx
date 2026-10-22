import { useEffect, useState } from 'react'
import { Sheet } from '../Sheet'

/**
 * Brings more of the wallet to the table. A stack cannot change under a hand
 * its owner is still playing, so while that is the case the sheet says so
 * instead of offering a button the server would refuse.
 */
export function RefillSheet({
  open,
  wallet,
  buyIn,
  bigBlind,
  inHand,
  busy,
  onClose,
  onSubmit,
}: {
  open: boolean
  wallet: number
  buyIn: number
  bigBlind: number
  inHand: boolean
  busy: boolean
  onClose: () => void
  onSubmit: (amount: number) => void
}) {
  const [amountText, setAmountText] = useState('')

  // Each opening starts from the table's buy-in, or the whole wallet when
  // that is less.
  useEffect(() => {
    if (open) setAmountText(String(Math.min(buyIn, wallet)))
  }, [open, buyIn, wallet])

  const amount = Number(amountText)
  const min = Math.max(1, bigBlind)
  const error =
    wallet < min
      ? `Your wallet has ${wallet.toLocaleString()} ETB — the smallest refill here is ${min.toLocaleString()} ETB.`
      : !Number.isInteger(amount) || amount < min
        ? `The smallest refill here is ${min.toLocaleString()} ETB.`
        : amount > wallet
          ? `Your wallet has ${wallet.toLocaleString()} ETB.`
          : ''

  const presets = [...new Set([buyIn, wallet].filter((v) => v >= min && v <= wallet))]

  return (
    <Sheet open={open} title="Add chips" onClose={onClose}>
      <form
        noValidate
        className="flex flex-col gap-3.5"
        onSubmit={(event) => {
          event.preventDefault()
          if (!error && !inHand) onSubmit(amount)
        }}
      >
        <p className="text-[12px] text-muted">
          Wallet balance <span className="font-semibold text-gold-light tabular-nums">{wallet.toLocaleString()} ETB</span>
        </p>

        {presets.length > 0 && (
          <div className="grid grid-cols-2 gap-1.5">
            {presets.map((value) => (
              <button
                key={value}
                type="button"
                onClick={() => setAmountText(String(value))}
                className={`rounded-lg border px-2 py-2 text-[11px] font-semibold tabular-nums transition-colors ${
                  amount === value
                    ? 'border-gold/60 bg-gold/15 text-gold-light'
                    : 'border-white/10 bg-white/5 text-muted hover:border-gold/30 hover:text-chalk'
                }`}
              >
                {value === wallet ? `Max · ${value.toLocaleString()}` : `Buy-in · ${value.toLocaleString()}`}
              </button>
            ))}
          </div>
        )}

        <div>
          <label className="label" htmlFor="refill-amount">
            Amount (ETB)
          </label>
          <input
            id="refill-amount"
            type="number"
            inputMode="numeric"
            step={1}
            value={amountText}
            onChange={(e) => setAmountText(e.target.value)}
            className="field tabular-nums"
          />
          {error && amountText !== '' && <p className="mt-1 text-[11px] text-red-400">{error}</p>}
        </div>

        {inHand && (
          <p className="text-[11px] leading-relaxed text-amber-200">
            You're in this hand — you can add chips once it's over for you, or after you fold.
          </p>
        )}

        <button type="submit" className="btn-gold w-full" disabled={busy || inHand || !!error}>
          {busy ? 'Adding…' : `Add ${!error ? amount.toLocaleString() : ''} chips`}
        </button>
      </form>
    </Sheet>
  )
}

/** Shown in place of the action bar when the player has no chips left. Their
 *  seat is kept for a while; this is the one thing they can do with it. */
export function OutOfChips({
  canRefill,
  busy,
  onRefill,
  onLeave,
}: {
  canRefill: boolean
  busy: boolean
  onRefill: () => void
  onLeave: () => void
}) {
  return (
    <div className="absolute inset-x-0 bottom-2 z-50 flex justify-center px-3 sm:bottom-3">
      <div className="animate-rise flex items-center gap-2 rounded-2xl border border-white/12 bg-black/70 py-1.5 pr-1.5 pl-3.5 backdrop-blur-md">
        <span className="text-xs text-white/80">Out of chips</span>
        {canRefill && (
          <button type="button" onClick={onRefill} disabled={busy} className="btn-gold px-3.5 py-1.5 text-xs">
            Refill
          </button>
        )}
        <button type="button" onClick={onLeave} disabled={busy} className="btn-ghost px-3 py-1.5 text-xs">
          Leave
        </button>
      </div>
    </div>
  )
}
