import { useState } from 'react'
import { Link, useParams } from 'react-router-dom'
import { adminApi } from '../adminApi'
import type { AdminWithdrawal } from '../adminApi'
import { useToast } from '../../components/Toast'
import { transactionKind } from '../../lib/transactionKinds'
import { useOverview } from '../AdminLayout'
import { EmptyState, ErrorState, etb, formatDate, LoadingBlock, Panel, signedEtb, TableWrap, td, th, useAsync } from '../ui'
import { RoleBadge } from './Users'
import { CompleteSheet, RejectSheet, WithdrawalRow } from './Withdrawals'

/** One player on one page: who they are, where their money went, and what
 *  they have asked to withdraw — what an admin wants in view before paying. */
export default function UserDetail() {
  const id = Number(useParams().id)
  const toast = useToast()
  const { refresh: refreshCounts } = useOverview()

  const detail = useAsync(() => adminApi.user(id), [id])
  const withdrawals = useAsync(() => adminApi.withdrawals('', 1, id), [id])
  const [completing, setCompleting] = useState<AdminWithdrawal | null>(null)
  const [rejecting, setRejecting] = useState<AdminWithdrawal | null>(null)

  async function afterAction(message: string) {
    toast(message, 'success')
    setCompleting(null)
    setRejecting(null)
    await Promise.all([detail.reload(), withdrawals.reload(), refreshCounts()])
  }

  if (detail.error) return <ErrorState message={detail.error} onRetry={() => void detail.reload()} />
  if (!detail.data) return <LoadingBlock />
  const { user, transactions } = detail.data

  return (
    <>
      <Link to="/admin/users" className="mb-4 inline-block text-xs font-semibold text-muted hover:text-chalk">
        ← All users
      </Link>

      <div className="card mb-6 flex flex-wrap items-center justify-between gap-4 p-5">
        <div className="flex items-center gap-4">
          <span className="grid size-12 place-items-center rounded-full bg-gold/20 font-heading text-lg font-bold text-gold-light uppercase">
            {user.username.charAt(0)}
          </span>
          <div>
            <h1 className="flex items-center gap-2 text-xl font-bold">
              {user.username} <RoleBadge role={user.role} />
            </h1>
            <p className="text-sm text-muted">
              {user.phone_number} · joined {formatDate(user.created_at, false)}
            </p>
            <p className="text-xs text-muted">
              Referral code <span className="font-mono text-chalk">{user.referral_code}</span>
              {user.referred_by && (
                <>
                  {' '}
                  · referred by <span className="font-mono text-chalk">{user.referred_by}</span>
                </>
              )}
            </p>
          </div>
        </div>
        <div className="text-right">
          <p className="text-[11px] tracking-wide text-muted uppercase">Wallet</p>
          <p className="font-heading text-3xl font-bold text-gold-light tabular-nums">{etb(user.wallet)}</p>
        </div>
      </div>

      <div className="grid gap-6">
        <Panel title="Withdrawals">
          {withdrawals.error ? (
            <ErrorState message={withdrawals.error} onRetry={() => void withdrawals.reload()} />
          ) : !withdrawals.data ? (
            <LoadingBlock />
          ) : withdrawals.data.items.length === 0 ? (
            <EmptyState title="No withdrawals yet" />
          ) : (
            <ul className="divide-y divide-white/6">
              {withdrawals.data.items.map((w) => (
                <WithdrawalRow key={w.id} withdrawal={w} showUser={false} onComplete={setCompleting} onReject={setRejecting} />
              ))}
            </ul>
          )}
        </Panel>

        <Panel
          title="Recent transactions"
          actions={
            <Link
              to={`/admin/transactions?query=${encodeURIComponent(user.username)}`}
              className="text-xs font-semibold text-gold-light hover:underline"
            >
              View all →
            </Link>
          }
        >
          {transactions.length === 0 ? (
            <EmptyState title="No transactions yet" />
          ) : (
            <TableWrap>
              <table className="w-full">
                <thead className="border-b border-white/8">
                  <tr>
                    <th className={th}>Type</th>
                    <th className={`${th} text-right`}>Amount</th>
                    <th className={th}>Tx ID</th>
                    <th className={th}>Date</th>
                  </tr>
                </thead>
                <tbody className="divide-y divide-white/6">
                  {transactions.map((t) => {
                    const kind = transactionKind(t.type)
                    return (
                      <tr key={t.id}>
                        <td className={td}>
                          <span className={`inline-flex items-center gap-1.5 rounded-md px-2 py-0.5 text-xs font-medium ${kind.tone}`}>
                            {kind.icon}
                            {kind.label}
                          </span>
                        </td>
                        <td className={`${td} text-right font-semibold tabular-nums ${t.amount >= 0 ? 'text-emerald-300' : 'text-chalk'}`}>
                          {signedEtb(t.amount)}
                        </td>
                        <td className={`${td} font-mono text-xs text-muted`}>{t.transaction_id}</td>
                        <td className={`${td} text-muted`}>{formatDate(t.created_at)}</td>
                      </tr>
                    )
                  })}
                </tbody>
              </table>
            </TableWrap>
          )}
        </Panel>
      </div>

      <CompleteSheet
        withdrawal={completing}
        onClose={() => setCompleting(null)}
        onDone={(w) => void afterAction(`Marked ${etb(w.amount)} as paid.`)}
        onError={(err) => toast(err, 'error')}
      />
      <RejectSheet
        withdrawal={rejecting}
        onClose={() => setRejecting(null)}
        onDone={(w) => void afterAction(`Rejected — ${etb(w.amount)} returned to the wallet.`)}
        onError={(err) => toast(err, 'error')}
      />
    </>
  )
}
