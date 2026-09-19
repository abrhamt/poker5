import { useEffect, useRef, useState } from 'react'
import { useSearchParams } from 'react-router-dom'
import { api } from './api'
import type { GatewayDepositStatus } from './types'

const POLL_EVERY_MS = 2000
const POLL_ATTEMPTS = 30

export type GatewayReturnResult =
  | { kind: 'credited'; amount: number }
  | { kind: 'failed' | 'cancelled' | 'expired'; amount: number }
  | { kind: 'timeout'; amount: number | null }

/** Watches the `deposit` query parameter the gateway sends players back with.
 *  The credit lands through the server's callback, not this poll: polling only
 *  tells the screen when to refresh and what to say, so a slow gateway is
 *  reported as "still confirming" rather than as a failure. */
export function useGatewayDepositReturn(onSettled: (result: GatewayReturnResult) => void) {
  const [params, setParams] = useSearchParams()
  const reference = params.get('deposit')
  const [pending, setPending] = useState<{ reference: string; amount: number | null } | null>(null)
  const settled = useRef(onSettled)
  settled.current = onSettled

  useEffect(() => {
    if (!reference) return
    let cancelled = false
    let attempts = 0
    setPending({ reference, amount: null })

    const finish = (result: GatewayReturnResult) => {
      if (cancelled) return
      cancelled = true
      setPending(null)
      const next = new URLSearchParams(params)
      next.delete('deposit')
      setParams(next, { replace: true })
      settled.current(result)
    }

    const tick = async () => {
      if (cancelled) return
      attempts += 1
      let status: GatewayDepositStatus | null = null
      try {
        status = await api.wallet.gatewayDepositStatus(reference)
        setPending((current) => (current ? { ...current, amount: status?.amount ?? null } : current))
      } catch {
        status = null
      }
      if (status && status.status !== 'pending') {
        finish({ kind: status.status, amount: status.amount })
        return
      }
      if (attempts >= POLL_ATTEMPTS) {
        finish({ kind: 'timeout', amount: status?.amount ?? null })
        return
      }
      window.setTimeout(() => void tick(), POLL_EVERY_MS)
    }

    void tick()
    return () => {
      cancelled = true
    }
    // The query string is the only trigger: re-running on `params` identity
    // would restart the poll every time the URL is rewritten.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [reference])

  return pending
}
