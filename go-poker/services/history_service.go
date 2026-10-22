package services

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"
	"time"

	"github.com/zuse/poker5/go-poker/repository"
)

// HandRecord is a finished hand, snapshotted under the table lock before the
// next deal clears it, then written to the database off the lock.
type HandRecord struct {
	RoomCode     string
	HandNumber   int
	SmallBlind   int
	BigBlind     int
	Board        []string
	WinnerName   string
	WinnerUserID int64
	WinningHand  string
	Pot          int64
	Rake         int64
	Players      []HandRecordPlayer
}

// HandRecordPlayer is one seat that was dealt into the hand.
type HandRecordPlayer struct {
	// UserID is 0 for a bot, which has no account and so no history of its own.
	UserID int64
	Name   string
	Seat   int
	Cards  [2]string
	// Invested is what went into the pot; Won is what came back, after rake.
	Invested   int64
	Won        int64
	Folded     bool
	ShowedDown bool
	HandName   string
}

// handDetails is game_sessions.details_json: everything about the hand that
// is not one player's own row. Hole cards are stored only for players who
// showed them, so no history view can leak a hand that was mucked.
type handDetails struct {
	Board      []string            `json:"board"`
	SmallBlind int                 `json:"small_blind"`
	BigBlind   int                 `json:"big_blind"`
	Players    []handDetailsPlayer `json:"players"`
}

type handDetailsPlayer struct {
	Name       string   `json:"name"`
	Seat       int      `json:"seat"`
	IsBot      bool     `json:"is_bot"`
	Cards      []string `json:"cards,omitempty"`
	Invested   int64    `json:"invested"`
	Won        int64    `json:"won"`
	Net        int64    `json:"net"`
	Folded     bool     `json:"folded"`
	ShowedDown bool     `json:"showed_down"`
	HandName   string   `json:"hand_name,omitempty"`
}

// HistoryService records every finished hand and serves a player their own
// hands and statistics. Only hands a player was dealt into are ever theirs.
type HistoryService struct {
	q  *repository.Queries
	db *sql.DB
}

func NewHistoryService(q *repository.Queries, db *sql.DB) *HistoryService {
	return &HistoryService{q: q, db: db}
}

// RecordHand writes the hand and one row per real player, in one transaction
// so a hand is never half-recorded.
func (s *HistoryService) RecordHand(ctx context.Context, rec HandRecord) error {
	details := handDetails{Board: rec.Board, SmallBlind: rec.SmallBlind, BigBlind: rec.BigBlind}
	if details.Board == nil {
		details.Board = []string{}
	}
	for _, p := range rec.Players {
		d := handDetailsPlayer{
			Name:       p.Name,
			Seat:       p.Seat,
			IsBot:      p.UserID == 0,
			Invested:   p.Invested,
			Won:        p.Won,
			Net:        p.Won - p.Invested,
			Folded:     p.Folded,
			ShowedDown: p.ShowedDown,
		}
		if p.ShowedDown {
			d.Cards = []string{p.Cards[0], p.Cards[1]}
			d.HandName = p.HandName
		}
		details.Players = append(details.Players, d)
	}
	detailsJSON, err := json.Marshal(details)
	if err != nil {
		return err
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	q := s.q.WithTx(tx)

	sessionID, err := q.CreateGameSession(ctx, repository.CreateGameSessionParams{
		TableID:          rec.RoomCode,
		HandNumber:       int32(rec.HandNumber),
		PotAmount:        rec.Pot,
		CommissionAmount: rec.Rake,
		WinnerUserID:     sql.NullInt64{Int64: rec.WinnerUserID, Valid: rec.WinnerUserID != 0},
		WinnerName:       rec.WinnerName,
		HandName:         clipTo(rec.WinningHand, 64),
		DetailsJson:      string(detailsJSON),
	})
	if err != nil {
		return err
	}

	for _, p := range rec.Players {
		if p.UserID == 0 {
			continue
		}
		if err := q.CreateHandPlayer(ctx, repository.CreateHandPlayerParams{
			SessionID:  sessionID,
			UserID:     p.UserID,
			RoomCode:   rec.RoomCode,
			PlayerName: p.Name,
			SeatIndex:  int32(p.Seat),
			Card1:      p.Cards[0],
			Card2:      p.Cards[1],
			Invested:   p.Invested,
			Won:        p.Won,
			Net:        p.Won - p.Invested,
			Folded:     p.Folded,
			ShowedDown: p.ShowedDown,
			HandName:   clipTo(p.HandName, 64),
		}); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// clipTo fits a value to a VARCHAR column, cutting rather than failing the insert.
func clipTo(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max]
}

// HistoryRoom is one room in a player's history, with their totals there.
type HistoryRoom struct {
	RoomCode    string    `json:"room_code"`
	RoomName    string    `json:"room_name"`
	RoomType    string    `json:"room_type"`
	SmallBlind  int32     `json:"small_blind"`
	BigBlind    int32     `json:"big_blind"`
	Hands       int64     `json:"hands"`
	HandsWon    int64     `json:"hands_won"`
	Net         int64     `json:"net"`
	FirstPlayed time.Time `json:"first_played"`
	LastPlayed  time.Time `json:"last_played"`
}

func (s *HistoryService) Rooms(ctx context.Context, userID int64) ([]HistoryRoom, error) {
	rows, err := s.q.ListUserHistoryRooms(ctx, userID)
	if err != nil {
		return nil, err
	}
	rooms := make([]HistoryRoom, 0, len(rows))
	for _, r := range rows {
		rooms = append(rooms, HistoryRoom{
			RoomCode:    r.RoomCode,
			RoomName:    r.RoomName,
			RoomType:    r.RoomType,
			SmallBlind:  r.SmallBlind,
			BigBlind:    r.BigBlind,
			Hands:       r.Hands,
			HandsWon:    r.HandsWon,
			Net:         r.Net,
			FirstPlayed: r.FirstPlayed,
			LastPlayed:  r.LastPlayed,
		})
	}
	return rooms, nil
}

// HistoryHand is one hand as its player sees it afterwards: their own cards
// and result, the board, and whatever the other players showed.
type HistoryHand struct {
	ID          int64               `json:"id"`
	HandNumber  int32               `json:"hand_number"`
	PlayedAt    time.Time           `json:"played_at"`
	Pot         int64               `json:"pot"`
	Rake        int64               `json:"rake"`
	Board       []string            `json:"board"`
	SmallBlind  int                 `json:"small_blind"`
	BigBlind    int                 `json:"big_blind"`
	WinnerName  string              `json:"winner_name"`
	WinningHand string              `json:"winning_hand"`
	You         HistoryHandSeat     `json:"you"`
	Players     []handDetailsPlayer `json:"players"`
}

type HistoryHandSeat struct {
	Name       string   `json:"name"`
	Cards      []string `json:"cards"`
	Invested   int64    `json:"invested"`
	Won        int64    `json:"won"`
	Net        int64    `json:"net"`
	Folded     bool     `json:"folded"`
	ShowedDown bool     `json:"showed_down"`
	HandName   string   `json:"hand_name"`
}

type HistoryHandPage struct {
	Hands      []HistoryHand `json:"hands"`
	Page       int           `json:"page"`
	PageSize   int           `json:"page_size"`
	Total      int64         `json:"total"`
	TotalPages int           `json:"total_pages"`
}

func (s *HistoryService) RoomHands(ctx context.Context, userID int64, roomCode string, page, pageSize int) (HistoryHandPage, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 50 {
		pageSize = 20
	}

	total, err := s.q.CountUserHandsInRoom(ctx, repository.CountUserHandsInRoomParams{UserID: userID, RoomCode: roomCode})
	if err != nil {
		return HistoryHandPage{}, err
	}
	rows, err := s.q.ListUserHandsInRoom(ctx, repository.ListUserHandsInRoomParams{
		UserID:   userID,
		RoomCode: roomCode,
		Limit:    int32(pageSize),
		Offset:   int32((page - 1) * pageSize),
	})
	if err != nil {
		return HistoryHandPage{}, err
	}

	hands := make([]HistoryHand, 0, len(rows))
	for _, r := range rows {
		var details handDetails
		_ = json.Unmarshal([]byte(r.DetailsJson), &details)
		board := details.Board
		if board == nil {
			board = []string{}
		}
		others := make([]handDetailsPlayer, 0, len(details.Players))
		for _, p := range details.Players {
			if p.Name != r.PlayerName {
				others = append(others, p)
			}
		}
		hands = append(hands, HistoryHand{
			ID:          r.ID,
			HandNumber:  r.HandNumber,
			PlayedAt:    r.CreatedAt,
			Pot:         r.PotAmount,
			Rake:        r.CommissionAmount,
			Board:       board,
			SmallBlind:  details.SmallBlind,
			BigBlind:    details.BigBlind,
			WinnerName:  r.WinnerName,
			WinningHand: r.WinningHand,
			You: HistoryHandSeat{
				Name:       r.PlayerName,
				Cards:      []string{r.Card1, r.Card2},
				Invested:   r.Invested,
				Won:        r.Won,
				Net:        r.Net,
				Folded:     r.Folded,
				ShowedDown: r.ShowedDown,
				HandName:   r.HandName,
			},
			Players: others,
		})
	}

	totalPages := int((total + int64(pageSize) - 1) / int64(pageSize))
	return HistoryHandPage{Hands: hands, Page: page, PageSize: pageSize, Total: total, TotalPages: totalPages}, nil
}

// handCategories orders hand names from strongest to weakest; longer names
// first so "Straight Flush" is never read as "Straight".
var handCategories = []string{
	"Royal Flush", "Straight Flush", "Four of a Kind", "Full House", "Flush",
	"Straight", "Three of a Kind", "Two Pair", "Pair", "High Card",
}

func handCategory(name string) string {
	for _, c := range handCategories {
		if strings.HasPrefix(name, c) {
			return c
		}
	}
	return ""
}

type PlayerStats struct {
	Hands        int64  `json:"hands"`
	HandsWon     int64  `json:"hands_won"`
	HandsFolded  int64  `json:"hands_folded"`
	Showdowns    int64  `json:"showdowns"`
	ShowdownsWon int64  `json:"showdowns_won"`
	Net          int64  `json:"net"`
	Invested     int64  `json:"invested"`
	Won          int64  `json:"won"`
	BiggestWin   int64  `json:"biggest_win"`
	BiggestLoss  int64  `json:"biggest_loss"`
	Rooms        int64  `json:"rooms"`
	BestHand     string `json:"best_hand"`
	// Winning showdown hands by category, strongest first, zero counts omitted.
	WinningHands []HandCategoryCount `json:"winning_hands"`
	// The running total of net chips over the most recent hands, oldest first.
	Trend []TrendPoint `json:"trend"`
}

type HandCategoryCount struct {
	Category string `json:"category"`
	Times    int64  `json:"times"`
}

type TrendPoint struct {
	At         time.Time `json:"at"`
	Net        int64     `json:"net"`
	Cumulative int64     `json:"cumulative"`
}

// trendLength is how many recent hands the profit chart covers.
const trendLength = 200

func (s *HistoryService) Stats(ctx context.Context, userID int64) (PlayerStats, error) {
	t, err := s.q.GetUserHandTotals(ctx, userID)
	if err != nil {
		return PlayerStats{}, err
	}
	stats := PlayerStats{
		Hands:        t.Hands,
		HandsWon:     t.HandsWon,
		HandsFolded:  t.HandsFolded,
		Showdowns:    t.Showdowns,
		ShowdownsWon: t.ShowdownsWon,
		Net:          t.Net,
		Invested:     t.Invested,
		Won:          t.Won,
		BiggestWin:   t.BiggestWin,
		BiggestLoss:  t.BiggestLoss,
		Rooms:        t.Rooms,
		WinningHands: []HandCategoryCount{},
		Trend:        []TrendPoint{},
	}

	names, err := s.q.ListUserWinningHandNames(ctx, userID)
	if err != nil {
		return PlayerStats{}, err
	}
	byCategory := map[string]int64{}
	for _, n := range names {
		if c := handCategory(n.HandName); c != "" {
			byCategory[c] += n.Times
		}
	}
	for _, c := range handCategories {
		if byCategory[c] > 0 {
			stats.WinningHands = append(stats.WinningHands, HandCategoryCount{Category: c, Times: byCategory[c]})
			if stats.BestHand == "" {
				stats.BestHand = c
			}
		}
	}

	recent, err := s.q.ListUserRecentNets(ctx, repository.ListUserRecentNetsParams{UserID: userID, Limit: trendLength})
	if err != nil {
		return PlayerStats{}, err
	}
	// The window starts where the hands before it left off, so the line ends
	// at the player's true all-time net rather than at the window's own sum.
	var windowSum int64
	for _, r := range recent {
		windowSum += r.Net
	}
	running := t.Net - windowSum
	for i := len(recent) - 1; i >= 0; i-- {
		running += recent[i].Net
		stats.Trend = append(stats.Trend, TrendPoint{At: recent[i].CreatedAt, Net: recent[i].Net, Cumulative: running})
	}
	return stats, nil
}
