import { useEffect, useLayoutEffect, useMemo, useRef, useState } from 'react'
import type { Transaction, TransactionPage } from '../lib/types'
import { transactionKind } from '../lib/transactionKinds'
import { useElementSize } from '../lib/useElementSize'

/** Only a starting guess — the real row height is measured below. Rows are one
 *  line from `sm` up and two while the audit details are stacked. */
const ROW_HEIGHT_GUESS_STACKED = 54
const ROW_HEIGHT_GUESS_INLINE = 45
const STACK_BREAKPOINT = 640

/**
 * The player's statement, sized to its container: it asks the server for
 * exactly as many rows as will fit and pages the rest, so the wallet screen
 * never grows a scrollbar on any device.
 *
 * Rows are deliberately dense — the category and the amount carry the meaning,
 * and the audit details (timestamp, reference) sit quietly beside them rather
 * than claiming columns of their own on a phone.
 */
export function TransactionLedger({
  page,
  loading,
  onPageChange,
  onPageSizeChange,
}: {
  page: TransactionPage | null
  loading: boolean
  onPageChange: (next: number) => void
  onPageSizeChange: (size: number) => void
}) {
  const [listRef, listSize] = useElementSize<HTMLDivElement>()
  const rowsRef = useRef<HTMLUListElement>(null)
  const [rowHeight, setRowHeight] = useState(ROW_HEIGHT_GUESS_STACKED)

  // Measure a real row rather than trusting a constant. A guess that is even a
  // pixel under the truth silently clips the last row at some viewport heights,
  // and the true height moves with the breakpoint, the font, and browser zoom.
  useLayoutEffect(() => {
    const first = rowsRef.current?.firstElementChild
    if (!first) return
    const measured = first.getBoundingClientRect().height
    if (measured > 0 && Math.abs(measured - rowHeight) > 0.5) setRowHeight(measured)
  })

  const pageSize = useMemo(() => {
    if (listSize.height === 0) return 0
    const height =
      rowsRef.current?.firstElementChild
        ? rowHeight
        : listSize.width < STACK_BREAKPOINT
          ? ROW_HEIGHT_GUESS_STACKED
          : ROW_HEIGHT_GUESS_INLINE
    return Math.max(3, Math.floor(listSize.height / height))
  }, [listSize, rowHeight])

  useEffect(() => {
    if (pageSize > 0) onPageSizeChange(pageSize)
  }, [pageSize, onPageSizeChange])

  const empty = page !== null && page.total === 0
  const firstRow = page ? (page.page - 1) * page.page_size + 1 : 0
  const lastRow = page ? firstRow + page.transactions.length - 1 : 0

  return (
    <>
      <div ref={listRef} className="min-h-0 flex-1 overflow-hidden">
        {empty ? (
          <p className="grid size-full place-items-center px-5 text-center text-sm text-muted">
            No transactions yet. Your deposits, buy-ins and winnings will appear here.
          </p>
        ) : (
          <ul
            ref={rowsRef}
            className={`divide-y divide-white/5 ${loading ? 'opacity-60' : ''} transition-opacity`}
          >
            {page?.transactions.map((tx) => (
              <Row key={tx.id} tx={tx} />
            ))}
          </ul>
        )}
      </div>

      {page && page.total > 0 ? (
        <div className="flex shrink-0 items-center justify-between gap-3 border-t border-white/8 px-3 py-2">
          <span className="text-[11px] text-muted tabular-nums">
            {firstRow}–{lastRow} of {page.total}
          </span>

          <div className="flex items-center gap-1">
            <PageButton
              label="Previous page"
              disabled={loading || page.page <= 1}
              onClick={() => onPageChange(page.page - 1)}
            >
              <polyline points="15 18 9 12 15 6" />
            </PageButton>

            <span className="px-1.5 text-[11px] font-medium text-muted tabular-nums">
              {page.page} / {page.total_pages}
            </span>

            <PageButton
              label="Next page"
              disabled={loading || page.page >= page.total_pages}
              onClick={() => onPageChange(page.page + 1)}
            >
              <polyline points="9 18 15 12 9 6" />
            </PageButton>
          </div>
        </div>
      ) : null}
    </>
  )
}

function Row({ tx }: { tx: Transaction }) {
  const kind = transactionKind(tx.type)
  // The service signs the ledger: credits positive, debits negative.
  const credit = tx.amount >= 0

  return (
    // Narrow screens stack the audit details under the label; from sm up they
    // break out into their own columns so the rows read as a statement instead
    // of leaving a gulf between the label and the amount.
    <li className="grid grid-cols-[auto_minmax(0,1fr)_auto] items-center gap-x-3 px-3 py-2 sm:grid-cols-[auto_minmax(0,1fr)_auto_auto_auto] sm:gap-x-5">
      <span className={`grid size-7 shrink-0 place-items-center rounded-lg ${kind.tone}`}>
        {kind.icon}
      </span>

      <div className="min-w-0">
        <p className="truncate text-[13px] font-medium text-chalk">{kind.label}</p>
        <p className="truncate text-[11px] text-muted sm:hidden">
          {formatDate(tx.created_at)}
          <span className="mx-1.5 text-white/15">·</span>
          <span className="font-mono">{tx.transaction_id}</span>
        </p>
      </div>

      <span className="hidden font-mono text-[11px] text-muted/70 sm:block">
        {tx.transaction_id}
      </span>

      <span className="hidden text-[11px] text-muted tabular-nums sm:block sm:w-28 sm:text-right">
        {formatDate(tx.created_at)}
      </span>

      <span
        className={`text-right text-[13px] font-semibold tabular-nums sm:w-28 ${
          credit ? 'text-emerald-300' : 'text-white/70'
        }`}
      >
        {credit ? '+' : '−'}
        {Math.abs(tx.amount).toLocaleString()}
        <span className="ml-1 text-[10px] font-medium text-muted">ETB</span>
      </span>
    </li>
  )
}

function PageButton({
  label,
  disabled,
  onClick,
  children,
}: {
  label: string
  disabled: boolean
  onClick: () => void
  children: React.ReactNode
}) {
  return (
    <button
      type="button"
      aria-label={label}
      disabled={disabled}
      onClick={onClick}
      className="grid size-7 place-items-center rounded-lg border border-white/10 bg-white/5 text-muted transition-colors enabled:hover:border-gold/40 enabled:hover:text-chalk disabled:opacity-30"
    >
      <svg viewBox="0 0 24 24" width="13" height="13" fill="none" stroke="currentColor" strokeWidth="2.5" strokeLinecap="round" strokeLinejoin="round">
        {children}
      </svg>
    </button>
  )
}

function formatDate(value: string): string {
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return value
  return date.toLocaleString(undefined, {
    day: '2-digit',
    month: 'short',
    hour: '2-digit',
    minute: '2-digit',
  })
}
