export function Spinner({ label }: { label?: string }) {
  return (
    <div className="flex min-h-[40vh] flex-col items-center justify-center gap-3 text-muted">
      <span className="size-8 animate-spin rounded-full border-2 border-white/10 border-t-gold" />
      {label ? <span className="text-sm">{label}</span> : null}
    </div>
  )
}
