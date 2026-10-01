import { Navigate, Outlet, useLocation } from 'react-router-dom'
import { useAuth } from '../lib/auth'
import { Spinner } from './Spinner'

/** Gate for player-facing routes. Admin accounts are sent to the dashboard:
 *  they have no wallet or seat of their own to play with. */
export function RequireAuth() {
  const { user, loading } = useAuth()
  const location = useLocation()

  if (loading) return <Spinner label="Loading session…" />
  if (!user) return <Navigate to="/login" replace state={{ from: location.pathname }} />
  if (user.role === 'admin') return <Navigate to="/admin" replace />

  return <Outlet />
}

/** Inverse gate: keeps signed-in users off the login/register screens. */
export function RequireAnon() {
  const { user, loading } = useAuth()

  if (loading) return <Spinner />
  if (user) return <Navigate to={user.role === 'admin' ? '/admin' : '/lobby'} replace />

  return <Outlet />
}
