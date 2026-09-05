import { useState } from 'react'
import { Link, useNavigate } from 'react-router-dom'
import { useAuth } from '../lib/auth'
import { otpFlow } from '../lib/otpFlow'
import { AuthShell, ErrorNote, PasswordField } from '../components/AuthShell'

export default function Register() {
  const { register } = useAuth()
  const navigate = useNavigate()
  const [error, setError] = useState('')
  const [busy, setBusy] = useState(false)

  async function onSubmit(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault()
    const form = new FormData(event.currentTarget)
    setError('')
    setBusy(true)
    try {
      // No account exists yet — this only sends a code. The account is created
      // when the code is verified on the next screen.
      const phone = await register({
        username: String(form.get('username')),
        phone_number: String(form.get('phone_number')),
        password: String(form.get('password')),
        referral_code: String(form.get('referral_code') ?? '') || undefined,
      })
      otpFlow.setPhone(phone)
      navigate('/verify-phone', { state: { phone } })
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Registration failed.')
    } finally {
      setBusy(false)
    }
  }

  return (
    <AuthShell
      title="Create Account"
      subtitle="Join the premier Texas Hold'em platform in Ethiopia"
    >
      <form onSubmit={onSubmit} className="flex flex-col gap-4">
        <ErrorNote message={error} />

        <div>
          <label className="label" htmlFor="username">
            Username
          </label>
          <input
            id="username"
            name="username"
            required
            autoComplete="username"
            placeholder="Choose a player name"
            className="field"
          />
        </div>

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

        <PasswordField id="password" label="Password" autoComplete="new-password" />

        <div>
          <label className="label" htmlFor="referral_code">
            Referral Code (Optional)
          </label>
          <input
            id="referral_code"
            name="referral_code"
            placeholder="8-character code"
            className="field"
          />
        </div>

        <button type="submit" className="btn-primary mt-1 w-full" disabled={busy}>
          {busy ? 'Sending code…' : 'Create Account'}
        </button>
      </form>

      <p className="mt-6 text-center text-sm text-muted">
        Already have an account?{' '}
        <Link to="/login" className="font-semibold text-gold-light hover:underline">
          Sign in
        </Link>
      </p>
    </AuthShell>
  )
}
