package services

import (
	"context"
	"errors"
	"fmt"
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
	ErrBelowBuyIn          = errors.New("your wallet balance is below this table's buy-in")
	ErrTableNotFound       = errors.New("table not found")
	ErrRefillDisabled      = errors.New("refills are not available at this table")
	ErrRefillDuringHand    = errors.New("you can refill once this hand is over for you")
	ErrOutOfChips          = errors.New("you are out of chips — refill to play the next hand")
)

// RefillTooSmallError carries the table's big blind, which is the smallest
// refill worth making.
type RefillTooSmallError struct{ Min int64 }

func (e RefillTooSmallError) Error() string {
	return fmt.Sprintf("the smallest refill at this table is %d ETB", e.Min)
}

const (
	DefaultTurnSeconds = 25
	FastStartSeconds   = 2

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
	// Each seated player's chosen avatar, by name, so the table state does not
	// read the users table on every refresh. Filled on join, or on first use
	// for a table restored after a restart.
	Avatars map[string]int16
}

type GameService struct {
	mu       sync.RWMutex
	tables   map[string]*ActiveTable
	wallet   *WalletService
	sse      *SSEHub
	settings *SettingsService
	storage  poker.Storage
	rooms    *RoomService
	// Optional collaborators, set after construction (see SetHistory and
	// SetAvatarLookup); without them hands go unrecorded and avatars default.
	history  *HistoryService
	avatarOf func(ctx context.Context, userID int64) (int16, error)
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

	// Every table, private or public, seats a player with exactly its buy-in.
	// The rest of the wallet stays in the wallet, to be brought to the felt
	// with Refill when the player wants it.
	buyInAmount := table.BuyIn
	if buyInAmount <= 0 {
		buyInAmount = poker.StartingChips
	}
	belowBuyIn := fmt.Errorf("%w: %d ETB is needed", ErrBelowBuyIn, buyInAmount)
	if user.Wallet < buyInAmount {
		return belowBuyIn
	}
	if _, err := s.wallet.BuyIn(ctx, user.ID, buyInAmount); err != nil {
		if errors.Is(err, ErrInsufficientBalance) {
			return belowBuyIn
		}
		return err
	}
	chips := int(buyInAmount)

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
	if table.Avatars == nil {
		table.Avatars = map[string]int16{}
	}
	table.Avatars[user.Username] = user.Avatar
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
		if p.Chips <= 0 {
			return ErrOutOfChips
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

// refillAllowed reports whether seated players at this table may top up.
// Private tables always may; public tables follow the site setting.
func (s *GameService) refillAllowed(ctx context.Context, table *ActiveTable) bool {
	if table.RoomType == "private" || s.settings == nil {
		return true
	}
	settings, err := s.settings.Get(ctx)
	if err != nil {
		return false
	}
	return settings.PublicRefillEnabled
}

// inLiveHand is true while the player still has a stake in the hand being
// played. A stack must not change under a hand the player is in; once they
// have folded, been dealt out, or the hand is over, it is theirs to top up.
func inLiveHand(table *ActiveTable, p *poker.Player) bool {
	g := table.Engine.Game
	return g.GameStarted && !g.Intermission && !g.GameFinished && !p.Folded
}

// Refill moves amount from the player's wallet onto their stack and returns
// the new stack. There is no ceiling beyond the wallet itself.
//
// A refill also sits the player back in: whether they busted or had been sat
// out by the turn clock, asking for more chips is them saying they are here.
func (s *GameService) Refill(ctx context.Context, roomCode string, userID int64, amount int64) (int, error) {
	table, err := s.lookupTable(ctx, roomCode)
	if err != nil {
		return 0, err
	}

	table.mu.Lock()
	defer table.mu.Unlock()

	seatIdx, seated := table.UserMap[userID]
	if !seated {
		return 0, ErrPlayerNotFound
	}
	var player *poker.Player
	for _, p := range table.Engine.Players {
		if p.SeatIndex == seatIdx {
			player = p
			break
		}
	}
	if player == nil {
		return 0, ErrPlayerNotFound
	}

	if !s.refillAllowed(ctx, table) {
		return 0, ErrRefillDisabled
	}
	if minRefill := int64(table.Engine.Game.BigBlind); amount < minRefill {
		return 0, RefillTooSmallError{Min: minRefill}
	}
	if inLiveHand(table, player) {
		return 0, ErrRefillDuringHand
	}

	if _, err := s.wallet.TopUp(ctx, userID, amount); err != nil {
		return 0, err
	}

	player.Chips += int(amount)
	if player.SittingOut {
		player.SittingOut = false
		s.cancelSitOutEviction(table, player.Name)
	}

	s.persistTable(table)
	s.evaluateCountdown(ctx, table)
	s.broadcastTableUpdate(table)
	return player.Chips, nil
}

// sitOutBustedPlayers runs when a hand ends. A player left with no chips is
// sat out, which starts the same eviction clock as walking away does: the seat
// stays theirs to refill for a while, then it is freed and the next player
// can have it.
func (s *GameService) sitOutBustedPlayers(table *ActiveTable) {
	for _, p := range table.Engine.Players {
		if p.IsBot || p.Chips > 0 || p.SittingOut {
			continue
		}
		p.SittingOut = true
		s.startSitOutEviction(table, p.Name)
	}
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
