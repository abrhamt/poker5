package services

import (
	"context"
	"log"
	"time"

	poker "github.com/zuse/poker5/go-poker"
)

// SetHistory turns on hand recording: every finished hand is written to the
// players' history. Left unset (as in most tests), hands are not recorded.
func (s *GameService) SetHistory(h *HistoryService) {
	s.history = h
}

// SetAvatarLookup supplies the users table's avatar for a player the table
// learned about without a join, e.g. a table restored after a restart.
func (s *GameService) SetAvatarLookup(fn func(ctx context.Context, userID int64) (int16, error)) {
	s.avatarOf = fn
}

// recordHand snapshots the finished hand under the table lock and writes it
// off the lock, so a slow database never holds up the next deal. gross is
// what each winner took from the pot; rake is what was then taken back.
func (s *GameService) recordHand(table *ActiveTable, gross, rake map[string]int64) {
	if s.history == nil {
		return
	}
	g := table.Engine.Game

	// Dealt in = holding real cards; a sat-out seat holds the "1B" backs.
	var dealt []*poker.Player
	contenders := 0
	for _, p := range table.Engine.Players {
		if p.Cards[0] == "" || p.Cards[0] == "1B" {
			continue
		}
		dealt = append(dealt, p)
		if !p.Folded {
			contenders++
		}
	}
	if len(dealt) == 0 {
		return
	}
	showdown := contenders > 1

	rec := HandRecord{
		RoomCode:   table.RoomCode,
		HandNumber: g.TotalHands,
		SmallBlind: g.SmallBlind,
		BigBlind:   g.BigBlind,
		Board:      append([]string{}, g.CommunityCards...),
	}
	if w := g.LastWinner; w != nil {
		rec.WinnerName = w.Name
		rec.WinningHand = w.HandName
		rec.WinnerUserID = table.UserIDs[w.Name]
	}
	for _, p := range dealt {
		showed := showdown && !p.Folded
		hand := ""
		if showed {
			hand = table.Engine.SolveHandFor(p)
		}
		rec.Players = append(rec.Players, HandRecordPlayer{
			UserID:     table.UserIDs[p.Name],
			Name:       p.Name,
			Seat:       p.SeatIndex,
			Cards:      p.Cards,
			Invested:   int64(p.TotalBet),
			Won:        gross[p.Name] - rake[p.Name],
			Folded:     p.Folded,
			ShowedDown: showed,
			HandName:   hand,
		})
		rec.Pot += gross[p.Name]
		rec.Rake += rake[p.Name]
	}

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := s.history.RecordHand(ctx, rec); err != nil {
			log.Printf("history: could not record hand %d at %s: %v", rec.HandNumber, rec.RoomCode, err)
		}
	}()
}

// avatarFor is the player's chosen avatar, or -1 (the client picks one from
// the name) for bots and anyone whose choice cannot be read. Caller holds the
// table lock.
func (s *GameService) avatarFor(ctx context.Context, table *ActiveTable, p *poker.Player) int16 {
	if a, ok := table.Avatars[p.Name]; ok {
		return a
	}
	uid, ok := table.UserIDs[p.Name]
	if !ok || s.avatarOf == nil {
		return -1
	}
	a, err := s.avatarOf(ctx, uid)
	if err != nil {
		return -1
	}
	if table.Avatars == nil {
		table.Avatars = map[string]int16{}
	}
	table.Avatars[p.Name] = a
	return a
}

// IsSeated reports whether the user holds a seat at any live table. A seated
// player is known to the engine by name, so their name must not change.
func (s *GameService) IsSeated(userID int64) bool {
	s.mu.RLock()
	tables := make([]*ActiveTable, 0, len(s.tables))
	for _, t := range s.tables {
		tables = append(tables, t)
	}
	s.mu.RUnlock()

	for _, t := range tables {
		t.mu.Lock()
		_, seated := t.UserMap[userID]
		t.mu.Unlock()
		if seated {
			return true
		}
	}
	return false
}

// SetPlayerAvatar shows a newly chosen avatar at any table the user sits at.
func (s *GameService) SetPlayerAvatar(userID int64, avatar int16) {
	s.mu.RLock()
	tables := make([]*ActiveTable, 0, len(s.tables))
	for _, t := range s.tables {
		tables = append(tables, t)
	}
	s.mu.RUnlock()

	for _, t := range tables {
		t.mu.Lock()
		for name, uid := range t.UserIDs {
			if uid == userID {
				if t.Avatars == nil {
					t.Avatars = map[string]int16{}
				}
				t.Avatars[name] = avatar
				s.broadcastTableUpdate(t)
			}
		}
		t.mu.Unlock()
	}
}
