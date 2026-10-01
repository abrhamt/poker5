import { useState } from 'react'
import { Link, useSearchParams } from 'react-router-dom'
import { adminApi } from '../adminApi'
import type { QueuedDeposit } from '../adminApi'
import { Sheet } from '../../components/Sheet'
import { ConfirmDialog } from '../../components/ConfirmDialog'
import { useToast } from '../../components/Toast'
import { useOverview } from '../AdminLayout'
import { Badge, EmptyState, ErrorState, formatDate, LoadingBlock, PageHeader, Pager, Panel, useAsync } from '../ui'

type Pending = { kind: 'approve' | 'reject'; deposit: QueuedDeposit } | null

export default function Deposits() {
  const [params, setParams] = useSearchParams()
  const page = Number(params.get('page') ?? '1') || 1
  const { refresh: refreshCounts } = useOverview()
  const toast = useToast()

  const { data, error, loading, reload } = useAsync(() => adminApi.deposits(page), [page])
  const [confirm, setConfirm] = useState<Pending>(null)
  const [crediting, setCrediting] = useState<QueuedDeposit | null>(null)
  const [busyId, setBusyId] = useState<number | null>(null)

  async function run(id: number, action: () => Promise<{ message: string }>) {
    setBusyId(id)
    try {
      const result = await action()
      toast(result.message, 'success')
    } catch (err) {
      toast(err instanceof Error ? err.message : 'Action failed.', 'error')
    } finally {
      setBusyId(null)
      await Promise.all([reload(), refreshCounts()])
    }
  }

  return (
    <>
      <PageHeader
        title="Deposit review"
        description={
          <>
            Receipts pasted while the bank verifier was unreachable. They are re-checked automatically every few minutes and credit
            themselves once it answers — what stays here needs a person. <strong className="text-chalk">Verify &amp; credit</strong>{' '}
            re-checks with the bank and never credits on trust. <strong className="text-chalk">Credit manually</strong> is for receipts
            the verifier cannot read: it credits the amount you type with no bank check.
          </>
        }
      />

      <Panel>
        {error ? (
          <ErrorState message={error} onRetry={() => void reload()} />
        ) : !data ? (
          <LoadingBlock />
        ) : data.items.length === 0 ? (
          <EmptyState title="Nothing waiting for review" text="Receipts land here only when the bank could not be asked." />
        ) : (
          <ul className={`divide-y divide-white/6 ${loading ? 'opacity-60' : ''}`}>
            {data.items.map((d) => (
              <li key={d.id} className="flex flex-col gap-3 px-4 py-4 lg:flex-row lg:items-center lg:gap-6">
                <div className="min-w-0 flex-1">
                  <p className="flex flex-wrap items-center gap-2">
                    <Link to={`/admin/users/${d.user_id}`} className="font-semibold text-chalk hover:text-gold-light hover:underline">
                      {d.username}
                    </Link>
                    {d.gave_up ? (
                      <Badge tone="red">{d.attempts} attempts — retry gave up</Badge>
                    ) : d.attempts > 0 ? (
                      <Badge tone="amber">{d.attempts} automatic attempts</Badge>
                    ) : (
                      <Badge>Not yet re-checked</Badge>
                    )}
                  </p>
                  <a
                    href={d.receipt_url}
                    target="_blank"
                    rel="noopener noreferrer"
                    className="mt-1 block text-xs break-all text-sky-300 hover:underline"
                  >
                    {d.receipt_url}
                  </a>
                  <p className="mt-1 text-xs text-muted">Submitted {formatDate(d.submitted_at)}</p>
                  {d.note && <p className="mt-0.5 text-xs text-muted">Last result: {d.note}</p>}
                </div>
                <div className="flex flex-wrap gap-2">
                  <button
                    type="button"
                    className="btn-gold px-3 py-2 text-xs"
                    disabled={busyId === d.id}
                    onClick={() => setConfirm({ kind: 'approve', deposit: d })}
                  >
                    {busyId === d.id ? 'Working…' : 'Verify & credit'}
                  </button>
                  <button type="button" className="btn-ghost px-3 py-2 text-xs" disabled={busyId === d.id} onClick={() => setCrediting(d)}>
                    Credit manually
                  </button>
                  <button
                    type="button"
                    className="btn-ghost px-3 py-2 text-xs"
                    disabled={busyId === d.id}
                    onClick={() => setConfirm({ kind: 'reject', deposit: d })}
                  >
                    Reject
                  </button>
                </div>
              </li>
            ))}
          </ul>
        )}
        {data && (
          <Pager
            page={data.page}
            pageSize={data.page_size}
            total={data.total}
            onPage={(p) => setParams(p > 1 ? { page: String(p) } : {})}
          />
        )}
      </Panel>

      <ConfirmDialog
        open={confirm !== null}
        message={
          confirm?.kind === 'approve'
            ? 'Re-check this receipt with the bank and credit it if it passes?'
            : 'Reject this receipt? The player is not credited.'
        }
        confirmLabel={confirm?.kind === 'approve' ? 'Verify & credit' : 'Reject'}
        onCancel={() => setConfirm(null)}
        onConfirm={() => {
          if (!confirm) return
          const { kind, deposit } = confirm
          setConfirm(null)
          void run(deposit.id, () => (kind === 'approve' ? adminApi.approveDeposit(deposit.id) : adminApi.rejectDeposit(deposit.id, '')))
        }}
      />

      <ManualCreditSheet
        deposit={crediting}
        min={data?.manual_min ?? 1}
        max={data?.manual_max ?? 1_000_000}
        onClose={() => setCrediting(null)}
        onSubmit={(amount, reference) => {
          const deposit = crediting
          setCrediting(null)
          if (deposit) void run(deposit.id, () => adminApi.creditDeposit(deposit.id, amount, reference))
        }}
      />
    </>
  )
}

function ManualCreditSheet({
  deposit,
  min,
  max,
  onClose,
  onSubmit,
}: {
  deposit: QueuedDeposit | null
  min: number
  max: number
  onClose: () => void
  onSubmit: (amount: number, reference: string) => void
}) {
  const [amount, setAmount] = useState('')
  const [reference, setReference] = useState('')
  const value = Number(amount)
  const valid = Number.isInteger(value) && value >= min && value <= max

  return (
    <Sheet
      open={deposit !== null}
      title="Credit manually"
      onClose={() => {
        setAmount('')
        setReference('')
        onClose()
      }}
    >
      {deposit && (
        <form
          className="flex flex-col gap-3.5"
          onSubmit={(event) => {
            event.preventDefault()
            if (!valid) return
            onSubmit(value, reference)
            setAmount('')
            setReference('')
          }}
        >
          <p className="text-sm text-chalk">
            Crediting <strong>{deposit.username}</strong> without a bank check. Read the amount off the receipt itself.
          </p>
          <div>
            <label className="label" htmlFor="manual-amount">
              Amount (ETB)
            </label>
            <input
              id="manual-amount"
              type="number"
              inputMode="numeric"
              min={min}
              max={max}
              step={1}
              required
              value={amount}
              onChange={(e) => setAmount(e.target.value)}
              className="field tabular-nums"
            />
          </div>
          <div>
            <label className="label" htmlFor="manual-reference">
              FT reference (recommended)
            </label>
            <input
              id="manual-reference"
              maxLength={64}
              value={reference}
              onChange={(e) => setReference(e.target.value)}
              className="field font-mono"
              placeholder="FT…"
            />
            <p className="mt-1 text-[11px] text-muted">The reference is what stops the same transfer being credited twice through its other link.</p>
          </div>
          <button type="submit" className="btn-gold w-full" disabled={!valid}>
            Credit {valid ? `${value.toLocaleString()} ETB` : ''}
          </button>
        </form>
      )}
    </Sheet>
  )
}
