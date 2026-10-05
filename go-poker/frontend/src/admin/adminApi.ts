import { post, request } from '../lib/api'
import type { Withdrawal, WithdrawalStatus } from '../lib/types'

/** Shapes mirrored from handlers/admin_handler.go. */

export interface Page<T> {
  items: T[]
  total: number
  page: number
  page_size: number
}

export interface AdminOverview {
  earnings: {
    house_balance: number
    rake_today: number
    rake_week: number
    rake_month: number
    referral_paid_today: number
    referral_paid_week: number
    referral_paid_month: number
  }
  pending_deposits: number
  pending_withdrawals: number
  pending_withdrawal_total: number
  total_users: number
}

export interface AdminSettings {
  rake_mode: 'percentage' | 'small_blind'
  rake_percentage: number
  referral_percentage_pct_mode: number
  referral_percentage_sb_mode: number
  countdown_seconds: number
  intermission_seconds: number
  real_deposits_enabled: boolean
  deposit_account_name: string
  deposit_account_number: string
  gateway_deposits_enabled: boolean
  withdrawals_enabled: boolean
  withdrawal_min_amount: number
  withdrawal_cancel_enabled: boolean
  withdrawal_reset_cooldown_hours: number
  public_refill_enabled: boolean
  updated_at?: string
}

type Range = { min: number; max: number }

export interface SettingsLimits {
  rake_percentage: Range
  referral_percentage: Range
  countdown_seconds: Range
  intermission_seconds: Range
  deposit_account_name: { max_length: number }
  deposit_account_number: { max_length: number }
  withdrawal_min_amount: Range
  withdrawal_reset_cooldown_hours: Range
}

export interface AdminUser {
  id: number
  username: string
  phone_number: string
  wallet: number
  referral_code: string
  referred_by: string
  role: string
  created_at: string
}

export interface AdminTransaction {
  id: number
  user_id?: number
  username?: string
  type: string
  amount: number
  reason: string
  transaction_id: string
  created_at: string
}

export interface QueuedDeposit {
  id: number
  user_id: number
  username: string
  receipt_url: string
  note: string
  attempts: number
  gave_up: boolean
  submitted_at: string
}

export interface DepositQueue extends Page<QueuedDeposit> {
  manual_min: number
  manual_max: number
}

export interface AdminWithdrawal extends Withdrawal {
  user_id: number
  username: string
  phone_number: string
  resolved_by: string
  debit_transaction_id: string
  refund_transaction_id: string
}

const qs = (params: Record<string, string | number | undefined>) => {
  const query = new URLSearchParams()
  for (const [key, value] of Object.entries(params)) {
    if (value !== undefined && value !== '') query.set(key, String(value))
  }
  const text = query.toString()
  return text ? `?${text}` : ''
}

export const adminApi = {
  overview: () => request<AdminOverview>('/api/admin/overview'),

  settings: () => request<{ settings: AdminSettings; limits: SettingsLimits }>('/api/admin/settings'),
  saveSettings: (settings: AdminSettings) =>
    post<{ settings: AdminSettings; limits: SettingsLimits }>('/api/admin/settings', settings),

  users: (query: string, page: number) => request<Page<AdminUser>>(`/api/admin/users${qs({ query, page })}`),
  user: (id: number) =>
    request<{ user: AdminUser; transactions: AdminTransaction[] }>(`/api/admin/users/${id}`),

  transactions: (query: string, type: string, page: number) =>
    request<Page<AdminTransaction>>(`/api/admin/transactions${qs({ query, type, page })}`),

  deposits: (page: number) => request<DepositQueue>(`/api/admin/deposits${qs({ page })}`),
  approveDeposit: (id: number) => post<{ message: string }>(`/api/admin/deposits/${id}/approve`),
  rejectDeposit: (id: number, note: string) =>
    post<{ message: string }>(`/api/admin/deposits/${id}/reject`, { note }),
  creditDeposit: (id: number, amount: number, reference: string) =>
    post<{ message: string }>(`/api/admin/deposits/${id}/credit`, { amount, reference }),

  withdrawals: (status: WithdrawalStatus | '', page: number, userId?: number) =>
    request<Page<AdminWithdrawal>>(`/api/admin/withdrawals${qs({ status, page, user_id: userId })}`),
  completeWithdrawal: (id: number, reference: string) =>
    post<{ withdrawal: AdminWithdrawal }>(`/api/admin/withdrawals/${id}/complete`, { reference }),
  rejectWithdrawal: (id: number, reason: string) =>
    post<{ withdrawal: AdminWithdrawal }>(`/api/admin/withdrawals/${id}/reject`, { reason }),
}
