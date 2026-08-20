package services

import (
	"context"
	"errors"
	"sync"
	"time"

	poker "github.com/zuse/poker5/go-poker"
	"github.com/zuse/poker5/go-poker/repository"
)

var (
	ErrTableFull           = errors.New("table is full")
	ErrAlreadySeated       = errors.New("already seated at table")
	ErrNotYourTurn         = errors.New("it is not your turn")
	ErrPlayerNotFound      = errors.New("player not seated at table")
	ErrInvalidAction       = errors.New("invalid poker action")
	ErrNotHost             = errors.New("only the room host can start early")
	ErrPublicNoManualStart = errors.New("public tables start automatically; manual start is not available")
	ErrHandInProgress      = errors.New("a hand is currently in progress; wait for it to finish before joining")
	ErrInsufficientFunds   = errors.New("your wallet balance must be greater than the big blind to join this table")
	ErrTableNotFound       = errors.New("table not found")
)

const (
	DefaultTurnSeconds  = 25
	FastStartSeconds    = 2
	IntermissionSeconds = 10

	// How long a sat-out player keeps their seat before their chips are
	// returned to their wallet and the seat is freed.
	SitOutEvictionAfter = 10 * time.Minute
)

type ActiveTable struct {
	mu                 sync.Mutex
	RoomCode           string
	RoomType           string
	HostUserID         int64
	BuyIn              int64
	MaxPlayers         int32
	Engine             *poker.GameEngine
	UserMap            map[int64]int
	UserIDs            map[string]int64
	TurnTimer          *time.Timer
	TimerExpiry        time.Time
	TurnSeconds        int
	CountdownActive    bool
	CountdownTimer     *time.Timer
	CountdownEndsAt    time.Time
	IntermissionTimer  *time.Timer
	IntermissionEndsAt time.Time
	// One eviction timer per sat-out player: sitting out stops the blinds, but
	// a seat held by someone who never comes back still costs the table.
	SitOutTimers map[string]*time.Timer
}

type GameService struct {
	mu       sync.RWMutex
	tables   map[string]*ActiveTable
	wallet   *WalletService
	sse      *SSEHub
	settings *SettingsService
	storage  poker.Storage
	rooms    *RoomService
}

func NewGameService(wallet *WalletService, sse *SSEHub, settings *SettingsService, storage poker.Storage, rooms *RoomService) *GameService {
	return &GameService{
		tables:   make(map[string]*ActiveTable),
		wallet:   wallet,
		sse:      sse,
		settings: settings,
		storage:  storage,
		rooms:    rooms,
	}
}

func (s *GameService) lookupTable(ctx context.Context, roomCode string) (*ActiveTable, error) {
	s.mu.RLock()
	table, exists := s.tables[roomCode]
	s.mu.RUnlock()
	if exists {
		return table, nil
	}

	if s.rooms == nil {
		return nil, ErrTableNotFound
	}
	room, err := s.rooms.GetRoomByCode(ctx, roomCode)
	if err != nil {
		return nil, ErrTableNotFound
	}
	return s.GetOrCreateTable(room), nil
}

func (s *GameService) GetOrCreateTable(room *repository.PokerRoom) *ActiveTable {
	s.mu.Lock()
	defer s.mu.Unlock()

	if table, exists := s.tables[room.RoomCode]; exists {
		return table
	}

	eng := poker.NewGameEngine(s.storage)
	g, players, isNew, err := s.storage.GetOrCreateGame(room.RoomCode)
	if err != nil {
		g = poker.NewGame(room.RoomCode)
		players = []*poker.Player{}
		isNew = true
	}

	eng.Game = g
	eng.Players = players
	if isNew {
		eng.Game.SmallBlind = int(room.SmallBlind)
		eng.Game.BigBlind = int(room.BigBlind)
		eng.Game.LastRaise = int(room.BigBlind)
	}

	userMap := make(map[int64]int)
	userIDs := make(map[string]int64)
	for i, p := range players {
		if p.ID > 0 {
			userMap[int64(p.ID)] = i
			userIDs[p.Name] = int64(p.ID)
		}
	}

	table := &ActiveTable{
		RoomCode:    room.RoomCode,
		RoomType:    room.RoomType,
		HostUserID:  room.HostUserID.Int64,
		BuyIn:       room.BuyIn,
		MaxPlayers:  room.MaxPlayers,
		Engine:      eng,
		UserMap:     userMap,
		UserIDs:     userIDs,
		TurnSeconds: DefaultTurnSeconds,
	}

	s.tables[room.RoomCode] = table

	if table.SitOutTimers == nil {
		table.SitOutTimers = map[string]*time.Timer{}
	}

	if len(players) >= 2 {
		switch {
		case eng.Game.Intermission:
			s.startIntermissionTimer(table)
		case eng.Game.GameStarted:
			s.resetTurnTimer(table)
		default:
			s.evaluateCountdown(context.Background(), table)
		}
	}

	return table
}

type TableLiveSummary struct {
	PlayerCount       int
	GameStarted       bool
	CountdownActive   bool
	CountdownEndsAtMs int64
}

func (s *GameService) GetLiveSummary(roomCode string) (TableLiveSummary, bool) {
	s.mu.RLock()
	table, exists := s.tables[roomCode]
	s.mu.RUnlock()
	if !exists {
		return TableLiveSummary{}, false
	}

	table.mu.Lock()
	defer table.mu.Unlock()

	return TableLiveSummary{
		PlayerCount:       len(table.Engine.Players),
		GameStarted:       table.Engine.Game.GameStarted,
		CountdownActive:   table.CountdownActive,
		CountdownEndsAtMs: unixMilliOrZero(table.CountdownEndsAt),
	}, true
}

func (s *GameService) GetSeatedRoomCode(userID int64) (string, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for code, table := range s.tables {
		table.mu.Lock()
		_, seated := table.UserMap[userID]
		table.mu.Unlock()
		if seated {
			return code, true
		}
	}
	return "", false
}

func (s *GameService) GetTable(roomCode string) (*ActiveTable, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	table, exists := s.tables[roomCode]
	if !exists {
		return nil, ErrTableNotFound
	}
	return table, nil
}

func (s *GameService) autoLeaveOtherTables(ctx context.Context, userID int64, currentRoomCode string) {
	s.mu.RLock()
	var otherCodes []string
	for code, tbl := range s.tables {
		if code != currentRoomCode {
			tbl.mu.Lock()
			if _, seated := tbl.UserMap[userID]; seated {
				otherCodes = append(otherCodes, code)
			}
			tbl.mu.Unlock()
		}
	}
	s.mu.RUnlock()

	for _, code := range otherCodes {
		_ = s.LeaveTable(ctx, code, userID)
	}
}

func (s *GameService) JoinTable(ctx context.Context, room *repository.PokerRoom, user *repository.User) error {
	s.autoLeaveOtherTables(ctx, user.ID, room.RoomCode)

	table := s.GetOrCreateTable(room)

	table.mu.Lock()
	defer table.mu.Unlock()

	if _, ok := table.UserMap[user.ID]; ok {
		return nil
	}

	if table.Engine.Game.GameStarted && !table.Engine.Game.Intermission {
		return ErrHandInProgress
	}

	if int32(len(table.Engine.Players)) >= table.MaxPlayers {
		return ErrTableFull
	}

	buyInAmount := table.BuyIn
	if table.RoomType == "private" {
		bigBlind := int64(table.Engine.Game.BigBlind)
		if user.Wallet <= bigBlind {
			return ErrInsufficientFunds
		}
		buyInAmount = user.Wallet
	}

	chips := int(buyInAmount)
	if chips <= 0 {
		chips = poker.StartingChips
	}

	_, err := s.wallet.BuyIn(ctx, user.ID, buyInAmount)
	if err != nil {
		return err
	}

	seatIdx := len(table.Engine.Players)
	player := &poker.Player{
		ID:        int(user.ID),
		Name:      user.Username,
		SeatIndex: seatIdx,
		IsBot:     false,
		Chips:     chips,
		Cards:     [2]string{"1B", "1B"},
		Stats:     poker.NewStats(),
		BotLine:   poker.NewBotLine(),
	}

	table.Engine.Players = append(table.Engine.Players, player)
	table.UserMap[user.ID] = seatIdx
	table.UserIDs[user.Username] = user.ID
	s.persistTable(table)

	s.evaluateCountdown(ctx, table)

	s.broadcastTableUpdate(table)
	return nil
}

func (s *GameService) persistTable(table *ActiveTable) {
	if table.Engine.Storage == nil {
		return
	}
	_ = table.Engine.Storage.SaveGame(table.Engine.Game, table.Engine.Players)
}

func (s *GameService) evaluateCountdown(ctx context.Context, table *ActiveTable) {
	if table.Engine.Game.GameStarted || table.Engine.Game.Intermission {
		return
	}
	count := table.Engine.DealtInCount()

	if count < 2 {
		s.cancelCountdownLocked(table)
		return
	}

	if int32(count) >= table.MaxPlayers {
		s.cancelCountdownLocked(table)
		s.startCountdownLocked(table, FastStartSeconds)
		return
	}

	if !table.CountdownActive {
		seconds := DefaultTurnSeconds
		if settings, err := s.settings.Get(ctx); err == nil {
			seconds = int(settings.CountdownSeconds)
		}
		s.startCountdownLocked(table, seconds)
	}
}

func (s *GameService) startCountdownLocked(table *ActiveTable, seconds int) {
	table.CountdownActive = true
	table.CountdownEndsAt = time.Now().Add(time.Duration(seconds) * time.Second)
	roomCode := table.RoomCode
	table.CountdownTimer = time.AfterFunc(time.Duration(seconds)*time.Second, func() {
		s.fireCountdown(roomCode)
	})
}

func (s *GameService) cancelCountdownLocked(table *ActiveTable) {
	if table.CountdownTimer != nil {
		table.CountdownTimer.Stop()
		table.CountdownTimer = nil
	}
	table.CountdownActive = false
}

func (s *GameService) fireCountdown(roomCode string) {
	s.mu.RLock()
	table, exists := s.tables[roomCode]
	s.mu.RUnlock()
	if !exists {
		return
	}

	table.mu.Lock()
	defer table.mu.Unlock()

	table.CountdownActive = false
	table.CountdownTimer = nil

	if len(table.Engine.Players) < 2 || table.Engine.Game.GameStarted {
		return
	}

	table.Engine.Game.Intermission = false
	table.Engine.StartHand()
	s.resetTurnTimer(table)
	s.broadcastTableUpdate(table)
}

// startSitOutEviction arms the timer that frees a seat whose player never came
// back. Cancelled by sitting back in, or by leaving.
func (s *GameService) startSitOutEviction(table *ActiveTable, playerName string) {
	if table.SitOutTimers == nil {
		table.SitOutTimers = map[string]*time.Timer{}
	}
	if existing := table.SitOutTimers[playerName]; existing != nil {
		existing.Stop()
	}

	roomCode := table.RoomCode
	userID, ok := table.UserIDs[playerName]
	if !ok {
		return
	}

	table.SitOutTimers[playerName] = time.AfterFunc(SitOutEvictionAfter, func() {
		// LeaveTable cashes their chips back to their wallet, which is the
		// whole reason this is not just a silent seat drop.
		_ = s.LeaveTable(context.Background(), roomCode, userID)
	})
}

func (s *GameService) cancelSitOutEviction(table *ActiveTable, playerName string) {
	if timer := table.SitOutTimers[playerName]; timer != nil {
		timer.Stop()
		delete(table.SitOutTimers, playerName)
	}
}

// SitIn returns a player to the game from the next hand onward. It is the
// consent step: nothing acts for a timed-out player again until they ask.
func (s *GameService) SitIn(ctx context.Context, roomCode string, userID int64) error {
	table, err := s.lookupTable(ctx, roomCode)
	if err != nil {
		return err
	}

	table.mu.Lock()
	defer table.mu.Unlock()

	seatIdx, seated := table.UserMap[userID]
	if !seated {
		return ErrPlayerNotFound
	}

	for _, p := range table.Engine.Players {
		if p.SeatIndex != seatIdx {
			continue
		}
		if !p.SittingOut {
			return nil
		}
		p.SittingOut = false
		s.cancelSitOutEviction(table, p.Name)
		s.persistTable(table)
		s.evaluateCountdown(ctx, table)
		s.broadcastTableUpdate(table)
		return nil
	}
	return ErrPlayerNotFound
}

func (s *GameService) LeaveTable(ctx context.Context, roomCode string, userID int64) error {
	table, err := s.lookupTable(ctx, roomCode)
	if err != nil {
		return err
	}

	table.mu.Lock()
	defer table.mu.Unlock()

	seatIdx, ok := table.UserMap[userID]
	if !ok {
		return nil
	}

	var remainingChips int
	for i, p := range table.Engine.Players {
		if p.SeatIndex == seatIdx {
			remainingChips = p.Chips
			s.cancelSitOutEviction(table, p.Name)
			delete(table.UserIDs, p.Name)
			table.Engine.Players = append(table.Engine.Players[:i], table.Engine.Players[i+1:]...)
			break
		}
	}

	delete(table.UserMap, userID)

	for i, p := range table.Engine.Players {
		p.SeatIndex = i
		if uid, found := table.UserIDs[p.Name]; found {
			table.UserMap[uid] = i
		}
	}

	if remainingChips > 0 {
		_, _ = s.wallet.CashOut(ctx, userID, int64(remainingChips))
	}

	if len(table.Engine.Players) < 2 {
		table.Engine.Game.GameStarted = false
		if table.TurnTimer != nil {
			table.TurnTimer.Stop()
		}
		if table.IntermissionTimer != nil {
			table.IntermissionTimer.Stop()
			table.IntermissionTimer = nil
		}
		s.cancelCountdownLocked(table)
	}

	s.persistTable(table)
	s.broadcastTableUpdate(table)
	return nil
}

func (s *GameService) StartHandManually(ctx context.Context, roomCode string, userID int64) error {
	table, err := s.lookupTable(ctx, roomCode)
	if err != nil {
		return err
	}

	table.mu.Lock()
	defer table.mu.Unlock()

	if table.RoomType != "private" {
		return ErrPublicNoManualStart
	}
	if table.HostUserID == 0 || table.HostUserID != userID {
		return ErrNotHost
	}
	if len(table.Engine.Players) < 2 {
		return errors.New("minimum 2 players required to start")
	}

	s.cancelCountdownLocked(table)
	if table.IntermissionTimer != nil {
		table.IntermissionTimer.Stop()
		table.IntermissionTimer = nil
	}
	table.Engine.Game.Intermission = false
	table.Engine.StartHand()
	s.resetTurnTimer(table)
	s.broadcastTableUpdate(table)
	return nil
}
