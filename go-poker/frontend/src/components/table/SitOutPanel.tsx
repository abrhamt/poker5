/**
 * Shown to a player whose clock ran out.
 *
 * The table folded once on their behalf and then stopped: they keep their seat
 * and their chips but are dealt out of every hand until they answer this. That
 * consent step is the point — a phone that died mid-session would otherwise
 * keep paying blinds every orbit until the stack was gone.
 */
export function SitOutPanel({ busy, onSitIn }: { busy: boolean; onSitIn: () => void }) {
  return (
    <div className="absolute inset-0 z-60 grid place-items-center bg-ink/70 px-4 backdrop-blur-[3px]">
      <div className="animate-rise card flex max-w-sm flex-col items-center gap-3 px-6 py-5 text-center">
        <span className="grid size-11 place-items-center rounded-full bg-gold/15 text-gold">
          <svg viewBox="0 0 24 24" width="22" height="22" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
            <circle cx="12" cy="12" r="9" />
            <polyline points="12 7 12 12 15 14" />
          </svg>
        </span>

        <h2 className="font-heading text-base font-bold">You've been sat out</h2>

        <p className="text-[13px] leading-relaxed text-muted">
          Your timer ran out, so we stopped playing for you. Your seat and chips are safe — you're
          just dealt out, and paying no blinds, until you're ready.
        </p>

        <button type="button" onClick={onSitIn} disabled={busy} className="btn-gold mt-1 w-full">
          {busy ? 'Sitting back in…' : "I'm back — deal me in"}
        </button>
      </div>
    </div>
  )
}
