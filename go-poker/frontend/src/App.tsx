import { BrowserRouter, Navigate, Route, Routes } from 'react-router-dom'
import { AuthProvider } from './lib/auth'
import { ToastProvider } from './components/Toast'
import { RequireAnon, RequireAuth } from './components/RequireAuth'
import Login from './pages/Login'
import Register from './pages/Register'
import VerifyPhone from './pages/VerifyPhone'
import ForgotPassword from './pages/ForgotPassword'
import ResetPassword from './pages/ResetPassword'
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
              {/* Both OTP flows are their own routes so a refresh — common on
                  mobile — keeps the user on the screen they were on instead of
                  costing them another SMS. */}
              <Route path="/verify-phone" element={<VerifyPhone />} />
              <Route path="/forgot-password" element={<ForgotPassword />} />
              <Route path="/reset-password" element={<ResetPassword />} />
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
