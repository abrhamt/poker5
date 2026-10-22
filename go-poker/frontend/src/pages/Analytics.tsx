import { useEffect, useMemo, useState } from 'react'
import { Link } from 'react-router-dom'
import { api } from '../lib/api'
import type { PlayerStats } from '../lib/types'
import { useToast } from '../components/Toast'
import { Spinner } from '../components/Spinner'
import { NetValue, PageShell, formatNet } from '../components/PageShell'
import { useElementSize } from '../lib/useElementSize'

/** The single series colour: the brand gold, stepped down to sit inside the
 *  dark-mode lightness band (validated against the panel surface). */
const SERIES = '#b38a24'
const GRID = 'rgb(255 255 255 / 0.08)'
const BASELINE = 'rgb(255 255 255 / 0.22)'

export default function Analytics() {
  const toast = useToast()
  const [stats, setStats] = useState<PlayerStats | null>(null)

  useEffect(() => {
    api.me
      .stats()
      .then(setStats)
      .catch((err) => toast(err instanceof Error ? err.message : 'Could not load your statistics.', 'error'))
  }, [toast])

  return (
    <PageShell title="Analytics" path="/analytics">
      {!stats ? (
        <Spinner label="Crunching your hands…" />
      ) : stats.hands === 0 ? (
        <section className="card px-4 py-8 text-center">
          <p className="font-heading text-lg font-semibold">Nothing to analyse yet</p>
          <p className="mt-1 text-sm text-muted">Play a few hands and your results will show up here.</p>
        </section>
      ) : (
        <>
          <section className="card px-4 py-4">
            <p className="text-xs text-muted">Net result, all tables</p>
            <NetValue net={stats.net} className="font-heading text-5xl font-extrabold" />
            <p className="mt-1 text-xs text-muted">
              Across {stats.hands.toLocaleString()} hands at {stats.rooms} table{stats.rooms === 1 ? '' : 's'} ·
              put in {stats.invested.toLocaleString()}, took {stats.won.toLocaleString()} after rake
            </p>
          </section>

          <section className="grid grid-cols-2 gap-2 sm:grid-cols-4">
            <StatTile label="Hands played" value={stats.hands.toLocaleString()} />
            <StatTile label="Hands won" value={percent(stats.hands_won, stats.hands)} detail={`${stats.hands_won} of ${stats.hands}`} />
            <StatTile
              label="Showdowns won"
              value={percent(stats.showdowns_won, stats.showdowns)}
              detail={`${stats.showdowns_won} of ${stats.showdowns}`}
            />
            <StatTile label="Folded" value={percent(stats.hands_folded, stats.hands)} detail={`${stats.hands_folded} hands`} />
            <StatTile label="Biggest win" value={formatNet(stats.biggest_win)} />
            <StatTile label="Biggest loss" value={formatNet(stats.biggest_loss)} />
            <StatTile label="Tables played" value={stats.rooms.toLocaleString()} />
            <StatTile label="Best hand won" value={stats.best_hand || '—'} />
          </section>

          <section className="card px-4 py-3">
            <h2 className="font-heading text-sm font-semibold">Profit over your last {stats.trend.length} hands</h2>
            <p className="text-xs text-muted">Running total of chips won and lost, after rake</p>
            <TrendChart points={stats.trend} />
            <DataTable
              head={['Hand', 'Result', 'Running total']}
              rows={stats.trend
                .map((p, i) => [`${i + 1}`, formatNet(p.net), formatNet(p.cumulative)])
                .reverse()}
            />
          </section>

          <section className="card px-4 py-3">
            <h2 className="font-heading text-sm font-semibold">Hands you won with at showdown</h2>
            <p className="text-xs text-muted">Strongest first</p>
            {stats.winning_hands.length ? (
              <>
                <CategoryBars data={stats.winning_hands} />
                <DataTable
                  head={['Hand', 'Times won']}
                  rows={stats.winning_hands.map((h) => [h.category, h.times.toLocaleString()])}
                />
              </>
            ) : (
              <p className="py-4 text-sm text-muted">No showdown wins yet.</p>
            )}
          </section>

          <Link to="/history" className="btn-ghost self-center px-4 py-2 text-sm">
            See every hand in Game history
          </Link>
        </>
      )}
    </PageShell>
  )
}

function percent(part: number, whole: number): string {
  if (!whole) return '—'
  return `${Math.round((part / whole) * 100)}%`
}

function StatTile({ label, value, detail }: { label: string; value: string; detail?: string }) {
  return (
    <div className="card px-3 py-2.5">
      <p className="text-[11px] text-muted">{label}</p>
      <p className="font-heading text-xl font-semibold text-chalk">{value}</p>
      {detail ? <p className="text-[11px] text-muted">{detail}</p> : null}
    </div>
  )
}

/** Clean axis ticks: 0, ±1/2/5 × 10ⁿ steps spanning [min, max]. */
function niceTicks(min: number, max: number, count = 4): number[] {
  const span = max - min || 1
  const raw = span / count
  const pow = 10 ** Math.floor(Math.log10(raw))
  const step = [1, 2, 5, 10].map((m) => m * pow).find((s) => s >= raw) ?? raw
  // From the step at or below min to the step at or above max, so every
  // point sits inside the gridlines.
  const ticks: number[] = []
  const top = Math.ceil(max / step) * step
  for (let v = Math.floor(min / step) * step; v <= top + step * 0.001; v += step) ticks.push(Math.round(v))
  return ticks
}

function compact(n: number): string {
  const a = Math.abs(n)
  const sign = n < 0 ? '−' : ''
  if (a >= 1_000_000) return `${sign}${+(a / 1_000_000).toFixed(1)}M`
  if (a >= 1_000) return `${sign}${+(a / 1_000).toFixed(1)}K`
  return `${sign}${a}`
}

function TrendChart({ points }: { points: PlayerStats['trend'] }) {
  const [ref, size] = useElementSize<HTMLDivElement>()
  const [hover, setHover] = useState<number | null>(null)
  const W = size.width
  const H = 190
  const pad = { top: 14, right: 56, bottom: 22, left: 42 }

  const geo = useMemo(() => {
    if (!W || points.length === 0) return null
    const values = points.map((p) => p.cumulative)
    const ticks = niceTicks(Math.min(0, ...values), Math.max(0, ...values))
    const lo = ticks[0]
    const hi = ticks[ticks.length - 1]
    const innerW = W - pad.left - pad.right
    const innerH = H - pad.top - pad.bottom
    const x = (i: number) => pad.left + (points.length === 1 ? innerW : (i / (points.length - 1)) * innerW)
    const y = (v: number) => pad.top + (1 - (v - lo) / (hi - lo || 1)) * innerH
    const line = points.map((p, i) => `${i ? 'L' : 'M'}${x(i).toFixed(1)},${y(p.cumulative).toFixed(1)}`).join('')
    const area = `${line}L${x(points.length - 1).toFixed(1)},${y(0)}L${x(0).toFixed(1)},${y(0)}Z`
    return { ticks, x, y, line, area, innerW }
  }, [W, points, pad.left, pad.right, pad.top, pad.bottom])

  const last = points.length - 1

  function onMove(clientX: number, rect: DOMRect) {
    if (!geo) return
    const rel = (clientX - rect.left - pad.left) / geo.innerW
    setHover(Math.max(0, Math.min(last, Math.round(rel * last))))
  }

  return (
    <div ref={ref} className="relative mt-2 select-none">
      {geo ? (
        <svg
          width={W}
          height={H}
          className="block touch-none"
          role="img"
          aria-label={`Running total over ${points.length} hands, ending at ${formatNet(points[last].cumulative)}`}
          onPointerMove={(e) => onMove(e.clientX, e.currentTarget.getBoundingClientRect())}
          onPointerDown={(e) => onMove(e.clientX, e.currentTarget.getBoundingClientRect())}
          onPointerLeave={() => setHover(null)}
        >
          {geo.ticks.map((t) => (
            <g key={t}>
              <line x1={pad.left} x2={W - pad.right} y1={geo.y(t)} y2={geo.y(t)} stroke={t === 0 ? BASELINE : GRID} strokeWidth="1" />
              <text x={pad.left - 6} y={geo.y(t)} dy="0.32em" textAnchor="end" className="fill-muted text-[10px] tabular-nums">
                {compact(t)}
              </text>
            </g>
          ))}
          <text x={pad.left} y={H - 6} className="fill-muted text-[10px]">
            Oldest
          </text>
          <text x={W - pad.right} y={H - 6} textAnchor="end" className="fill-muted text-[10px]">
            Latest
          </text>

          <path d={geo.area} fill={SERIES} opacity="0.1" />
          <path d={geo.line} fill="none" stroke={SERIES} strokeWidth="2" strokeLinejoin="round" strokeLinecap="round" />

          {/* End-dot and its value: the one label the line carries. */}
          <circle cx={geo.x(last)} cy={geo.y(points[last].cumulative)} r="4.5" fill={SERIES} stroke="var(--color-panel)" strokeWidth="2" />
          <text
            x={geo.x(last) + 9}
            y={geo.y(points[last].cumulative)}
            dy="0.32em"
            className="fill-chalk text-[11px] font-semibold tabular-nums"
          >
            {formatNet(points[last].cumulative)}
          </text>

          {hover !== null ? (
            <g pointerEvents="none">
              <line x1={geo.x(hover)} x2={geo.x(hover)} y1={pad.top} y2={H - pad.bottom} stroke={BASELINE} strokeWidth="1" />
              <circle cx={geo.x(hover)} cy={geo.y(points[hover].cumulative)} r="4.5" fill={SERIES} stroke="var(--color-panel)" strokeWidth="2" />
            </g>
          ) : null}
        </svg>
      ) : (
        <div style={{ height: H }} />
      )}

      {geo && hover !== null ? (
        <div
          className="pointer-events-none absolute top-0 z-10 rounded-lg border border-white/12 bg-ink/95 px-2.5 py-1.5 text-[11px] shadow-deep"
          style={{
            left: Math.min(Math.max(geo.x(hover) - 70, 0), W - 140),
            width: 140,
          }}
        >
          <p className="text-muted">
            Hand {hover + 1} of {points.length} · {new Date(points[hover].at).toLocaleString(undefined, { day: 'numeric', month: 'short', hour: '2-digit', minute: '2-digit' })}
          </p>
          <p className="text-chalk">
            This hand <span className="font-semibold tabular-nums">{formatNet(points[hover].net)}</span>
          </p>
          <p className="text-chalk">
            Running total <span className="font-semibold tabular-nums">{formatNet(points[hover].cumulative)}</span>
          </p>
        </div>
      ) : null}
    </div>
  )
}

function CategoryBars({ data }: { data: PlayerStats['winning_hands'] }) {
  const [hover, setHover] = useState<string | null>(null)
  const max = Math.max(...data.map((d) => d.times))

  return (
    <ul className="mt-3 flex flex-col gap-2">
      {data.map((d) => (
        <li
          key={d.category}
          className="relative grid grid-cols-[7.5rem_1fr] items-center gap-2"
          onPointerEnter={() => setHover(d.category)}
          onPointerLeave={() => setHover(null)}
        >
          <span className="truncate text-xs text-white/75">{d.category}</span>
          <span className="flex items-center gap-2">
            <span
              className="h-[18px] rounded-r-[4px]"
              style={{ width: `${Math.max(2, (d.times / max) * 85)}%`, background: SERIES }}
            />
            <span className="text-xs font-semibold text-chalk tabular-nums">{d.times}</span>
          </span>
          {hover === d.category ? (
            <span className="pointer-events-none absolute -top-7 left-[7.5rem] z-10 rounded-lg border border-white/12 bg-ink/95 px-2 py-1 text-[11px] whitespace-nowrap text-chalk shadow-deep">
              Won {d.times} time{d.times === 1 ? '' : 's'} with {d.category.toLowerCase()}
            </span>
          ) : null}
        </li>
      ))}
    </ul>
  )
}

/** The chart's numbers as a plain table, for anyone who'd rather read than look. */
function DataTable({ head, rows }: { head: string[]; rows: string[][] }) {
  return (
    <details className="mt-2 text-xs">
      <summary className="cursor-pointer text-muted hover:text-chalk">Show data</summary>
      <div className="mt-2 max-h-60 overflow-y-auto rounded-lg border border-white/8">
        <table className="w-full text-left">
          <thead className="sticky top-0 bg-panel text-muted">
            <tr>
              {head.map((h) => (
                <th key={h} className="px-2 py-1 font-medium">
                  {h}
                </th>
              ))}
            </tr>
          </thead>
          <tbody className="tabular-nums">
            {rows.map((r, i) => (
              <tr key={i} className="border-t border-white/6">
                {r.map((c, j) => (
                  <td key={j} className="px-2 py-1">
                    {c}
                  </td>
                ))}
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </details>
  )
}
