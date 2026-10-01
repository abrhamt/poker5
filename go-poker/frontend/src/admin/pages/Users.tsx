import { useEffect, useState } from 'react'
import { Link, useNavigate, useSearchParams } from 'react-router-dom'
import { adminApi } from '../adminApi'
import { Badge, EmptyState, ErrorState, etb, formatDate, LoadingBlock, PageHeader, Pager, Panel, TableWrap, td, th, useAsync, useDebounced } from '../ui'

export function RoleBadge({ role }: { role: string }) {
  return role === 'admin' ? <Badge tone="gold">Admin</Badge> : <Badge>{role}</Badge>
}

export default function Users() {
  const [params, setParams] = useSearchParams()
  const navigate = useNavigate()
  const page = Number(params.get('page') ?? '1') || 1
  const query = params.get('query') ?? ''
  const [text, setText] = useState(query)
  const debounced = useDebounced(text)

  // The search box writes to the URL once typing settles, so a refresh or a
  // shared link keeps the search.
  useEffect(() => {
    if (debounced.trim() === query) return
    setParams(debounced.trim() ? { query: debounced.trim() } : {}, { replace: true })
  }, [debounced, query, setParams])

  const { data, error, loading, reload } = useAsync(() => adminApi.users(query, page), [query, page])

  return (
    <>
      <PageHeader title="Users" description="Search by username or phone number." />

      <input
        type="search"
        value={text}
        onChange={(e) => setText(e.target.value)}
        placeholder="Search username or phone…"
        className="field mb-4 max-w-md"
      />

      <Panel>
        {error ? (
          <ErrorState message={error} onRetry={() => void reload()} />
        ) : !data ? (
          <LoadingBlock />
        ) : data.items.length === 0 ? (
          <EmptyState title="No users found" />
        ) : (
          <TableWrap>
            <table className={`w-full ${loading ? 'opacity-60' : ''}`}>
              <thead className="border-b border-white/8">
                <tr>
                  <th className={th}>Username</th>
                  <th className={th}>Phone</th>
                  <th className={`${th} text-right`}>Wallet</th>
                  <th className={th}>Role</th>
                  <th className={th}>Joined</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-white/6">
                {data.items.map((u) => (
                  <tr
                    key={u.id}
                    className="cursor-pointer hover:bg-white/3"
                    onClick={() => navigate(`/admin/users/${u.id}`)}
                  >
                    <td className={td}>
                      <Link to={`/admin/users/${u.id}`} className="font-semibold text-chalk hover:text-gold-light" onClick={(e) => e.stopPropagation()}>
                        {u.username}
                      </Link>
                    </td>
                    <td className={`${td} text-muted`}>{u.phone_number}</td>
                    <td className={`${td} text-right tabular-nums`}>{etb(u.wallet)}</td>
                    <td className={td}>
                      <RoleBadge role={u.role} />
                    </td>
                    <td className={`${td} text-muted`}>{formatDate(u.created_at, false)}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </TableWrap>
        )}
        {data && (
          <Pager
            page={data.page}
            pageSize={data.page_size}
            total={data.total}
            onPage={(p) => {
              const next = new URLSearchParams(params)
              if (p > 1) next.set('page', String(p))
              else next.delete('page')
              setParams(next)
            }}
          />
        )}
      </Panel>
    </>
  )
}
