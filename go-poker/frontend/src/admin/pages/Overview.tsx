import { useEffect } from 'react'
import { Link } from 'react-router-dom'
import type { ReactNode } from 'react'
import { useOverview } from '../AdminLayout'
import { etb, LoadingBlock, PageHeader, StatCard } from '../ui'

export default function Overview() {
  const { overview, refresh } = useOverview()

  // The layout polls, but landing here should never show numbers from before
  // the action that brought the admin back.
  useEffect(() => {
    void refresh()
  }, [refresh])

  if (!overview) return <LoadingBlock />
  const e = overview.earnings

  return (
    <>
      <PageHeader title="Overview" description="House earnings and anything waiting on you." />

      <div className="mb-6 grid gap-3 md:grid-cols-2">
        <ActionCard
          to="/admin/withdrawals"
          count={overview.pending_withdrawals}
          title={overview.pending_withdrawals === 1 ? 'withdrawal to pay' : 'withdrawals to pay'}
          detail={
            overview.pending_withdrawals > 0
              ? `${etb(overview.pending_withdrawal_total)} set aside for players`
              : 'Nothing waiting'
          }
        />
        <ActionCard
          to="/admin/deposits"
          count={overview.pending_deposits}
          title={overview.pending_deposits === 1 ? 'deposit to review' : 'deposits to review'}
          detail={overview.pending_deposits > 0 ? 'Receipts the bank verifier could not settle' : 'Nothing waiting'}
        />
      </div>

      <h2 className="mb-3 text-xs font-semibold tracking-wide text-muted uppercase">House</h2>
      <div className="mb-6 grid grid-cols-2 gap-3 lg:grid-cols-4">
        <StatCard label="House balance" value={etb(e.house_balance)} tone="gold" hint="Everything the house account holds" />
        <StatCard label="Rake today" value={etb(e.rake_today)} />
        <StatCard label="Rake this week" value={etb(e.rake_week)} />
        <StatCard label="Rake this month" value={etb(e.rake_month)} />
      </div>

      <h2 className="mb-3 text-xs font-semibold tracking-wide text-muted uppercase">Referrals paid</h2>
      <div className="mb-6 grid grid-cols-2 gap-3 lg:grid-cols-4">
        <StatCard label="Today" value={etb(e.referral_paid_today)} tone="muted" />
        <StatCard label="This week" value={etb(e.referral_paid_week)} tone="muted" />
        <StatCard label="This month" value={etb(e.referral_paid_month)} tone="muted" />
        <StatCard label="Accounts" value={overview.total_users.toLocaleString()} tone="muted" />
      </div>
    </>
  )
}

function ActionCard({ to, count, title, detail }: { to: string; count: number; title: string; detail: ReactNode }) {
  const active = count > 0
  return (
    <Link
      to={to}
      className={`group flex items-center justify-between gap-4 rounded-2xl border p-4 transition-colors ${
        active ? 'border-gold/40 bg-gold/10 hover:border-gold/70' : 'border-white/10 bg-panel/60 hover:border-white/20'
      }`}
    >
      <div>
        <p className="flex items-baseline gap-2">
          <span className={`font-heading text-3xl font-bold tabular-nums ${active ? 'text-gold-light' : 'text-chalk'}`}>{count}</span>
          <span className="text-sm font-medium text-chalk">{title}</span>
        </p>
        <p className="mt-0.5 text-xs text-muted">{detail}</p>
      </div>
      <span className="text-muted transition-transform group-hover:translate-x-0.5">→</span>
    </Link>
  )
}
