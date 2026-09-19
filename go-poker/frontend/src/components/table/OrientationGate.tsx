/**
 * The tap target that gets the game into landscape fullscreen. It has to be a
 * real user gesture — browsers refuse `requestFullscreen` without one — so the
 * table shows this as soon as it opens, and again any time the phone is turned
 * back to portrait.
 */
export function OrientationGate({
  isPortrait,
  onEnter,
}: {
  isPortrait: boolean
  onEnter: () => void
}) {
  return (
    <div className="fixed inset-0 z-100 grid place-items-center bg-ink px-6 text-center">
      <div className="flex flex-col items-center gap-5">
        <span className={`text-gold ${isPortrait ? 'animate-rotate-hint' : ''}`}>
          <svg viewBox="0 0 24 24" width="64" height="64" fill="none" stroke="currentColor" strokeWidth="1.6" strokeLinecap="round" strokeLinejoin="round">
            <rect x="7" y="2" width="10" height="20" rx="2" />
            <path d="M11 18h2" />
          </svg>
        </span>

        <div>
          <h1 className="font-heading text-xl font-bold">Rotate your device</h1>
          <p className="mt-1.5 max-w-xs text-sm text-muted">
            The table plays in landscape. Turn your phone sideways, then tap below to go
            full screen.
          </p>
        </div>

        <button type="button" onClick={onEnter} className="btn-gold px-7 py-3 text-base">
          Rotate & Play
        </button>
      </div>
    </div>
  )
}
