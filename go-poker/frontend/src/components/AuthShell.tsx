import { useState } from 'react'
import type { ReactNode } from 'react'

export function AuthShell({
  title,
  subtitle,
  children,
}: {
  title: string
  subtitle: string
  children: ReactNode
}) {
  return (
    <div className="flex min-h-screen items-center justify-center px-4 py-10">
      <div className="card w-full max-w-md p-7 sm:p-9">
        <div className="mb-7 flex flex-col items-center text-center">
          <span className="mb-4 grid size-16 place-items-center rounded-2xl bg-gold/10 text-gold shadow-gold">
            <svg viewBox="0 0 24 24" width="36" height="36" fill="currentColor">
              <path d="M12 2C9 7 4 9 4 14a6 6 0 0 0 10.5 4l-.5 4h2l-.5-4A6 6 0 0 0 20 14c0-5-5-7-8-12z" />
            </svg>
          </span>
          <h1 className="text-2xl font-bold tracking-tight">{title}</h1>
          <p className="mt-1 text-sm text-muted">{subtitle}</p>
        </div>
        {children}
      </div>
    </div>
  )
}

export function ErrorNote({ message }: { message: string }) {
  if (!message) return null
  return (
    <div
      role="alert"
      className="animate-rise rounded-xl border border-danger/40 bg-danger/10 px-3.5 py-2.5 text-sm text-red-200"
    >
      {message}
    </div>
  )
}

export function PasswordField({
  id,
  label,
  autoComplete,
}: {
  id: string
  label: string
  autoComplete: string
}) {
  const [visible, setVisible] = useState(false)

  return (
    <div>
      <label className="label" htmlFor={id}>
        {label}
      </label>
      <div className="relative">
        <input
          id={id}
          name={id}
          type={visible ? 'text' : 'password'}
          required
          autoComplete={autoComplete}
          placeholder="••••••••"
          className="field pr-16"
        />
        <button
          type="button"
          onClick={() => setVisible((v) => !v)}
          aria-label={visible ? 'Hide password' : 'Show password'}
          className="absolute inset-y-0 right-2 my-auto h-7 rounded-lg px-2 text-xs font-semibold text-muted hover:text-gold-light"
        >
          {visible ? 'Hide' : 'Show'}
        </button>
      </div>
    </div>
  )
}
