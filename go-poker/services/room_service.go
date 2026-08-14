package services

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/zuse/poker5/go-poker/repository"
	"github.com/zuse/poker5/go-poker/utilities"
)

var (
	ErrRoomNotFound     = errors.New("room not found")
	ErrInvalidBlindTier = errors.New("invalid blind tier")
	ErrInvalidBuyIn     = errors.New("buy-in is outside the allowed range for this tier")
	ErrInvalidBlinds    = errors.New("small blind must be at least 10")
	ErrInvalidSeats     = errors.New("seat count must be between 2 and 9")
)

// BlindTier is one of the fixed public-room blind levels.
type BlindTier struct {
	SmallBlind int32
	BigBlind   int32
}

var PublicBlindTiers = []BlindTier{
	{SmallBlind: 10, BigBlind: 20},
	{SmallBlind: 20, BigBlind: 40},
	{SmallBlind: 50, BigBlind: 100},
}

const (
	PublicMaxPlayers     = 6
	MinPrivateSmallBlind = 10
	MinPrivateSeats      = 2
	MaxPrivateSeats      = 9

	// Buy-in is bounded as a multiple of the big blind, standard cash-game convention.
	MinBuyInBBMultiple = 50
	MaxBuyInBBMultiple = 200
)

type RoomService struct {
	q *repository.Queries
}

func NewRoomService(q *repository.Queries) *RoomService {
	return &RoomService{q: q}
}

func buyInBounds(bigBlind int32) (min int64, max int64) {
	return int64(bigBlind) * MinBuyInBBMultiple, int64(bigBlind) * MaxBuyInBBMultiple
}

func findPublicTier(smallBlind int32) (BlindTier, bool) {
	for _, t := range PublicBlindTiers {
		if t.SmallBlind == smallBlind {
			return t, true
		}
	}
	return BlindTier{}, false
}

// CreatePrivateRoom creates a private table with no fixed buy-in: players
// bring their whole wallet balance to the felt when they join (see
// GameService.JoinTable), so long as it clears the big blind.
func (s *RoomService) CreatePrivateRoom(ctx context.Context, hostUserID int64, roomName string, smallBlind int32, maxPlayers int32) (*repository.PokerRoom, error) {
	if roomName == "" {
		roomName = "Private Poker Room"
	}
	if smallBlind <= 0 {
		smallBlind = PublicBlindTiers[0].SmallBlind
	}
	if smallBlind < MinPrivateSmallBlind {
		return nil, ErrInvalidBlinds
	}
	bigBlind := smallBlind * 2

	if maxPlayers <= 0 {
		maxPlayers = PublicMaxPlayers
	}
	if maxPlayers < MinPrivateSeats || maxPlayers > MaxPrivateSeats {
		return nil, ErrInvalidSeats
	}

	code, err := utilities.GenerateRoomCode()
	if err != nil {
		return nil, err
	}

	_, err = s.q.CreateRoom(ctx, repository.CreateRoomParams{
		RoomCode:   code,
		RoomName:   roomName,
		RoomType:   "private",
		HostUserID: sql.NullInt64{Int64: hostUserID, Valid: true},
		BuyIn:      0,
		SmallBlind: smallBlind,
		BigBlind:   bigBlind,
		MaxPlayers: maxPlayers,
		Status:     "active",
	})
	if err != nil {
		return nil, err
	}

	room, err := s.q.GetRoomByCode(ctx, code)
	if err != nil {
		return nil, err
	}
	return &room, nil
}

func (s *RoomService) CreatePublicRoom(ctx context.Context, smallBlind int32, buyIn int64, roomName string) (*repository.PokerRoom, error) {
	tier, ok := findPublicTier(smallBlind)
	if !ok {
		if smallBlind == 0 {
			tier = PublicBlindTiers[0]
		} else {
			return nil, ErrInvalidBlindTier
		}
	}
	if roomName == "" {
		roomName = fmt.Sprintf("Public %d/%d Table", tier.SmallBlind, tier.BigBlind)
	}

	min, max := buyInBounds(tier.BigBlind)
	if buyIn <= 0 {
		buyIn = min
	}
	if buyIn < min || buyIn > max {
		return nil, ErrInvalidBuyIn
	}

	code, err := utilities.GenerateRoomCode()
	if err != nil {
		return nil, err
	}

	_, err = s.q.CreateRoom(ctx, repository.CreateRoomParams{
		RoomCode:   code,
		RoomName:   roomName,
		RoomType:   "public",
		HostUserID: sql.NullInt64{},
		BuyIn:      buyIn,
		SmallBlind: tier.SmallBlind,
		BigBlind:   tier.BigBlind,
		MaxPlayers: PublicMaxPlayers,
		Status:     "active",
	})
	if err != nil {
		return nil, err
	}

	room, err := s.q.GetRoomByCode(ctx, code)
	if err != nil {
		return nil, err
	}
	return &room, nil
}

func (s *RoomService) GetRoomByCode(ctx context.Context, roomCode string) (*repository.PokerRoom, error) {
	room, err := s.q.GetRoomByCode(ctx, roomCode)
	if err != nil {
		return nil, ErrRoomNotFound
	}
	return &room, nil
}

func (s *RoomService) ListPublicRooms(ctx context.Context) ([]repository.PokerRoom, error) {
	return s.q.ListActivePublicRooms(ctx)
}

func (s *RoomService) QuickJoinPublicRoom(ctx context.Context, smallBlind int32) (*repository.PokerRoom, error) {
	rooms, err := s.q.ListActivePublicRooms(ctx)
	if err == nil {
		for _, r := range rooms {
			if smallBlind == 0 || r.SmallBlind == smallBlind {
				return &r, nil
			}
		}
	}
	tier := PublicBlindTiers[0]
	if t, ok := findPublicTier(smallBlind); ok {
		tier = t
	}
	min, _ := buyInBounds(tier.BigBlind)
	return s.CreatePublicRoom(ctx, tier.SmallBlind, min, "")
}
