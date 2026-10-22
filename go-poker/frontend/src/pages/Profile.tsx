import { useEffect, useState } from 'react'
import { api } from '../lib/api'
import { useAuth } from '../lib/auth'
import { useToast } from '../components/Toast'
import { PageShell } from '../components/PageShell'
import { AVATAR_COUNT, PlayerAvatar } from '../components/PlayerAvatar'

/** The player's own account: avatar, username and password. */
export default function Profile() {
  const { user } = useAuth()
  if (!user) return null

  return (
    <PageShell title="Profile" path="/profile">
      <AvatarSection />
      <UsernameSection />
      <PasswordSection />

      <section className="card px-4 py-3 text-sm">
        <h2 className="mb-2 font-heading font-semibold">Account</h2>
        <dl className="grid grid-cols-[auto_1fr] gap-x-4 gap-y-1">
          <dt className="text-muted">Phone</dt>
          <dd>{user.phone_number}</dd>
          {user.created_at ? (
            <>
              <dt className="text-muted">Member since</dt>
              <dd>{new Date(user.created_at).toLocaleDateString(undefined, { day: 'numeric', month: 'long', year: 'numeric' })}</dd>
            </>
          ) : null}
        </dl>
      </section>
    </PageShell>
  )
}

function AvatarSection() {
  const { user, refresh } = useAuth()
  const toast = useToast()
  const [saving, setSaving] = useState<number | null>(null)
  const current = user?.avatar ?? -1
  const name = user?.username ?? ''

  async function choose(avatar: number) {
    if (avatar === current) return
    setSaving(avatar)
    try {
      await api.me.setAvatar(avatar)
      await refresh()
      toast('Avatar updated.', 'success')
    } catch (err) {
      toast(err instanceof Error ? err.message : 'Could not change your avatar.', 'error')
    } finally {
      setSaving(null)
    }
  }

  // "Automatic" is the avatar picked from the username — offered first, since
  // it is what everyone starts with.
  const options = [-1, ...Array.from({ length: AVATAR_COUNT }, (_, i) => i)]

  return (
    <section className="card px-4 py-3">
      <div className="flex items-center gap-3">
        <span className="size-16 shrink-0 overflow-hidden rounded-full ring-2 ring-gold">
          <PlayerAvatar name={name} avatar={current} className="block size-full" />
        </span>
        <div>
          <h2 className="font-heading font-semibold">Avatar</h2>
          <p className="text-xs text-muted">This is how other players see you at the table.</p>
        </div>
      </div>

      <div className="mt-3 grid grid-cols-5 gap-2 sm:grid-cols-7" role="radiogroup" aria-label="Choose an avatar">
        {options.map((a) => {
          const selected = a === current
          return (
            <button
              key={a}
              type="button"
              role="radio"
              aria-checked={selected}
              aria-label={a === -1 ? 'Automatic avatar' : `Avatar ${a + 1}`}
              disabled={saving !== null}
              onClick={() => void choose(a)}
              className={`relative aspect-square overflow-hidden rounded-full transition-[box-shadow,transform] active:scale-95 disabled:opacity-60 ${
                selected ? 'ring-2 ring-gold-bright ring-offset-2 ring-offset-panel' : 'ring-1 ring-white/15 hover:ring-gold/60'
              }`}
            >
              <PlayerAvatar name={name} avatar={a} className="block size-full" />
              {a === -1 ? (
                <span className="absolute inset-x-0 bottom-0 bg-black/65 py-0.5 text-center text-[8px] font-semibold tracking-wide uppercase">
                  Auto
                </span>
              ) : null}
            </button>
          )
        })}
      </div>
    </section>
  )
}

function UsernameSection() {
  const { user, refresh } = useAuth()
  const toast = useToast()
  const [value, setValue] = useState(user?.username ?? '')
  const [busy, setBusy] = useState(false)

  useEffect(() => setValue(user?.username ?? ''), [user?.username])

  async function save(event: React.FormEvent) {
    event.preventDefault()
    setBusy(true)
    try {
      await api.me.setUsername(value.trim())
      await refresh()
      toast('Username updated.', 'success')
    } catch (err) {
      toast(err instanceof Error ? err.message : 'Could not change your username.', 'error')
    } finally {
      setBusy(false)
    }
  }

  const unchanged = value.trim() === user?.username

  return (
    <form onSubmit={save} className="card flex flex-col gap-2 px-4 py-3">
      <h2 className="font-heading font-semibold">Username</h2>
      <label className="label" htmlFor="username">
        Shown at the table and in hand histories
      </label>
      <div className="flex gap-2">
        <input
          id="username"
          className="field"
          value={value}
          onChange={(e) => setValue(e.target.value)}
          autoComplete="username"
          minLength={3}
          maxLength={20}
          pattern="[A-Za-z0-9_]{3,20}"
          title="3–20 letters, numbers or underscores"
          required
        />
        <button type="submit" className="btn-gold shrink-0" disabled={busy || unchanged}>
          {busy ? 'Saving…' : 'Save'}
        </button>
      </div>
      <p className="text-[11px] text-muted">3–20 letters, numbers or underscores. Leave your table before changing it.</p>
    </form>
  )
}

function PasswordSection() {
  const toast = useToast()
  const [busy, setBusy] = useState(false)

  async function save(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault()
    const form = event.currentTarget
    const data = new FormData(form)
    const current = String(data.get('current'))
    const next = String(data.get('next'))
    if (next !== String(data.get('confirm'))) {
      toast('The new passwords do not match.', 'error')
      return
    }
    setBusy(true)
    try {
      await api.me.setPassword(current, next)
      form.reset()
      toast('Password changed. Your other devices have been signed out.', 'success')
    } catch (err) {
      toast(err instanceof Error ? err.message : 'Could not change your password.', 'error')
    } finally {
      setBusy(false)
    }
  }

  return (
    <form onSubmit={save} className="card flex flex-col gap-2 px-4 py-3">
      <h2 className="font-heading font-semibold">Password</h2>
      <div>
        <label className="label" htmlFor="current">
          Current password
        </label>
        <input id="current" name="current" type="password" className="field" autoComplete="current-password" required />
      </div>
      <div>
        <label className="label" htmlFor="next">
          New password
        </label>
        <input id="next" name="next" type="password" className="field" autoComplete="new-password" minLength={6} required />
      </div>
      <div>
        <label className="label" htmlFor="confirm">
          Confirm new password
        </label>
        <input id="confirm" name="confirm" type="password" className="field" autoComplete="new-password" minLength={6} required />
      </div>
      <button type="submit" className="btn-gold mt-1" disabled={busy}>
        {busy ? 'Changing…' : 'Change password'}
      </button>
      <p className="text-[11px] text-muted">You stay signed in here; every other device is signed out.</p>
    </form>
  )
}
