import type { BlindTier } from '../../lib/types'
import { TierPiece, tierName } from './TierPiece'

/** What the tiles mean. Kept behind the "?" so the lobby itself stays wordless. */
export function HelpPanel({ tiers }: { tiers: BlindTier[] }) {
  return (
    <div className="flex flex-col gap-4 text-[13px] leading-relaxed text-muted">
      <section>
        <h3 className="mb-1.5 text-xs font-semibold tracking-wide text-chalk uppercase">
          Stake levels
        </h3>
        <ul className="flex flex-col gap-1">
          {tiers.map((tier, i) => (
            <li key={tier.small_blind} className="flex items-center gap-2.5">
              <span className="flex w-5 justify-center text-gold-light">
                <TierPiece index={i} size={16} />
              </span>
              <span className="text-chalk">{tierName(i)}</span>
              <span className="tabular-nums">
                blinds {tier.small_blind}/{tier.big_blind}
              </span>
            </li>
          ))}
        </ul>
        <p className="mt-1.5">
          The blinds are the forced bets that start each hand — bigger blinds mean a bigger
          game.
        </p>
      </section>

      <section>
        <h3 className="mb-1.5 text-xs font-semibold tracking-wide text-chalk uppercase">
          On each table
        </h3>
        <dl className="flex flex-col gap-1.5">
          <Row
            term={
              <span className="flex gap-[3px]">
                <span className="h-1.5 w-4 rounded-full bg-emerald-400" />
                <span className="h-1.5 w-4 rounded-full bg-white/12" />
              </span>
            }
            desc="One bar per seat. Filled bars are taken; they turn red when the table is full."
          />
          <Row term={<span className="font-bold text-chalk tabular-nums">1,000</span>} desc="The buy-in: what leaves your wallet and becomes your chips at that table." />
          <Row
            term={<span className="rounded-full bg-gold/20 px-1.5 py-px text-[10px] font-bold text-gold-light">9s</span>}
            desc="A hand is about to be dealt. Sit down before it hits zero to be in it."
          />
          <Row
            term={<span className="size-1.5 rounded-full bg-emerald-400" />}
            desc="A hand is in progress. You can still open the table and take a seat once it finishes."
          />
        </dl>
      </section>

      <section>
        <h3 className="mb-1.5 text-xs font-semibold tracking-wide text-chalk uppercase">
          Getting seated
        </h3>
        <p>
          <span className="font-semibold text-chalk">Quick Join</span> drops you at the first
          open table in the level you picked.{' '}
          <span className="font-semibold text-chalk">Private</span> creates your own table with a
          5-digit code to share — everyone brings their whole wallet balance instead of a fixed
          buy-in.
        </p>
      </section>
    </div>
  )
}

function Row({ term, desc }: { term: React.ReactNode; desc: string }) {
  return (
    <div className="flex items-start gap-2.5">
      <dt className="flex w-12 shrink-0 items-center justify-center pt-1">{term}</dt>
      <dd>{desc}</dd>
    </div>
  )
}
