import { useState } from 'react'
import { Link, useNavigate } from 'react-router-dom'
import { useAuth } from '../lib/auth'
import { AuthShell, ErrorNote, PasswordField } from '../components/AuthShell'

export default function Login() {
  const { login } = useAuth()
  const navigate = useNavigate()
  const [error, setError] = useState('')
  const [busy, setBusy] = useState(false)

  async function onSubmit(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault()
    const form = new FormData(event.currentTarget)
    setError('')
    setBusy(true)
    try {
      await login(String(form.get('login')), String(form.get('password')))
      // RequireAuth forwards admin accounts on to the Go-rendered dashboard.
      navigate('/lobby', { replace: true })
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Sign in failed.')
    } finally {
      setBusy(false)
    }
  }

  return (
    <AuthShell title="Golden Poker" subtitle="Sign in to enter live tables">
      <form onSubmit={onSubmit} className="flex flex-col gap-4">
        <ErrorNote message={error} />

        <div>
          <label className="label" htmlFor="login">
            Username or Phone
          </label>
          <input
            id="login"
            name="login"
            required
            autoComplete="username"
            placeholder="Username or phone (e.g. 0912345678)"
            className="field"
          />
        </div>

        <PasswordField id="password" label="Password" autoComplete="current-password" />

        <button type="submit" className="btn-primary mt-1 w-full" disabled={busy}>
          {busy ? 'Signing in…' : 'Sign In'}
        </button>

        <Link
          to="/forgot-password"
          className="text-center text-sm font-semibold text-gold-light hover:underline"
        >
          Forgot your password?
        </Link>
      </form>

      <p className="mt-6 text-center text-sm text-muted">
        New to Golden Poker?{' '}
        <Link to="/register" className="font-semibold text-gold-light hover:underline">
          Create Account
        </Link>
      </p>
    </AuthShell>
  )
}
