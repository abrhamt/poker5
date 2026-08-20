import { BrowserRouter, Navigate, Route, Routes } from 'react-router-dom'
import { AuthProvider } from './lib/auth'
import { ToastProvider } from './components/Toast'
import { RequireAnon, RequireAuth } from './components/RequireAuth'
import Login from './pages/Login'
import Register from './pages/Register'
import Lobby from './pages/Lobby'
import Wallet from './pages/Wallet'
import Table from './pages/Table'

export default function App() {
  return (
    <BrowserRouter>
      <ToastProvider>
        <AuthProvider>
          <Routes>
            <Route element={<RequireAnon />}>
              <Route path="/login" element={<Login />} />
              <Route path="/register" element={<Register />} />
            </Route>

            {/* Every signed-in screen owns the whole viewport and carries its
                own chrome — one hamburger, no shared nav bar. */}
            <Route element={<RequireAuth />}>
              <Route path="/lobby" element={<Lobby />} />
              <Route path="/wallet" element={<Wallet />} />
              <Route path="/table/:code" element={<Table />} />
            </Route>

            <Route path="/" element={<Navigate to="/lobby" replace />} />
            <Route path="*" element={<Navigate to="/lobby" replace />} />
          </Routes>
        </AuthProvider>
      </ToastProvider>
    </BrowserRouter>
  )
}
