package poker

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	dbschema "github.com/zuse/poker5/go-poker/db"
)

// PostgresStorage persists live table state — the hand in progress, the chips
// in front of each seat — so a restart does not strand seated players' money.
// Their wallet was already debited on buy-in, and an in-memory table simply
// vanishing would leave nothing to recover it from.
type PostgresStorage struct {
	db    *sql.DB
	owned bool
	mu    sync.Mutex
}

// NewPostgresStorage opens its own connection from a DATABASE_URL-style DSN.
func NewPostgresStorage(dsn string) (*PostgresStorage, error) {
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return nil, fmt.Errorf("open postgres: %w", err)
	}
	if err := dbschema.Apply(context.Background(), db); err != nil {
		_ = db.Close()
		return nil, err
	}
	return &PostgresStorage{db: db, owned: true}, nil
}

// NewPostgresStorageFromDB wraps the application's existing pool, so live game
// state lives in the same database as everything else. The passed-in db is not
// closed by Close — the caller retains ownership.
func NewPostgresStorageFromDB(db *sql.DB) (*PostgresStorage, error) {
	if db == nil {
		return nil, errors.New("nil database handle")
	}
	return &PostgresStorage{db: db, owned: false}, nil
}

func (s *PostgresStorage) Close() error {
	if !s.owned {
		return nil
	}
	return s.db.Close()
}

func (s *PostgresStorage) GetOrCreateGame(tableID string) (*Game, []*Player, bool, error) {
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

func (s *PostgresStorage) SaveGame(g *Game, players []*Player) error {
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

func (s *PostgresStorage) LoadGame(tableID string) (*Game, []*Player, error) {
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

// insertGame reads the new id back with RETURNING: PostgreSQL has no
// LastInsertId, and the id is needed immediately to attach players to.
func (s *PostgresStorage) insertGame(g *Game) (int64, error) {
	deckJSON, _ := json.Marshal(g.Deck)
	graveyardJSON, _ := json.Marshal(g.CardGraveyard)
	communityJSON, _ := json.Marshal(g.CommunityCards)
	notifJSON, _ := json.Marshal(g.Notifications)
	winnerJSON, _ := json.Marshal(emptyWinner(g.LastWinner))
	awardsJSON, _ := json.Marshal(emptyAwards(g.PotAwards))

	var id int64
	err := s.db.QueryRow(`
        INSERT INTO poker_game
        (table_id, phase, pot, current_bet, last_raise, small_blind, big_blind,
         raises_this_round, dealer_orbit_count, game_started, game_finished,
         open_cards_mode, spectator_mode, initial_dealer_name, deck,
         card_graveyard, community_cards, current_player_index, total_hands,
         version, notifications, created_at, updated_at, intermission,
         last_winner_json, pot_awards_json, intermission_started_at)
        VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,
                $19,$20,$21,$22,$23,$24,$25,$26,$27)
        RETURNING id`,
		g.TableID, g.Phase, g.Pot, g.CurrentBet, g.LastRaise, g.SmallBlind, g.BigBlind,
		g.RaisesThisRound, g.DealerOrbitCount, g.GameStarted, g.GameFinished,
		g.OpenCardsMode, g.SpectatorMode, nullString(g.InitialDealerName),
		string(deckJSON), string(graveyardJSON), string(communityJSON), g.CurrentPlayerIdx,
		g.TotalHands, g.Version, string(notifJSON), g.CreatedAt, g.UpdatedAt,
		g.Intermission, string(winnerJSON), string(awardsJSON), g.IntermissionStartedAt).Scan(&id)
	return id, err
}

func (s *PostgresStorage) updateGame(g *Game) error {
	deckJSON, _ := json.Marshal(g.Deck)
	graveyardJSON, _ := json.Marshal(g.CardGraveyard)
	communityJSON, _ := json.Marshal(g.CommunityCards)
	notifJSON, _ := json.Marshal(g.Notifications)
	winnerJSON, _ := json.Marshal(emptyWinner(g.LastWinner))
	awardsJSON, _ := json.Marshal(emptyAwards(g.PotAwards))
	g.UpdatedAt = time.Now()
	_, err := s.db.Exec(`
        UPDATE poker_game SET
            phase = $1, pot = $2, current_bet = $3, last_raise = $4, small_blind = $5,
            big_blind = $6, raises_this_round = $7, dealer_orbit_count = $8,
            game_started = $9, game_finished = $10, open_cards_mode = $11,
            spectator_mode = $12, initial_dealer_name = $13, deck = $14,
            card_graveyard = $15, community_cards = $16, current_player_index = $17,
            total_hands = $18, version = $19, notifications = $20, updated_at = $21,
            intermission = $22, last_winner_json = $23, pot_awards_json = $24,
            intermission_started_at = $25
        WHERE id = $26`,
		g.Phase, g.Pot, g.CurrentBet, g.LastRaise, g.SmallBlind,
		g.BigBlind, g.RaisesThisRound, g.DealerOrbitCount,
		g.GameStarted, g.GameFinished, g.OpenCardsMode,
		g.SpectatorMode, nullString(g.InitialDealerName), string(deckJSON),
		string(graveyardJSON), string(communityJSON), g.CurrentPlayerIdx,
		g.TotalHands, g.Version, string(notifJSON), g.UpdatedAt,
		g.Intermission, string(winnerJSON), string(awardsJSON),
		g.IntermissionStartedAt, g.ID)
	return err
}

func (s *PostgresStorage) loadByTableID(tableID string) (*Game, error) {
	row := s.db.QueryRow(`SELECT id, table_id, phase, pot, current_bet, last_raise,
        small_blind, big_blind, raises_this_round, dealer_orbit_count,
        game_started, game_finished, open_cards_mode, spectator_mode,
        initial_dealer_name, deck, card_graveyard, community_cards,
        current_player_index, total_hands, version, notifications, created_at, updated_at,
        intermission, last_winner_json, pot_awards_json, intermission_started_at
        FROM poker_game WHERE table_id = $1`, tableID)
	g := &Game{}
	var initialDealer sql.NullString
	var deckJSON, graveyardJSON, communityJSON, notifJSON string
	var winnerJSON, awardsJSON string
	if err := row.Scan(&g.ID, &g.TableID, &g.Phase, &g.Pot, &g.CurrentBet, &g.LastRaise,
		&g.SmallBlind, &g.BigBlind, &g.RaisesThisRound, &g.DealerOrbitCount,
		&g.GameStarted, &g.GameFinished, &g.OpenCardsMode, &g.SpectatorMode, &initialDealer,
		&deckJSON, &graveyardJSON, &communityJSON, &g.CurrentPlayerIdx,
		&g.TotalHands, &g.Version, &notifJSON, &g.CreatedAt, &g.UpdatedAt,
		&g.Intermission, &winnerJSON, &awardsJSON, &g.IntermissionStartedAt); err != nil {
		return nil, err
	}
	if initialDealer.Valid {
		v := initialDealer.String
		g.InitialDealerName = &v
	}
	_ = json.Unmarshal([]byte(deckJSON), &g.Deck)
	_ = json.Unmarshal([]byte(graveyardJSON), &g.CardGraveyard)
	_ = json.Unmarshal([]byte(communityJSON), &g.CommunityCards)
	g.Notifications = decodeNotifications(notifJSON)
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
		g.Notifications = []Notification{}
	}
	for i, p := range Phases {
		if p == g.Phase {
			g.PhaseIndex = i
		}
	}
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

func (s *PostgresStorage) upsertPlayers(gameID int64, players []*Player) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.Exec(`DELETE FROM poker_player WHERE game_id = $1`, gameID); err != nil {
		return err
	}

	for _, p := range players {
		statsJSON, _ := json.Marshal(p.Stats)
		card1 := sql.NullString{String: p.Cards[0], Valid: p.Cards[0] != "" && p.Cards[0] != "1B"}
		card2 := sql.NullString{String: p.Cards[1], Valid: p.Cards[1] != "" && p.Cards[1] != "1B"}
		var winProb sql.NullFloat64
		if p.WinProbability != nil {
			winProb = sql.NullFloat64{Float64: *p.WinProbability, Valid: true}
		}
		_, err := tx.Exec(`
            INSERT INTO poker_player
            (game_id, user_id, name, seat_index, is_bot, chips, round_bet, total_bet,
             folded, all_in, is_dealer, is_small_blind, is_big_blind,
             card1, card2, win_probability, sitting_out, stats_data)
            VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18)`,
			gameID, p.ID, p.Name, p.SeatIndex, p.IsBot, p.Chips, p.RoundBet, p.TotalBet,
			p.Folded, p.AllIn, p.IsDealer, p.IsSmallBlind,
			p.IsBigBlind, card1, card2, winProb, p.SittingOut, string(statsJSON))
		if err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *PostgresStorage) loadPlayers(gameID int64) ([]*Player, error) {
	rows, err := s.db.Query(`SELECT user_id, name, seat_index, is_bot, chips, round_bet,
        total_bet, folded, all_in, is_dealer, is_small_blind, is_big_blind,
        card1, card2, win_probability, sitting_out, stats_data
        FROM poker_player WHERE game_id = $1 ORDER BY seat_index`, gameID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	players := []*Player{}
	for rows.Next() {
		p := &Player{}
		var card1, card2 sql.NullString
		var winProb sql.NullFloat64
		var statsJSON string
		if err := rows.Scan(&p.ID, &p.Name, &p.SeatIndex, &p.IsBot, &p.Chips, &p.RoundBet,
			&p.TotalBet, &p.Folded, &p.AllIn, &p.IsDealer, &p.IsSmallBlind, &p.IsBigBlind,
			&card1, &card2, &winProb, &p.SittingOut, &statsJSON); err != nil {
			return nil, err
		}
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

func nullString(s *string) interface{} {
	if s == nil {
		return nil
	}
	return *s
}

func (s *PostgresStorage) GetConfig(key string) (string, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var value string
	err := s.db.QueryRow(`SELECT value FROM poker_config WHERE key = $1`, key).Scan(&value)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return value, true, nil
}

func (s *PostgresStorage) SetConfig(key, value string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, err := s.db.Exec(`
        INSERT INTO poker_config (key, value) VALUES ($1, $2)
        ON CONFLICT (key) DO UPDATE SET value = excluded.value`,
		key, value)
	return err
}

type ConfigStore interface {
	GetConfig(key string) (string, bool, error)
	SetConfig(key, value string) error
}

// decodeNotifications reads the notifications column, which holds either the
// current array of objects or — for any game saved before notifications gained
// a kind and a sequence — a plain array of strings. Old rows are upgraded in
// place on read so no database migration is needed.
func decodeNotifications(raw string) []Notification {
	if raw == "" {
		return []Notification{}
	}

	var structured []Notification
	if err := json.Unmarshal([]byte(raw), &structured); err == nil {
		// A literal "null" unmarshals cleanly into a nil slice; callers should
		// never have to nil-check what this returns.
		if structured == nil {
			return []Notification{}
		}
		return structured
	}

	var legacy []string
	if err := json.Unmarshal([]byte(raw), &legacy); err != nil {
		return []Notification{}
	}

	out := make([]Notification, 0, len(legacy))
	for i, text := range legacy {
		out = append(out, Notification{Seq: i + 1, Kind: NoteSystem, Text: text})
	}
	return out
}
