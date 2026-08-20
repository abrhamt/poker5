import { createContext, use, useCallback, useEffect, useMemo, useState } from 'react'
import type { ReactNode } from 'react'
import { api, ApiError } from './api'
import type { User } from './types'

interface AuthValue {
  user: User | null
  /** True until the initial /api/auth/me round-trip settles. */
  loading: boolean
  login: (login: string, password: string) => Promise<User>
  register: (input: {
    username: string
    phone_number: string
    password: string
    referral_code?: string
  }) => Promise<void>
  logout: () => Promise<void>
  /** Re-reads the session — used after anything that moves the wallet balance. */
  refresh: () => Promise<void>
}

const AuthContext = createContext<AuthValue | null>(null)

export function AuthProvider({ children }: { children: ReactNode }) {
  const [user, setUser] = useState<User | null>(null)
  const [loading, setLoading] = useState(true)

  const refresh = useCallback(async () => {
    try {
      setUser(await api.auth.me())
    } catch (err) {
      // A 401 just means "not signed in"; anything else is worth keeping the
      // last known user for rather than bouncing a player out mid-hand.
      if (err instanceof ApiError && err.status === 401) setUser(null)
    }
  }, [])

  useEffect(() => {
    refresh().finally(() => setLoading(false))
  }, [refresh])

  const value = useMemo<AuthValue>(
    () => ({
      user,
      loading,
      refresh,
      login: async (login, password) => {
        const res = await api.auth.login(login, password)
        setUser(res.user)
        return res.user
      },
      register: async (input) => {
        await api.auth.register(input)
        await refresh()
      },
      logout: async () => {
        await api.auth.logout()
        setUser(null)
      },
    }),
    [user, loading, refresh],
  )

  return <AuthContext value={value}>{children}</AuthContext>
}

export function useAuth(): AuthValue {
  const ctx = use(AuthContext)
  if (!ctx) throw new Error('useAuth must be used inside <AuthProvider>')
  return ctx
}
