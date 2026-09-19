import { useEffect, useState } from 'react'
import { Link, useLocation, useNavigate } from 'react-router-dom'
import { useAuth } from '../lib/auth'
import { otpFlow } from '../lib/otpFlow'
import { AuthShell } from '../components/AuthShell'
import { OtpEntry } from '../components/OtpEntry'

/** Second half of registration. The account does not exist until the code
 *  entered here checks out — at which point the server signs the new player in
 *  and the lobby is one redirect away. */
export default function VerifyPhone() {
  const { verifyOtp } = useAuth()
  const navigate = useNavigate()
  const location = useLocation()
  // Router state on the first render, sessionStorage after a refresh.
  const [phone] = useState(
    () => (location.state as { phone?: string } | null)?.phone || otpFlow.phone(),
  )

  useEffect(() => {
    if (!phone) navigate('/register', { replace: true })
  }, [phone, navigate])

  if (!phone) return null

  return (
    <AuthShell title="Verify Your Phone" subtitle="Enter the code we sent by SMS">
      <OtpEntry
        phone={phone}
        purpose="register"
        submitLabel="Verify & Continue"
        busyLabel="Verifying…"
        onSubmit={async (code) => {
          await verifyOtp(phone, code)
          otpFlow.clear()
          navigate('/lobby', { replace: true })
        }}
      />

      <p className="mt-6 text-center text-sm text-muted">
        Wrong number?{' '}
        <Link to="/register" className="font-semibold text-gold-light hover:underline">
          Start over
        </Link>
      </p>
    </AuthShell>
  )
}
