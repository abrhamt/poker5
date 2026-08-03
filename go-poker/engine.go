package poker

import (
	"math/rand"
	"strings"
	"time"
)

var timeNow = time.Now

const MaxSeats = 6

const IntermissionDuration = 5 * time.Second

var (
	Ranks    = []byte{'2', '3', '4', '5', '6', '7', '8', '9', 'T', 'J', 'Q', 'K', 'A'}
	Suits    = []byte{'C', 'D', 'H', 'S'}
	FullDeck = buildFullDeck()
	AlphaNum = []byte("abcdefghijklmnopqrstuvwxyz0123456789")
)

func buildFullDeck() []string {
	deck := make([]string, 0, 52)
	for _, s := range Suits {
		for _, r := range Ranks {
			deck = append(deck, string([]byte{r, s}))
		}
	}
	return deck
}

func ShuffleDeck(deck []string) []string {
	out := append([]string{}, deck...)
	rand.Shuffle(len(out), func(i, j int) { out[i], out[j] = out[j], out[i] })
	return out
}

func GenerateTableID() string {
	b := make([]byte, 6)
	for i := range b {
		b[i] = AlphaNum[rand.Intn(len(AlphaNum))]
	}
	return string(b)
}

func CombinationCount(n, k int) int {
	if k < 0 || k > n {
		return 0
	}
	kk := k
	if n-k < kk {
		kk = n - k
	}
	result := 1
	for i := 1; i <= kk; i++ {
		result = result * (n - kk + i) / i
	}
	return result
}

type Storage interface {
	SaveGame(g *Game, players []*Player) error
	LoadGame(tableID string) (*Game, []*Player, error)
	GetOrCreateGame(tableID string) (*Game, []*Player, bool, error)
}

type GameEngine struct {
	Game            *Game
	Players         []*Player
	Storage         Storage
	OnNotification  func(string)
	OnStateChanged  func()
	CurrentPlayerIx int
}

func NewGameEngine(storage Storage) *GameEngine {
	return &GameEngine{Storage: storage}
}

func (e *GameEngine) LoadFromStorage(tableID string) error {
	g, players, _, err := e.Storage.GetOrCreateGame(tableID)
	if err != nil {
		return err
	}
	e.Game = g
	e.Players = players
	return nil
}

func (e *GameEngine) LoadFromGame(g *Game, players []*Player) {
	e.Game = g
	e.Players = players
}

func (e *GameEngine) addNotification(msg string) {
	e.Game.Notifications = append(e.Game.Notifications, msg)
	if len(e.Game.Notifications) > MaxNotifications {
		e.Game.Notifications = e.Game.Notifications[len(e.Game.Notifications)-MaxNotifications:]
	}
	if e.OnNotification != nil {
		e.OnNotification(msg)
	}
}

func (e *GameEngine) InitGame(playerNames []string) {
	if e.Game == nil {
		e.Game = NewGame("")
	}
	e.Game.Deck = ShuffleDeck(FullDeck)
	e.Game.GameStarted = false
	e.Game.GameFinished = false
	e.Game.TotalHands = 0
	e.Game.Notifications = []string{}
	e.Game.PhaseIndex = 0
	e.Game.Phase = Phases[0]
	e.Game.Pot = 0
	e.Game.CurrentBet = 0
	e.Game.LastRaise = DefaultLastRaise
	e.Game.SmallBlind = DefaultSmallBlind
	e.Game.BigBlind = DefaultBigBlind
	e.Game.DealerOrbitCount = -1
	e.Game.InitialDealerName = nil
	e.Game.OpenCardsMode = false
	e.Game.SpectatorMode = false
	e.Game.CommunityCards = []string{}
	e.Game.CurrentPlayerIdx = 0
	e.Game.RaisesThisRound = 0
	e.Players = []*Player{}
	for idx, name := range playerNames {
		isBot := strings.HasPrefix(strings.ToLower(name), "bot")
		e.Players = append(e.Players, &Player{
			Name:      name,
			SeatIndex: idx,
			IsBot:     isBot,
			Chips:     StartingChips,
			RoundBet:  0,
			TotalBet:  0,
			Folded:    false,
			AllIn:     false,
			Cards:     [2]string{"1B", "1B"},
			Stats:     NewStats(),
			BotLine:   NewBotLine(),
		})
	}
	e.Game.GameStarted = true
}