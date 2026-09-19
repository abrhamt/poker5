import { Navigate, Outlet, useLocation } from 'react-router-dom'
import { useAuth } from '../lib/auth'
import { Spinner } from './Spinner'

/** Gate for player-facing routes. Admin accounts are pushed to the admin
 *  dashboard, which is still served by Go outside this SPA. */
export function RequireAuth() {
  const { user, loading } = useAuth()
  const location = useLocation()

  if (loading) return <Spinner label="Loading session…" />
  if (!user) return <Navigate to="/login" replace state={{ from: location.pathname }} />
  if (user.role === 'admin') {
    window.location.replace('/admin')
    return <Spinner label="Redirecting to the admin dashboard…" />
  }

  return <Outlet />
}

/** Inverse gate: keeps signed-in players off the login/register screens.
 *  Admins land on /lobby too and RequireAuth bounces them to /admin. */
export function RequireAnon() {
  const { user, loading } = useAuth()

  if (loading) return <Spinner />
  if (user) return <Navigate to="/lobby" replace />

  return <Outlet />
}
