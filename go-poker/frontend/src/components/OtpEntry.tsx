import { useEffect, useRef, useState } from 'react'
import { api } from '../lib/api'
import { ErrorNote } from './AuthShell'

/** Seconds the server makes a caller wait between sends. Mirrored here so the
 *  normal path never produces an error at all — the button is simply disabled
 *  until the cooldown is up. The server enforces it regardless; this is only
 *  the courtesy layer, and it does not survive a refresh. */
const RESEND_COOLDOWN_SECONDS = 60

/** Code entry shared by both flows: six digits, a submit, and a resend that
 *  mints a fresh code (the newest SMS is always the one that works). */
export function OtpEntry({
  phone,
  purpose,
  submitLabel,
  busyLabel,
  onSubmit,
}: {
  phone: string
  purpose: 'register' | 'reset'
  submitLabel: string
  busyLabel: string
  onSubmit: (code: string) => Promise<void>
}) {
  const [code, setCode] = useState('')
  const [error, setError] = useState('')
  const [notice, setNotice] = useState('')
  const [busy, setBusy] = useState(false)
  const [cooldown, setCooldown] = useState(RESEND_COOLDOWN_SECONDS)
  const timer = useRef<number | undefined>(undefined)

  useEffect(() => {
    timer.current = window.setInterval(() => {
      setCooldown((seconds) => (seconds > 0 ? seconds - 1 : 0))
    }, 1000)
    return () => window.clearInterval(timer.current)
  }, [])

  async function submit(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault()
    setError('')
    setNotice('')
    setBusy(true)
    try {
      await onSubmit(code.trim())
    } catch (err) {
      setError(err instanceof Error ? err.message : 'That did not work. Try again.')
    } finally {
      setBusy(false)
    }
  }

  async function resend() {
    setError('')
    setNotice('')
    setBusy(true)
    try {
      await api.auth.resendOtp(phone, purpose)
      setCode('')
      setCooldown(RESEND_COOLDOWN_SECONDS)
      setNotice('A new code is on its way. The previous one no longer works.')
    } catch (err) {
      // A 429 lands here when the client-side countdown was lost to a refresh;
      // the server's message carries the real wait.
      setError(err instanceof Error ? err.message : 'Could not send another code.')
    } finally {
      setBusy(false)
    }
  }

  return (
    <form onSubmit={submit} className="flex flex-col gap-4">
      <ErrorNote message={error} />
      {notice && (
        <div className="rounded-xl border border-gold/30 bg-gold/10 px-3.5 py-2.5 text-sm text-gold-light">
          {notice}
        </div>
      )}

      <div>
        <label className="label" htmlFor="code">
          6-Digit Code
        </label>
        <input
          id="code"
          name="code"
          value={code}
          onChange={(event) => setCode(event.target.value.replace(/\D/g, '').slice(0, 6))}
          required
          inputMode="numeric"
          autoComplete="one-time-code"
          placeholder="000000"
          className="field text-center text-2xl tracking-[0.4em]"
        />
        <p className="mt-2 text-xs text-muted">
          Sent to {phone}. It expires in 5 minutes.
        </p>
      </div>

      <button type="submit" className="btn-primary mt-1 w-full" disabled={busy || code.length < 6}>
        {busy ? busyLabel : submitLabel}
      </button>

      <button
        type="button"
        onClick={resend}
        disabled={busy || cooldown > 0}
        className="text-sm font-semibold text-gold-light hover:underline disabled:text-muted disabled:no-underline"
      >
        {cooldown > 0 ? `Resend code in ${cooldown}s` : 'Send a new code'}
      </button>
    </form>
  )
}
