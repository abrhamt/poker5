/** Shapes mirrored from the Go API. Keep in sync with:
 *  - GameEngine.ToDict()            (engine_state.go)
 *  - GameService.GetTableState()    (services/game_service_actions.go)
 *  - the JSON handlers under handlers/
 */

export interface User {
  id: number
  username: string
  phone_number: string
  wallet: number
  referral_code: string
  role?: string
  created_at?: string
}

export interface Transaction {
  id: number
  /** Signed: credits are positive, debits (buy-ins, rake) negative. */
  amount: number
  type: string
  reason: string
  transaction_id: string
  created_at: string
}

export interface TransactionPage {
  transactions: Transaction[]
  page: number
  page_size: number
  total: number
  total_pages: number
}

export interface BlindTier {
  small_blind: number
  big_blind: number
}

export interface PublicRoom {
  room_code: string
  room_name: string
  buy_in: number
  small_blind: number
  big_blind: number
  max_players: number
  /** Live seat/countdown data, present once the table has been instantiated. */
  player_count: number
  game_started: boolean
  countdown_active: boolean
  countdown_ends_at_ms: number
}

export interface SeatPlayer {
  name: string
  chips: number
  round_bet: number
  total_bet: number
  folded: boolean
  all_in: boolean
  is_bot: boolean
  dealer: boolean
  small_blind: boolean
  big_blind: boolean
  cards: [string, string]
  seat_index: number
  hand_name: string
  win_probability?: number | null
  is_me?: boolean
  /** Holding a seat but dealt out — no cards, no blinds, no turn. */
  sitting_out?: boolean
}

/** One line of the table's action feed. `kind` is one of the engine's
 *  Note* constants: "action" | "result" | "phase" | "system". */
export interface Notification {
  seq: number
  kind: string
  text: string
}

export interface Winner {
  name: string
  seat_index: number
  amount: number
  hand_name: string
  hand_rank: number
  winning_cards: string[] | null
  hole_cards: string[] | null
  board_cards: string[] | null
  is_tie: boolean
  tied_with?: string[]
}

export interface PotAward {
  pot_index: number
  amount: number
  winner: string
  hand_name: string
  cards: string[]
}

export interface TableState {
  room_code: string
  /** Only set by the "table doesn't exist" fallback in GetTableState. */
  status?: 'empty'

  phase: string
  pot: number
  current_bet: number
  last_raise: number
  small_blind: number
  big_blind: number
  community_cards: string[] | null
  players: SeatPlayer[]

  game_started: boolean
  game_finished: boolean
  intermission: boolean
  total_hands: number
  notifications: Notification[] | null
  version: number
  winner: Winner | null
  pot_awards: PotAward[] | null

  buy_in: number
  room_type: 'public' | 'private' | string
  max_players: number
  host_user_id: number
  is_host?: boolean

  turn_remaining_seconds: number
  turn_total_seconds: number
  turn_ends_at_ms: number
  countdown_active: boolean
  countdown_remaining_seconds: number
  countdown_ends_at_ms: number
  intermission_ends_at_ms: number

  current_turn_player?: string
  is_my_turn?: boolean
  is_sitting_out?: boolean
}

export type PokerAction = 'fold' | 'check' | 'call' | 'raise' | 'all-in'

/** The empty state the UI renders before the first fetch resolves. */
export const emptyTableState = (roomCode: string): TableState => ({
  room_code: roomCode,
  phase: 'preflop',
  pot: 0,
  current_bet: 0,
  last_raise: 0,
  small_blind: 0,
  big_blind: 0,
  community_cards: [],
  players: [],
  game_started: false,
  game_finished: false,
  intermission: false,
  total_hands: 0,
  notifications: [],
  version: 0,
  winner: null,
  pot_awards: null,
  buy_in: 0,
  room_type: 'public',
  max_players: 6,
  host_user_id: 0,
  turn_remaining_seconds: 0,
  turn_total_seconds: 25,
  turn_ends_at_ms: 0,
  countdown_active: false,
  countdown_remaining_seconds: 0,
  countdown_ends_at_ms: 0,
  intermission_ends_at_ms: 0,
})

/** What the deposit screen needs before it can render. The account fields are
 *  only present when real deposits are on — while the toggle is off the server
 *  withholds them rather than advertising a half-configured account. */
export type DepositMethod = 'receipt' | 'gateway'

export type DepositInfo = {
  real_deposits_enabled: boolean
  methods?: DepositMethod[]
  account_name?: string
  account_number?: string
  min_amount?: number
  gateway_min_amount?: number
  gateway_max_amount?: number
}

/** A started automatic deposit: the player is sent to `checkout_url` and comes
 *  back to the wallet with `reference` in the query string. */
export type GatewayCheckout = {
  reference: string
  checkout_url: string
  amount: number
}

export type GatewayDepositStatus = {
  reference: string
  status: 'pending' | 'credited' | 'failed' | 'cancelled' | 'expired'
  amount: number
  transaction_id?: string
}

/** The result of submitting a receipt. `pending_review` is not a failure: the
 *  transfer is real and kept on file, it just could not be checked with the
 *  bank yet. */
export type DepositOutcome = {
  status: 'credited' | 'pending_review'
  amount: number
  reference?: string
  transaction_id?: string
  message: string
}
