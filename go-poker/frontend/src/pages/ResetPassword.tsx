import { useEffect, useState } from 'react'
import { Link, useLocation, useNavigate } from 'react-router-dom'
import { api } from '../lib/api'
import { useAuth } from '../lib/auth'
import { otpFlow } from '../lib/otpFlow'
import { AuthShell, ErrorNote, PasswordField } from '../components/AuthShell'
import { OtpEntry } from '../components/OtpEntry'

/** Steps two and three of a reset. Verifying the code exchanges it for a token
 *  with its own, longer clock, so taking a minute to choose a password can't
 *  expire the code out from under someone who has already proved they hold the
 *  phone. Both the phone and the token survive a refresh (sessionStorage), so a
 *  reload doesn't cost another SMS. */
export default function ResetPassword() {
  const navigate = useNavigate()
  const location = useLocation()
  const { refresh } = useAuth()

  const [phone] = useState(
    () => (location.state as { phone?: string } | null)?.phone || otpFlow.phone(),
  )
  const [token, setToken] = useState(() => otpFlow.resetToken())
  const [error, setError] = useState('')
  const [busy, setBusy] = useState(false)

  useEffect(() => {
    if (!phone) navigate('/forgot-password', { replace: true })
  }, [phone, navigate])

  if (!phone) return null

  if (!token) {
    return (
      <AuthShell title="Reset Password" subtitle="Enter the code we sent by SMS">
        <OtpEntry
          phone={phone}
          purpose="reset"
          submitLabel="Verify Code"
          busyLabel="Verifying…"
          onSubmit={async (code) => {
            const { reset_token } = await api.auth.verifyResetOtp(phone, code)
            otpFlow.setResetToken(reset_token)
            setToken(reset_token)
          }}
        />

        <p className="mt-6 text-center text-sm text-muted">
          Wrong number?{' '}
          <Link to="/forgot-password" className="font-semibold text-gold-light hover:underline">
            Start over
          </Link>
        </p>
      </AuthShell>
    )
  }

  async function onSubmit(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault()
    const form = new FormData(event.currentTarget)
    setError('')
    setBusy(true)
    try {
      await api.auth.resetPassword(token, String(form.get('password')))
      otpFlow.clear()
      // The server signed this device in and destroyed every other session.
      await refresh()
      navigate('/lobby', { replace: true })
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Could not set the new password.')
    } finally {
      setBusy(false)
    }
  }

  return (
    <AuthShell title="Choose a New Password" subtitle="This signs you out on every other device">
      <form onSubmit={onSubmit} className="flex flex-col gap-4">
        <ErrorNote message={error} />

        <PasswordField id="password" label="New Password" autoComplete="new-password" />
        <p className="-mt-1 text-xs text-muted">At least 6 characters.</p>

        <button type="submit" className="btn-primary mt-1 w-full" disabled={busy}>
          {busy ? 'Updating…' : 'Update Password'}
        </button>
      </form>
    </AuthShell>
  )
}
