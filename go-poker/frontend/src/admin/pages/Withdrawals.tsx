import { useState } from 'react'
import { Link, useSearchParams } from 'react-router-dom'
import { adminApi } from '../adminApi'
import type { AdminWithdrawal } from '../adminApi'
import type { WithdrawalStatus } from '../../lib/types'
import { Sheet } from '../../components/Sheet'
import { useToast } from '../../components/Toast'
import { METHOD_LABEL } from '../../components/wallet/Withdraw'
import { useOverview } from '../AdminLayout'
import { Badge, CopyButton, EmptyState, ErrorState, etb, formatDate, LoadingBlock, PageHeader, Pager, Panel, timeAgo, useAsync } from '../ui'

const TABS: { status: WithdrawalStatus | ''; label: string }[] = [
  { status: 'pending', label: 'Pending' },
  { status: 'completed', label: 'Paid' },
  { status: 'rejected', label: 'Rejected' },
  { status: 'cancelled', label: 'Cancelled' },
  { status: '', label: 'All' },
]

const QUICK_REASONS = [
  'The account name does not match the account number.',
  'The account number is invalid.',
  'Please contact support about this withdrawal.',
]

export function WithdrawalStatusBadge({ status }: { status: WithdrawalStatus }) {
  switch (status) {
    case 'pending':
      return <Badge tone="amber">Pending</Badge>
    case 'completed':
      return <Badge tone="green">Paid</Badge>
    case 'rejected':
      return <Badge tone="red">Rejected</Badge>
    default:
      return <Badge>Cancelled</Badge>
  }
}

export default function Withdrawals() {
  const [params, setParams] = useSearchParams()
  // "all" in the URL reads better than an empty status parameter.
  const rawStatus = params.get('status') ?? 'pending'
  const status = (rawStatus === 'all' ? '' : rawStatus) as WithdrawalStatus | ''
  const page = Number(params.get('page') ?? '1') || 1
  const { overview, refresh: refreshCounts } = useOverview()
  const toast = useToast()

  const { data, error, loading, reload } = useAsync(() => adminApi.withdrawals(status, page), [status, page])
  const [completing, setCompleting] = useState<AdminWithdrawal | null>(null)
  const [rejecting, setRejecting] = useState<AdminWithdrawal | null>(null)

  function go(next: { status?: WithdrawalStatus | ''; page?: number }) {
    const query = new URLSearchParams()
    const s = next.status ?? status
    if (s !== 'pending') query.set('status', s || 'all')
    const p = next.page ?? 1
    if (p > 1) query.set('page', String(p))
    setParams(query)
  }

  async function afterAction(message: string) {
    toast(message, 'success')
    setCompleting(null)
    setRejecting(null)
    await Promise.all([reload(), refreshCounts()])
  }

  return (
    <>
      <PageHeader
        title="Withdrawals"
        description="Pay the request out by CBE transfer or Telebirr, then mark it paid. Rejecting returns the money to the player's wallet."
      />

      <div className="mb-4 flex gap-1 overflow-x-auto rounded-xl border border-white/8 bg-black/20 p-1">
        {TABS.map((tab) => {
          const active = status === tab.status
          const count = tab.status === 'pending' ? overview?.pending_withdrawals : undefined
          return (
            <button
              key={tab.label}
              type="button"
              onClick={() => go({ status: tab.status })}
              className={`flex items-center gap-1.5 rounded-lg px-3 py-1.5 text-sm font-medium whitespace-nowrap transition-colors ${
                active ? 'bg-gold/15 text-gold-light' : 'text-muted hover:text-chalk'
              }`}
            >
              {tab.label}
              {count ? <span className="rounded-full bg-gold px-1.5 text-[10px] font-bold text-ink">{count}</span> : null}
            </button>
          )
        })}
      </div>

      <Panel>
        {error ? (
          <ErrorState message={error} onRetry={() => void reload()} />
        ) : !data ? (
          <LoadingBlock />
        ) : data.items.length === 0 ? (
          <EmptyState
            title={status === 'pending' ? 'No withdrawals waiting' : 'No withdrawals here'}
            text={status === 'pending' ? 'New requests appear here as players make them.' : undefined}
          />
        ) : (
          <ul className={`divide-y divide-white/6 transition-opacity ${loading ? 'opacity-60' : ''}`}>
            {data.items.map((w) => (
              <WithdrawalRow key={w.id} withdrawal={w} onComplete={setCompleting} onReject={setRejecting} />
            ))}
          </ul>
        )}
        {data && <Pager page={data.page} pageSize={data.page_size} total={data.total} onPage={(p) => go({ page: p })} />}
      </Panel>

      <CompleteSheet
        withdrawal={completing}
        onClose={() => setCompleting(null)}
        onDone={(w) => void afterAction(`Marked ${etb(w.amount)} to ${w.username} as paid.`)}
        onError={(err) => toast(err, 'error')}
      />
      <RejectSheet
        withdrawal={rejecting}
        onClose={() => setRejecting(null)}
        onDone={(w) => void afterAction(`Rejected — ${etb(w.amount)} returned to ${w.username}'s wallet.`)}
        onError={(err) => toast(err, 'error')}
      />
    </>
  )
}

export function WithdrawalRow({
  withdrawal: w,
  onComplete,
  onReject,
  showUser = true,
}: {
  withdrawal: AdminWithdrawal
  onComplete?: (w: AdminWithdrawal) => void
  onReject?: (w: AdminWithdrawal) => void
  showUser?: boolean
}) {
  const pending = w.status === 'pending'
  return (
    <li className="flex flex-col gap-3 px-4 py-4 md:flex-row md:items-center md:gap-6">
      <div className="flex items-center justify-between gap-3 md:w-40 md:flex-col md:items-start md:justify-start md:gap-1">
        <span className="font-heading text-xl font-bold text-gold-light tabular-nums">{etb(w.amount)}</span>
        <WithdrawalStatusBadge status={w.status} />
      </div>

      <div className="grid min-w-0 flex-1 gap-x-6 gap-y-1.5 text-sm sm:grid-cols-2">
        <div className="min-w-0">
          <p className="text-[11px] tracking-wide text-muted uppercase">{METHOD_LABEL[w.method]}</p>
          <p className="flex items-center gap-2">
            <span className="font-mono font-semibold tracking-wide text-chalk">{w.account_number}</span>
            <CopyButton value={w.account_number} />
          </p>
          <p className="flex items-center gap-2 text-chalk">
            <span className="truncate">{w.account_name}</span>
            <CopyButton value={w.account_name} />
          </p>
        </div>
        <div className="min-w-0">
          {showUser && (
            <>
              <p className="text-[11px] tracking-wide text-muted uppercase">Player</p>
              <p>
                <Link to={`/admin/users/${w.user_id}`} className="font-semibold text-chalk hover:text-gold-light hover:underline">
                  {w.username}
                </Link>{' '}
                <span className="text-muted">{w.phone_number}</span>
              </p>
            </>
          )}
          <p className="text-xs text-muted" title={formatDate(w.created_at)}>
            Requested {timeAgo(w.created_at)} · {formatDate(w.created_at)}
          </p>
          {!pending && (
            <p className="text-xs text-muted">
              {w.status === 'cancelled' ? 'Cancelled by the player' : `${w.status === 'completed' ? 'Paid' : 'Rejected'} by ${w.resolved_by || '—'}`}{' '}
              · {formatDate(w.resolved_at)}
            </p>
          )}
          {w.payout_reference && <p className="text-xs text-muted">Reference: <span className="font-mono text-chalk">{w.payout_reference}</span></p>}
          {w.status === 'rejected' && w.note && <p className="text-xs text-red-300">Reason: {w.note}</p>}
        </div>
      </div>

      {pending && onComplete && onReject && (
        <div className="flex gap-2 md:flex-col">
          <button type="button" className="btn-gold flex-1 px-4 py-2 text-xs" onClick={() => onComplete(w)}>
            Mark paid
          </button>
          <button type="button" className="btn-ghost flex-1 px-4 py-2 text-xs" onClick={() => onReject(w)}>
            Reject
          </button>
        </div>
      )}
    </li>
  )
}

function Summary({ w }: { w: AdminWithdrawal }) {
  return (
    <div className="rounded-xl border border-gold/25 bg-gold/5 p-3 text-sm">
      <p className="font-heading text-xl font-bold text-gold-light">{etb(w.amount)}</p>
      <p className="mt-1 text-chalk">
        {METHOD_LABEL[w.method]} · <span className="font-mono">{w.account_number}</span>
      </p>
      <p className="text-chalk">{w.account_name}</p>
      <p className="mt-1 text-xs text-muted">Requested by {w.username}</p>
    </div>
  )
}

export function CompleteSheet({
  withdrawal,
  onClose,
  onDone,
  onError,
}: {
  withdrawal: AdminWithdrawal | null
  onClose: () => void
  onDone: (w: AdminWithdrawal) => void
  onError: (message: string) => void
}) {
  const [reference, setReference] = useState('')
  const [busy, setBusy] = useState(false)

  return (
    <Sheet
      open={withdrawal !== null}
      title="Mark as paid"
      onClose={() => {
        setReference('')
        onClose()
      }}
    >
      {withdrawal && (
        <form
          className="flex flex-col gap-3.5"
          onSubmit={async (event) => {
            event.preventDefault()
            setBusy(true)
            try {
              await adminApi.completeWithdrawal(withdrawal.id, reference)
              setReference('')
              onDone(withdrawal)
            } catch (err) {
              onError(err instanceof Error ? err.message : 'Could not mark it paid.')
            } finally {
              setBusy(false)
            }
          }}
        >
          <Summary w={withdrawal} />
          <div>
            <label className="label" htmlFor="payout-reference">
              Transfer reference (optional)
            </label>
            <input
              id="payout-reference"
              className="field font-mono"
              maxLength={64}
              value={reference}
              onChange={(e) => setReference(e.target.value)}
              placeholder={withdrawal.method === 'cbe' ? 'FT…' : 'Telebirr transaction number'}
            />
          </div>
          <p className="text-[11px] leading-relaxed text-muted">
            Only confirm once the money has actually been sent. This cannot be undone, and nothing further leaves the player's wallet.
          </p>
          <button type="submit" className="btn-gold w-full" disabled={busy}>
            {busy ? 'Saving…' : `I have sent ${etb(withdrawal.amount)}`}
          </button>
        </form>
      )}
    </Sheet>
  )
}

export function RejectSheet({
  withdrawal,
  onClose,
  onDone,
  onError,
}: {
  withdrawal: AdminWithdrawal | null
  onClose: () => void
  onDone: (w: AdminWithdrawal) => void
  onError: (message: string) => void
}) {
  const [reason, setReason] = useState('')
  const [busy, setBusy] = useState(false)

  return (
    <Sheet
      open={withdrawal !== null}
      title="Reject withdrawal"
      onClose={() => {
        setReason('')
        onClose()
      }}
    >
      {withdrawal && (
        <form
          className="flex flex-col gap-3.5"
          onSubmit={async (event) => {
            event.preventDefault()
            if (!reason.trim()) return
            setBusy(true)
            try {
              await adminApi.rejectWithdrawal(withdrawal.id, reason)
              setReason('')
              onDone(withdrawal)
            } catch (err) {
              onError(err instanceof Error ? err.message : 'Could not reject it.')
            } finally {
              setBusy(false)
            }
          }}
        >
          <Summary w={withdrawal} />
          <div>
            <label className="label" htmlFor="reject-reason">
              Reason (shown to the player)
            </label>
            <textarea
              id="reject-reason"
              className="field resize-none"
              rows={3}
              maxLength={255}
              required
              value={reason}
              onChange={(e) => setReason(e.target.value)}
            />
            <div className="mt-2 flex flex-wrap gap-1.5">
              {QUICK_REASONS.map((r) => (
                <button
                  key={r}
                  type="button"
                  onClick={() => setReason(r)}
                  className="rounded-lg border border-white/10 bg-white/5 px-2 py-1 text-left text-[11px] text-muted hover:border-gold/30 hover:text-chalk"
                >
                  {r}
                </button>
              ))}
            </div>
          </div>
          <p className="text-[11px] leading-relaxed text-muted">{etb(withdrawal.amount)} goes straight back to {withdrawal.username}'s wallet.</p>
          <button type="submit" className="btn-danger w-full" disabled={busy || !reason.trim()}>
            {busy ? 'Rejecting…' : 'Reject and refund'}
          </button>
        </form>
      )}
    </Sheet>
  )
}
