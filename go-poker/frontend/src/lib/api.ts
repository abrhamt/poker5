import type { PokerAction, PublicRoom, TableState, TransactionPage, User, BlindTier } from './types'

/** Thrown for any non-2xx response, carrying the server's `error` message so
 *  callers can render it verbatim instead of a generic failure string. */
export class ApiError extends Error {
  constructor(
    message: string,
    readonly status: number,
  ) {
    super(message)
    this.name = 'ApiError'
  }
}

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  let res: Response
  try {
    res = await fetch(path, {
      credentials: 'include',
      headers: init?.body ? { 'Content-Type': 'application/json' } : undefined,
      ...init,
    })
  } catch {
    throw new ApiError('Network unreachable — check your connection.', 0)
  }

  const text = await res.text()
  let body: unknown = null
  if (text) {
    try {
      body = JSON.parse(text)
    } catch {
      body = null
    }
  }

  if (!res.ok) {
    const message =
      (body as { error?: string } | null)?.error ?? `Request failed (${res.status})`
    throw new ApiError(message, res.status)
  }

  return body as T
}

const post = <T,>(path: string, payload?: unknown) =>
  request<T>(path, {
    method: 'POST',
    body: payload === undefined ? undefined : JSON.stringify(payload),
  })

export const api = {
  auth: {
    me: () => request<{ user: User }>('/api/auth/me').then((r) => r.user),
    login: (login: string, password: string) =>
      post<{ token: string; user: User }>('/api/auth/login', { login, password }),
    register: (input: {
      username: string
      phone_number: string
      password: string
      referral_code?: string
    }) => post<{ user: User }>('/api/auth/register', input),
    logout: () => post<{ message: string }>('/api/auth/logout'),
  },

  rooms: {
    listPublic: () =>
      request<{ rooms: PublicRoom[]; tiers: BlindTier[]; active_room_code: string }>(
        '/api/rooms/public',
      ),
    quickJoin: (smallBlind: number) =>
      post<{ room_code: string }>('/api/rooms/quick-join', { small_blind: smallBlind }),
    createPrivate: (input: { room_name?: string; small_blind: number; max_players: number }) =>
      post<{ room_code: string }>('/api/rooms/create-private', input),
    joinByCode: (code: string) => post<{ room_code: string }>('/api/rooms/join-code', { code }),
  },

  table: {
    state: (roomCode: string) => request<TableState>(`/api/table/${roomCode}/state`),
    join: (roomCode: string) => post<unknown>(`/api/table/${roomCode}/join`),
    leave: (roomCode: string) => post<unknown>(`/api/table/${roomCode}/leave`),
    start: (roomCode: string) => post<unknown>(`/api/table/${roomCode}/start`),
    sitIn: (roomCode: string) => post<unknown>(`/api/table/${roomCode}/sit-in`),
    act: (roomCode: string, action: PokerAction, amount = 0) =>
      post<unknown>(`/api/table/${roomCode}/act`, { action, amount }),
  },

  wallet: {
    transactions: (page = 1, pageSize?: number) => {
      const query = new URLSearchParams({ page: String(page) })
      if (pageSize) query.set('page_size', String(pageSize))
      return request<TransactionPage>(`/api/wallet/transactions?${query}`)
    },
    deposit: (amount: number) => post<{ transaction_id: string }>('/api/wallet/deposit', { amount }),
  },
}
