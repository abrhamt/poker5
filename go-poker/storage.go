package poker

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	_ "modernc.org/sqlite"
)

type SQLiteStorage struct {
	db *sql.DB
	mu sync.Mutex
}

func NewSQLiteStorage(dsn string) (*SQLiteStorage, error) {
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	if _, err := db.Exec(Schema); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("apply schema: %w", err)
	}
	return &SQLiteStorage{db: db}, nil
}

func (s *SQLiteStorage) Close() error { return s.db.Close() }

const Schema = `
CREATE TABLE IF NOT EXISTS poker_game (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    table_id TEXT NOT NULL UNIQUE,
    phase TEXT NOT NULL DEFAULT 'preflop',
    pot INTEGER NOT NULL DEFAULT 0,
    current_bet INTEGER NOT NULL DEFAULT 0,
    last_raise INTEGER NOT NULL DEFAULT 20,
    small_blind INTEGER NOT NULL DEFAULT 10,
    big_blind INTEGER NOT NULL DEFAULT 20,
    raises_this_round INTEGER NOT NULL DEFAULT 0,
    dealer_orbit_count INTEGER NOT NULL DEFAULT -1,
    game_started INTEGER NOT NULL DEFAULT 0,
    game_finished INTEGER NOT NULL DEFAULT 0,
    open_cards_mode INTEGER NOT NULL DEFAULT 0,
    spectator_mode INTEGER NOT NULL DEFAULT 0,
    initial_dealer_name TEXT,
    deck TEXT NOT NULL DEFAULT '[]',
    card_graveyard TEXT NOT NULL DEFAULT '[]',
    community_cards TEXT NOT NULL DEFAULT '[]',
    current_player_index INTEGER NOT NULL DEFAULT 0,
    total_hands INTEGER NOT NULL DEFAULT 0,
    created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    version INTEGER NOT NULL DEFAULT 0,
    notifications TEXT NOT NULL DEFAULT '[]',
    intermission INTEGER NOT NULL DEFAULT 0,
    last_winner_json TEXT NOT NULL DEFAULT '{}',
    pot_awards_json TEXT NOT NULL DEFAULT '[]',
    intermission_started_at INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE IF NOT EXISTS poker_player (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    game_id INTEGER NOT NULL,
    name TEXT NOT NULL,
    seat_index INTEGER NOT NULL,
    is_bot INTEGER NOT NULL DEFAULT 0,
    chips INTEGER NOT NULL DEFAULT 2000,
    round_bet INTEGER NOT NULL DEFAULT 0,
    total_bet INTEGER NOT NULL DEFAULT 0,
    folded INTEGER NOT NULL DEFAULT 0,
    all_in INTEGER NOT NULL DEFAULT 0,
    is_dealer INTEGER NOT NULL DEFAULT 0,
    is_small_blind INTEGER NOT NULL DEFAULT 0,
    is_big_blind INTEGER NOT NULL DEFAULT 0,
    card1 TEXT,
    card2 TEXT,
    win_probability REAL,
    stats_data TEXT NOT NULL DEFAULT '{}',
    UNIQUE(game_id, seat_index),
    FOREIGN KEY(game_id) REFERENCES poker_game(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_poker_player_game ON poker_player(game_id);

CREATE TABLE IF NOT EXISTS poker_config (
    key TEXT PRIMARY KEY,
    value TEXT NOT NULL
);
`

func (s *SQLiteStorage) GetOrCreateGame(tableID string) (*Game, []*Player, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	g, err := s.loadByTableID(tableID)
	if err == nil {
		players, err := s.loadPlayers(g.ID)
		if err != nil {
			return nil, nil, false, err
		}
		return g, players, false, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return nil, nil, false, err
	}

	g = NewGame(tableID)
	g.SmallBlind = DefaultSmallBlind
	g.BigBlind = DefaultBigBlind
	g.LastRaise = DefaultLastRaise
	g.CreatedAt = time.Now()
	g.UpdatedAt = time.Now()
	id, err := s.insertGame(g)
	if err != nil {
		return nil, nil, false, err
	}
	g.ID = id
	return g, []*Player{}, true, nil
}

func (s *SQLiteStorage) SaveGame(g *Game, players []*Player) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if g.ID == 0 {
		id, err := s.insertGame(g)
		if err != nil {
			return err
		}
		g.ID = id
	} else {
		if err := s.updateGame(g); err != nil {
			return err
		}
	}

	for _, p := range players {
		p.GameID = g.ID
	}
	return s.upsertPlayers(g.ID, players)
}

func (s *SQLiteStorage) LoadGame(tableID string) (*Game, []*Player, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	g, err := s.loadByTableID(tableID)
	if err != nil {
		return nil, nil, err
	}
	players, err := s.loadPlayers(g.ID)
	if err != nil {
		return nil, nil, err
	}
	return g, players, nil
}

func (s *SQLiteStorage) insertGame(g *Game) (int64, error) {
	deckJSON, _ := json.Marshal(g.Deck)
	graveyardJSON, _ := json.Marshal(g.CardGraveyard)
	communityJSON, _ := json.Marshal(g.CommunityCards)
	notifJSON, _ := json.Marshal(g.Notifications)
	winnerJSON, _ := json.Marshal(emptyWinner(g.LastWinner))
	awardsJSON, _ := json.Marshal(emptyAwards(g.PotAwards))
	res, err := s.db.Exec(`
        INSERT INTO poker_game
        (table_id, phase, pot, current_bet, last_raise, small_blind, big_blind,
         raises_this_round, dealer_orbit_count, game_started, game_finished,
         open_cards_mode, spectator_mode, initial_dealer_name, deck,
         card_graveyard, community_cards, current_player_index, total_hands,
         version, notifications, created_at, updated_at, intermission,
         last_winner_json, pot_awards_json, intermission_started_at)
        VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		g.TableID, g.Phase, g.Pot, g.CurrentBet, g.LastRaise, g.SmallBlind, g.BigBlind,
		g.RaisesThisRound, g.DealerOrbitCount, boolInt(g.GameStarted), boolInt(g.GameFinished),
		boolInt(g.OpenCardsMode), boolInt(g.SpectatorMode), nullString(g.InitialDealerName),
		string(deckJSON), string(graveyardJSON), string(communityJSON), g.CurrentPlayerIdx,
		g.TotalHands, g.Version, string(notifJSON), g.CreatedAt.Format(time.RFC3339Nano),
		g.UpdatedAt.Format(time.RFC3339Nano), boolInt(g.Intermission),
		string(winnerJSON), string(awardsJSON), g.IntermissionStartedAt)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (s *SQLiteStorage) updateGame(g *Game) error {
	deckJSON, _ := json.Marshal(g.Deck)
	graveyardJSON, _ := json.Marshal(g.CardGraveyard)
	communityJSON, _ := json.Marshal(g.CommunityCards)
	notifJSON, _ := json.Marshal(g.Notifications)
	winnerJSON, _ := json.Marshal(emptyWinner(g.LastWinner))
	awardsJSON, _ := json.Marshal(emptyAwards(g.PotAwards))
	g.UpdatedAt = time.Now()
	_, err := s.db.Exec(`
        UPDATE poker_game SET
            phase = ?, pot = ?, current_bet = ?, last_raise = ?, small_blind = ?,
            big_blind = ?, raises_this_round = ?, dealer_orbit_count = ?,
            game_started = ?, game_finished = ?, open_cards_mode = ?,
            spectator_mode = ?, initial_dealer_name = ?, deck = ?,
            card_graveyard = ?, community_cards = ?, current_player_index = ?,
            total_hands = ?, version = ?, notifications = ?, updated_at = ?,
            intermission = ?, last_winner_json = ?, pot_awards_json = ?,
            intermission_started_at = ?
        WHERE id = ?`,
		g.Phase, g.Pot, g.CurrentBet, g.LastRaise, g.SmallBlind,
		g.BigBlind, g.RaisesThisRound, g.DealerOrbitCount,
		boolInt(g.GameStarted), boolInt(g.GameFinished), boolInt(g.OpenCardsMode),
		boolInt(g.SpectatorMode), nullString(g.InitialDealerName), string(deckJSON),
		string(graveyardJSON), string(communityJSON), g.CurrentPlayerIdx,
		g.TotalHands, g.Version, string(notifJSON), g.UpdatedAt.Format(time.RFC3339Nano),
		boolInt(g.Intermission), string(winnerJSON), string(awardsJSON),
		g.IntermissionStartedAt, g.ID)
	return err
}

func (s *SQLiteStorage) loadByTableID(tableID string) (*Game, error) {
	row := s.db.QueryRow(`SELECT id, table_id, phase, pot, current_bet, last_raise,
        small_blind, big_blind, raises_this_round, dealer_orbit_count,
        game_started, game_finished, open_cards_mode, spectator_mode,
        initial_dealer_name, deck, card_graveyard, community_cards,
        current_player_index, total_hands, version, notifications, created_at, updated_at,
        intermission, last_winner_json, pot_awards_json, intermission_started_at
        FROM poker_game WHERE table_id = ?`, tableID)
	g := &Game{}
	var started, finished, openCards, spectator int
	var intermission int
	var initialDealer sql.NullString
	var deckJSON, graveyardJSON, communityJSON, notifJSON string
	var winnerJSON, awardsJSON string
	var createdAt, updatedAt string
	if err := row.Scan(&g.ID, &g.TableID, &g.Phase, &g.Pot, &g.CurrentBet, &g.LastRaise,
		&g.SmallBlind, &g.BigBlind, &g.RaisesThisRound, &g.DealerOrbitCount,
		&started, &finished, &openCards, &spectator, &initialDealer,
		&deckJSON, &graveyardJSON, &communityJSON, &g.CurrentPlayerIdx,
		&g.TotalHands, &g.Version, &notifJSON, &createdAt, &updatedAt,
		&intermission, &winnerJSON, &awardsJSON, &g.IntermissionStartedAt); err != nil {
		return nil, err
	}
	g.GameStarted = started != 0
	g.GameFinished = finished != 0
	g.OpenCardsMode = openCards != 0
	g.SpectatorMode = spectator != 0
	g.Intermission = intermission != 0
	if initialDealer.Valid {
		v := initialDealer.String
		g.InitialDealerName = &v
	}
	_ = json.Unmarshal([]byte(deckJSON), &g.Deck)
	_ = json.Unmarshal([]byte(graveyardJSON), &g.CardGraveyard)
	_ = json.Unmarshal([]byte(communityJSON), &g.CommunityCards)
	_ = json.Unmarshal([]byte(notifJSON), &g.Notifications)
	if winnerJSON != "" && winnerJSON != "{}" {
		_ = json.Unmarshal([]byte(winnerJSON), &g.LastWinner)
	}
	if awardsJSON != "" && awardsJSON != "[]" {
		_ = json.Unmarshal([]byte(awardsJSON), &g.PotAwards)
	}
	if g.Deck == nil {
		g.Deck = []string{}
	}
	if g.CardGraveyard == nil {
		g.CardGraveyard = []string{}
	}
	if g.CommunityCards == nil {
		g.CommunityCards = []string{}
	}
	if g.Notifications == nil {
		g.Notifications = []string{}
	}
	for i, p := range Phases {
		if p == g.Phase {
			g.PhaseIndex = i
		}
	}
	g.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdAt)
	g.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updatedAt)
	return g, nil
}

func emptyWinner(w *Winner) *Winner {
	if w != nil {
		return w
	}
	return &Winner{}
}

func emptyAwards(a []PotAward) []PotAward {
	if a != nil {
		return a
	}
	return []PotAward{}
}

func (s *SQLiteStorage) upsertPlayers(gameID int64, players []*Player) error {
	for _, p := range players {
		statsJSON, _ := json.Marshal(p.Stats)
		card1 := sql.NullString{String: p.Cards[0], Valid: p.Cards[0] != "" && p.Cards[0] != "1B"}
		card2 := sql.NullString{String: p.Cards[1], Valid: p.Cards[1] != "" && p.Cards[1] != "1B"}
		var winProb sql.NullFloat64
		if p.WinProbability != nil {
			winProb = sql.NullFloat64{Float64: *p.WinProbability, Valid: true}
		}
		_, err := s.db.Exec(`
            INSERT INTO poker_player
            (game_id, name, seat_index, is_bot, chips, round_bet, total_bet,
             folded, all_in, is_dealer, is_small_blind, is_big_blind,
             card1, card2, win_probability, stats_data)
            VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
            ON CONFLICT(game_id, seat_index) DO UPDATE SET
                name=excluded.name,
                is_bot=excluded.is_bot,
                chips=excluded.chips,
                round_bet=excluded.round_bet,
                total_bet=excluded.total_bet,
                folded=excluded.folded,
                all_in=excluded.all_in,
                is_dealer=excluded.is_dealer,
                is_small_blind=excluded.is_small_blind,
                is_big_blind=excluded.is_big_blind,
                card1=excluded.card1,
                card2=excluded.card2,
                win_probability=excluded.win_probability,
                stats_data=excluded.stats_data`,
			gameID, p.Name, p.SeatIndex, boolInt(p.IsBot), p.Chips, p.RoundBet, p.TotalBet,
			boolInt(p.Folded), boolInt(p.AllIn), boolInt(p.IsDealer), boolInt(p.IsSmallBlind),
			boolInt(p.IsBigBlind), card1, card2, winProb, string(statsJSON))
		if err != nil {
			return err
		}
	}
	return nil
}

func (s *SQLiteStorage) loadPlayers(gameID int64) ([]*Player, error) {
	rows, err := s.db.Query(`SELECT id, name, seat_index, is_bot, chips, round_bet,
        total_bet, folded, all_in, is_dealer, is_small_blind, is_big_blind,
        card1, card2, win_probability, stats_data
        FROM poker_player WHERE game_id = ? ORDER BY seat_index`, gameID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	players := []*Player{}
	for rows.Next() {
		p := &Player{}
		var isBot, folded, allIn, dealer, sb, bb int
		var card1, card2 sql.NullString
		var winProb sql.NullFloat64
		var statsJSON string
		if err := rows.Scan(&p.ID, &p.Name, &p.SeatIndex, &isBot, &p.Chips, &p.RoundBet,
			&p.TotalBet, &folded, &allIn, &dealer, &sb, &bb,
			&card1, &card2, &winProb, &statsJSON); err != nil {
			return nil, err
		}
		p.IsBot = isBot != 0
		p.Folded = folded != 0
		p.AllIn = allIn != 0
		p.IsDealer = dealer != 0
		p.IsSmallBlind = sb != 0
		p.IsBigBlind = bb != 0
		if card1.Valid {
			p.Cards[0] = card1.String
		} else {
			p.Cards[0] = "1B"
		}
		if card2.Valid {
			p.Cards[1] = card2.String
		} else {
			p.Cards[1] = "1B"
		}
		if winProb.Valid {
			v := winProb.Float64
			p.WinProbability = &v
		}
		_ = json.Unmarshal([]byte(statsJSON), &p.Stats)
		p.BotLine = NewBotLine()
		players = append(players, p)
	}
	return players, rows.Err()
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func nullString(s *string) interface{} {
	if s == nil {
		return nil
	}
	return *s
}

func (s *SQLiteStorage) GetConfig(key string) (string, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var value string
	err := s.db.QueryRow(`SELECT value FROM poker_config WHERE key = ?`, key).Scan(&value)
	if err == sql.ErrNoRows {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return value, true, nil
}

func (s *SQLiteStorage) SetConfig(key, value string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, err := s.db.Exec(`
        INSERT INTO poker_config (key, value) VALUES (?, ?)
        ON CONFLICT(key) DO UPDATE SET value = excluded.value`,
		key, value)
	return err
}

type ConfigStore interface {
	GetConfig(key string) (string, bool, error)
	SetConfig(key, value string) error
}