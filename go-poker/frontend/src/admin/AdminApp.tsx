import { Navigate, Route, Routes, useLocation } from 'react-router-dom'
import { useAuth } from '../lib/auth'
import { Spinner } from '../components/Spinner'
import { AdminLayout } from './AdminLayout'
import Overview from './pages/Overview'
import Withdrawals from './pages/Withdrawals'
import Deposits from './pages/Deposits'
import Users from './pages/Users'
import UserDetail from './pages/UserDetail'
import Transactions from './pages/Transactions'
import Settings from './pages/Settings'

/**
 * The admin dashboard. It is loaded as its own chunk (see App.tsx), so players
 * never download it. The server enforces admin access on every /api/admin
 * call; this guard only keeps the UI from rendering for someone it would
 * refuse anyway.
 */
export default function AdminApp() {
  const { user, loading } = useAuth()
  const location = useLocation()

  if (loading) return <Spinner label="Loading session…" />
  if (!user) return <Navigate to="/login" replace state={{ from: location.pathname }} />
  if (user.role !== 'admin') return <Navigate to="/lobby" replace />

  return (
    <Routes>
      <Route element={<AdminLayout />}>
        <Route index element={<Overview />} />
        <Route path="withdrawals" element={<Withdrawals />} />
        <Route path="deposits" element={<Deposits />} />
        <Route path="users" element={<Users />} />
        <Route path="users/:id" element={<UserDetail />} />
        <Route path="transactions" element={<Transactions />} />
        <Route path="settings" element={<Settings />} />
        <Route path="*" element={<Navigate to="/admin" replace />} />
      </Route>
    </Routes>
  )
}
