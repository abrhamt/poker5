import { useEffect, useState } from 'react'
import type { ReactNode } from 'react'
import { adminApi } from '../adminApi'
import type { AdminSettings, SettingsLimits } from '../adminApi'
import { ConfirmDialog } from '../../components/ConfirmDialog'
import { useToast } from '../../components/Toast'
import { ErrorState, formatDate, LoadingBlock, PageHeader, Panel, Toggle, useAsync } from '../ui'

export default function Settings() {
  const toast = useToast()
  const { data, error, reload } = useAsync(() => adminApi.settings(), [])
  const [draft, setDraft] = useState<AdminSettings | null>(null)
  const [confirming, setConfirming] = useState(false)
  const [saving, setSaving] = useState(false)

  useEffect(() => {
    if (data) setDraft(data.settings)
  }, [data])

  if (error) return <ErrorState message={error} onRetry={() => void reload()} />
  if (!data || !draft) return <LoadingBlock />

  const limits = data.limits
  const dirty = JSON.stringify(draft) !== JSON.stringify(data.settings)
  const problems = validate(draft, limits)

  function set<K extends keyof AdminSettings>(key: K, value: AdminSettings[K]) {
    setDraft((d) => (d ? { ...d, [key]: value } : d))
  }

  async function save() {
    if (!draft) return
    setConfirming(false)
    setSaving(true)
    try {
      await adminApi.saveSettings(draft)
      toast('Settings saved.', 'success')
      await reload()
    } catch (err) {
      toast(err instanceof Error ? err.message : 'Could not save settings.', 'error')
    } finally {
      setSaving(false)
    }
  }

  return (
    <div className="pb-24">
      <PageHeader
        title="Settings"
        description={data.settings.updated_at ? `Last saved ${formatDate(data.settings.updated_at)}. Changes apply site-wide immediately.` : undefined}
      />

      <div className="grid gap-6 xl:grid-cols-2">
        <Panel title="Game">
          <div className="flex flex-col gap-4 p-4">
            <Field label="Rake mode">
              <select className="field" value={draft.rake_mode} onChange={(e) => set('rake_mode', e.target.value as AdminSettings['rake_mode'])}>
                <option value="percentage">Percentage of the pot</option>
                <option value="small_blind">Site always takes the small blind</option>
              </select>
            </Field>
            <NumberField
              label="Rake percentage"
              hint={`Used in percentage mode. ${limits.rake_percentage.min}–${limits.rake_percentage.max}%.`}
              value={draft.rake_percentage}
              step={0.1}
              onChange={(v) => set('rake_percentage', v)}
              disabled={draft.rake_mode !== 'percentage'}
            />
            <div className="grid gap-4 sm:grid-cols-2">
              <NumberField
                label="Referral % (percentage mode)"
                value={draft.referral_percentage_pct_mode}
                step={0.1}
                onChange={(v) => set('referral_percentage_pct_mode', v)}
              />
              <NumberField
                label="Referral % (small-blind mode)"
                value={draft.referral_percentage_sb_mode}
                step={0.1}
                onChange={(v) => set('referral_percentage_sb_mode', v)}
              />
            </div>
            <Toggle
              label="Refill on public tables"
              description="Lets seated players add chips from their wallet between hands. Private tables always allow it."
              checked={draft.public_refill_enabled}
              onChange={(v) => set('public_refill_enabled', v)}
            />
            <div className="grid gap-4 sm:grid-cols-2">
              <NumberField
                label="Countdown before a hand (s)"
                hint={`${limits.countdown_seconds.min}–${limits.countdown_seconds.max} seconds.`}
                value={draft.countdown_seconds}
                onChange={(v) => set('countdown_seconds', v)}
              />
              <NumberField
                label="Pause between hands (s)"
                hint={`${limits.intermission_seconds.min}–${limits.intermission_seconds.max} seconds.`}
                value={draft.intermission_seconds}
                onChange={(v) => set('intermission_seconds', v)}
              />
            </div>
          </div>
        </Panel>

        <Panel title="Withdrawals">
          <div className="flex flex-col gap-4 p-4">
            <Toggle
              label="Accept withdrawal requests"
              description="When off, players cannot request a payout. Requests already waiting stay in the queue."
              checked={draft.withdrawals_enabled}
              onChange={(v) => set('withdrawals_enabled', v)}
            />
            <Toggle
              label="Players can cancel a pending request"
              description="Cancelling returns the amount to their wallet. Turn off if you pay out in batches and need requests to stay put."
              checked={draft.withdrawal_cancel_enabled}
              onChange={(v) => set('withdrawal_cancel_enabled', v)}
            />
            <div className="grid gap-4 sm:grid-cols-2">
              <NumberField
                label="Minimum withdrawal (ETB)"
                hint={`${limits.withdrawal_min_amount.min.toLocaleString()}–${limits.withdrawal_min_amount.max.toLocaleString()} ETB.`}
                value={draft.withdrawal_min_amount}
                onChange={(v) => set('withdrawal_min_amount', v)}
              />
              <NumberField
                label="Pause after password reset (hours)"
                hint="0 turns the pause off."
                value={draft.withdrawal_reset_cooldown_hours}
                onChange={(v) => set('withdrawal_reset_cooldown_hours', v)}
              />
            </div>
          </div>
        </Panel>

        <Panel title="Deposits" className="xl:col-span-2">
          <div className="grid gap-4 p-4 lg:grid-cols-2">
            <div className="flex flex-col gap-4">
              <Toggle
                label="Real-money deposits by CBE receipt"
                description="Players transfer to the account on the right and paste the confirmation SMS. Off means play money: any amount, credited instantly."
                checked={draft.real_deposits_enabled}
                onChange={(v) => set('real_deposits_enabled', v)}
              />
              <Toggle
                label="Automatic deposits (payment gateway)"
                description="Players pay online and are credited automatically."
                checked={draft.gateway_deposits_enabled}
                onChange={(v) => set('gateway_deposits_enabled', v)}
              />
            </div>
            <div className="flex flex-col gap-4">
              <Field label="Deposit account name" hint="Must match the name on CBE receipts exactly.">
                <input
                  className="field"
                  maxLength={limits.deposit_account_name.max_length}
                  value={draft.deposit_account_name}
                  onChange={(e) => set('deposit_account_name', e.target.value)}
                />
              </Field>
              <Field label="Deposit account number" hint="The full number, as on the CBE account.">
                <input
                  className="field font-mono"
                  maxLength={limits.deposit_account_number.max_length}
                  value={draft.deposit_account_number}
                  onChange={(e) => set('deposit_account_number', e.target.value)}
                />
              </Field>
            </div>
          </div>
        </Panel>
      </div>

      {(dirty || saving) && (
        <div className="fixed inset-x-0 bottom-0 z-30 border-t border-gold/20 bg-ink/90 backdrop-blur-xl lg:left-64">
          <div className="mx-auto flex max-w-6xl flex-wrap items-center justify-between gap-3 px-4 py-3 lg:px-8">
            <p className={`text-sm ${problems.length ? 'text-red-300' : 'text-muted'}`}>
              {problems.length ? problems[0] : 'You have unsaved changes.'}
            </p>
            <div className="flex gap-2">
              <button type="button" className="btn-ghost px-4 py-2 text-sm" disabled={saving} onClick={() => setDraft(data.settings)}>
                Discard
              </button>
              <button
                type="button"
                className="btn-gold px-4 py-2 text-sm"
                disabled={saving || problems.length > 0}
                onClick={() => setConfirming(true)}
              >
                {saving ? 'Saving…' : 'Save changes'}
              </button>
            </div>
          </div>
        </div>
      )}

      <ConfirmDialog
        open={confirming}
        message="Save these settings? They apply to every table and every player immediately."
        confirmLabel="Save"
        onConfirm={() => void save()}
        onCancel={() => setConfirming(false)}
      />
    </div>
  )
}

/** The same bounds the server enforces, checked here so the save button can
 *  say what is wrong before a round-trip does. */
function validate(s: AdminSettings, l: SettingsLimits): string[] {
  const out: string[] = []
  const inRange = (v: number, r: { min: number; max: number }) => Number.isFinite(v) && v >= r.min && v <= r.max
  if (!inRange(s.rake_percentage, l.rake_percentage)) out.push(`Rake percentage must be ${l.rake_percentage.min}–${l.rake_percentage.max}.`)
  if (!inRange(s.referral_percentage_pct_mode, l.referral_percentage) || !inRange(s.referral_percentage_sb_mode, l.referral_percentage))
    out.push(`Referral percentages must be ${l.referral_percentage.min}–${l.referral_percentage.max}.`)
  if (!Number.isInteger(s.countdown_seconds) || !inRange(s.countdown_seconds, l.countdown_seconds))
    out.push(`Countdown must be ${l.countdown_seconds.min}–${l.countdown_seconds.max} seconds.`)
  if (!Number.isInteger(s.intermission_seconds) || !inRange(s.intermission_seconds, l.intermission_seconds))
    out.push(`Pause between hands must be ${l.intermission_seconds.min}–${l.intermission_seconds.max} seconds.`)
  if (!Number.isInteger(s.withdrawal_min_amount) || !inRange(s.withdrawal_min_amount, l.withdrawal_min_amount))
    out.push(`Minimum withdrawal must be ${l.withdrawal_min_amount.min}–${l.withdrawal_min_amount.max.toLocaleString()} ETB.`)
  if (!Number.isInteger(s.withdrawal_reset_cooldown_hours) || !inRange(s.withdrawal_reset_cooldown_hours, l.withdrawal_reset_cooldown_hours))
    out.push(`The password-reset pause must be ${l.withdrawal_reset_cooldown_hours.min}–${l.withdrawal_reset_cooldown_hours.max} hours.`)
  if (s.real_deposits_enabled && (!s.deposit_account_name.trim() || !s.deposit_account_number.trim()))
    out.push('Set the deposit account name and number before enabling receipt deposits.')
  return out
}

function Field({ label, hint, children }: { label: string; hint?: string; children: ReactNode }) {
  return (
    <div>
      <label className="label">{label}</label>
      {children}
      {hint && <p className="mt-1 text-[11px] text-muted">{hint}</p>}
    </div>
  )
}

function NumberField({
  label,
  hint,
  value,
  step = 1,
  disabled,
  onChange,
}: {
  label: string
  hint?: string
  value: number
  step?: number
  disabled?: boolean
  onChange: (value: number) => void
}) {
  return (
    <Field label={label} hint={hint}>
      <input
        type="number"
        inputMode="decimal"
        step={step}
        disabled={disabled}
        className="field tabular-nums disabled:opacity-50"
        value={Number.isFinite(value) ? value : ''}
        onChange={(e) => onChange(e.target.value === '' ? Number.NaN : Number(e.target.value))}
      />
    </Field>
  )
}
