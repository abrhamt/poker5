import { useEffect, useState } from 'react'
import { Link, useSearchParams } from 'react-router-dom'
import { adminApi } from '../adminApi'
import { transactionKind } from '../../lib/transactionKinds'
import { EmptyState, ErrorState, formatDate, LoadingBlock, PageHeader, Pager, Panel, signedEtb, TableWrap, td, th, useAsync, useDebounced } from '../ui'

const TYPES = [
  'deposit',
  'withdrawal',
  'withdrawal_refund',
  'buy_in',
  'cash_out',
  'winner_payout',
  'site_rake',
  'referral_commission',
]

export default function Transactions() {
  const [params, setParams] = useSearchParams()
  const page = Number(params.get('page') ?? '1') || 1
  const query = params.get('query') ?? ''
  const type = params.get('type') ?? ''
  const [text, setText] = useState(query)
  const debounced = useDebounced(text)

  function update(next: { query?: string; type?: string; page?: number }, replace = false) {
    const out = new URLSearchParams()
    const q = next.query ?? query
    const t = next.type ?? type
    const p = next.page ?? 1
    if (q) out.set('query', q)
    if (t) out.set('type', t)
    if (p > 1) out.set('page', String(p))
    setParams(out, { replace })
  }

  useEffect(() => {
    if (debounced.trim() !== query) update({ query: debounced.trim() }, true)
    // update is recreated every render; only the debounced text should trigger this.
  }, [debounced])

  const { data, error, loading, reload } = useAsync(() => adminApi.transactions(query, type, page), [query, type, page])

  return (
    <>
      <PageHeader title="Transactions" description="Every wallet movement across the site, newest first." />

      <div className="mb-4 flex flex-col gap-2 sm:flex-row">
        <input
          type="search"
          value={text}
          onChange={(e) => setText(e.target.value)}
          placeholder="Filter by username…"
          className="field sm:max-w-xs"
        />
        <select value={type} onChange={(e) => update({ type: e.target.value })} className="field sm:max-w-56">
          <option value="">All types</option>
          {TYPES.map((t) => (
            <option key={t} value={t}>
              {transactionKind(t).label}
            </option>
          ))}
        </select>
      </div>

      <Panel>
        {error ? (
          <ErrorState message={error} onRetry={() => void reload()} />
        ) : !data ? (
          <LoadingBlock />
        ) : data.items.length === 0 ? (
          <EmptyState title="No transactions found" />
        ) : (
          <TableWrap>
            <table className={`w-full ${loading ? 'opacity-60' : ''}`}>
              <thead className="border-b border-white/8">
                <tr>
                  <th className={th}>User</th>
                  <th className={th}>Type</th>
                  <th className={`${th} text-right`}>Amount</th>
                  <th className={th}>Tx ID</th>
                  <th className={th}>Date</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-white/6">
                {data.items.map((t) => {
                  const kind = transactionKind(t.type)
                  return (
                    <tr key={t.id}>
                      <td className={td}>
                        <Link to={`/admin/users/${t.user_id}`} className="font-semibold text-chalk hover:text-gold-light">
                          {t.username}
                        </Link>
                      </td>
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
        {data && <Pager page={data.page} pageSize={data.page_size} total={data.total} onPage={(p) => update({ page: p })} />}
      </Panel>
    </>
  )
}
