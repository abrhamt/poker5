import { useState } from 'react'
import { Link, useNavigate } from 'react-router-dom'
import { api } from '../lib/api'
import { otpFlow } from '../lib/otpFlow'
import { AuthShell, ErrorNote } from '../components/AuthShell'

/** Step one of a reset: prove which account, and get a code sent to its phone. */
export default function ForgotPassword() {
  const navigate = useNavigate()
  const [error, setError] = useState('')
  const [busy, setBusy] = useState(false)

  async function onSubmit(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault()
    const form = new FormData(event.currentTarget)
    setError('')
    setBusy(true)
    try {
      const { phone_number } = await api.auth.forgotPassword(String(form.get('phone_number')))
      otpFlow.setPhone(phone_number)
      otpFlow.setResetToken('')
      navigate('/reset-password', { state: { phone: phone_number } })
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Could not start a reset.')
    } finally {
      setBusy(false)
    }
  }

  return (
    <AuthShell
      title="Reset Password"
      subtitle="We'll text a code to the number on your account"
    >
      <form onSubmit={onSubmit} className="flex flex-col gap-4">
        <ErrorNote message={error} />

        <div>
          <label className="label" htmlFor="phone_number">
            Phone Number
          </label>
          <input
            id="phone_number"
            name="phone_number"
            type="tel"
            required
            autoComplete="tel"
            placeholder="0911223344 or +251911223344"
            className="field"
          />
        </div>

        <button type="submit" className="btn-primary mt-1 w-full" disabled={busy}>
          {busy ? 'Sending code…' : 'Send Code'}
        </button>
      </form>

      <p className="mt-6 text-center text-sm text-muted">
        Remembered it?{' '}
        <Link to="/login" className="font-semibold text-gold-light hover:underline">
          Sign in
        </Link>
      </p>
    </AuthShell>
  )
}
