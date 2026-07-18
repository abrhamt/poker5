package poker

import (
	"errors"
	"fmt"
	"math/rand"
	"sort"
	"strings"
	"sync"
	"time"
)

// timeNow is overridable in tests.
var timeNow = time.Now

// MaxSeats is the maximum number of players at the table. Mirrors the
// six fixed seats rendered by templates/table.html.
const MaxSeats = 6

// IntermissionDuration is how long the showdown modal stays open before
// the next hand starts. The UI counts down from this value.
const IntermissionDuration = 5 * time.Second

// Ranks / Suits / FullDeck mirror the lists at the top of engine.py.
var (
	Ranks     = []byte{'2', '3', '4', '5', '6', '7', '8', '9', 'T', 'J', 'Q', 'K', 'A'}
	Suits     = []byte{'C', 'D', 'H', 'S'}
	FullDeck  = buildFullDeck()
	AlphaNum  = []byte("abcdefghijklmnopqrstuvwxyz0123456789")
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

// ShuffleDeck is the Go port of shuffle_deck. It returns a fresh slice so
// callers can mutate the result without affecting the input.
func ShuffleDeck(deck []string) []string {
	out := append([]string{}, deck...)
	rand.Shuffle(len(out), func(i, j int) { out[i], out[j] = out[j], out[i] })
	return out
}

// GenerateTableID mirrors engine.generate_table_id.
func GenerateTableID() string {
	b := make([]byte, 6)
	for i := range b {
		b[i] = AlphaNum[rand.Intn(len(AlphaNum))]
	}
	return string(b)
}

// CombinationCount mirrors engine.combination_count.
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

// Storage is the persistence interface that engine.save_state / engine.load
// use. The SQLite implementation lives in storage.go; an in-memory
// implementation is used by the tests.
type Storage interface {
	SaveGame(g *Game, players []*Player) error
	LoadGame(tableID string) (*Game, []*Player, error)
	GetOrCreateGame(tableID string) (*Game, []*Player, bool, error)
}

// GameEngine is the Go port of engine.GameEngine.
type GameEngine struct {
	Game            *Game
	Players         []*Player
	Storage         Storage
	OnNotification  func(string)
	OnStateChanged  func()
	CurrentPlayerIx int
}

// NewGameEngine constructs an in-memory engine bound to the supplied storage.
func NewGameEngine(storage Storage) *GameEngine {
	return &GameEngine{Storage: storage}
}

// LoadFromStorage pulls the game + players from the storage layer. It mirrors
// GameEngine.load_from_model.
func (e *GameEngine) LoadFromStorage(tableID string) error {
	g, players, _, err := e.Storage.GetOrCreateGame(tableID)
	if err != nil {
		return err
	}
	e.Game = g
	e.Players = players
	return nil
}

// LoadFromGame refreshes in-memory state from a Game + players pair, e.g.
// after a polling fetch from the database.
func (e *GameEngine) LoadFromGame(g *Game, players []*Player) {
	e.Game = g
	e.Players = players
}

// addNotification appends a message and trims to MaxNotifications.
func (e *GameEngine) addNotification(msg string) {
	e.Game.Notifications = append(e.Game.Notifications, msg)
	if len(e.Game.Notifications) > MaxNotifications {
		e.Game.Notifications = e.Game.Notifications[len(e.Game.Notifications)-MaxNotifications:]
	}
	if e.OnNotification != nil {
		e.OnNotification(msg)
	}
}

// InitGame resets the engine to a fresh game with the given player names.
//
// Mirrors GameEngine.init_game in engine.py. Unlike the Python
// implementation the engine mutates its existing Game struct in place so
// that the storage layer's primary key (and any deferred SaveGame call)
// remains valid.
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

// StartHand starts a new hand: reset per-player state, set dealer, post
// blinds, deal cards. Mirrors GameEngine.start_hand.
func (e *GameEngine) StartHand() bool {
	e.Game.GameStarted = true
	e.Game.TotalHands++
	e.Game.PhaseIndex = 0
	e.Game.Pot = 0
	e.Game.CurrentBet = 0
	e.Game.LastRaise = e.Game.BigBlind
	e.Game.RaisesThisRound = 0
	e.Game.CommunityCards = []string{}

	for _, p := range e.Players {
		p.Folded = false
		p.AllIn = false
		p.TotalBet = 0
		p.RoundBet = 0
		p.WinProbability = nil
		p.Cards = [2]string{"1B", "1B"}
		p.BotLine = NewBotLine()
	}

	active := []*Player{}
	for _, p := range e.Players {
		if p.Chips > 0 {
			active = append(active, p)
		}
	}
	for _, p := range e.Players {
		if p.Chips <= 0 {
			e.addNotification(fmt.Sprintf("%s is out of the game!", p.Name))
		}
	}
	e.Players = active

	if len(e.Players) == 0 {
		return false
	}

	humanCount := 0
	for _, p := range e.Players {
		if !p.IsBot {
			humanCount++
		}
	}
	e.Game.OpenCardsMode = humanCount == 1
	e.Game.SpectatorMode = humanCount == 0

	if e.Game.InitialDealerName != nil {
		found := false
		for _, p := range e.Players {
			if p.Name == *e.Game.InitialDealerName {
				found = true
				break
			}
		}
		if !found {
			name := e.Players[0].Name
			e.Game.InitialDealerName = &name
			e.Game.DealerOrbitCount = -1
		}
	} else if len(e.Players) > 0 {
		name := e.Players[0].Name
		e.Game.InitialDealerName = &name
	}

	for _, p := range e.Players {
		p.Stats.Hands++
	}

	if len(e.Players) == 1 {
		champion := e.Players[0]
		e.addNotification(fmt.Sprintf("%s wins the game!", champion.Name))
		e.Game.GameFinished = true
		return false
	}

	e.setDealer()
	e.setBlinds()
	e.dealCards()
	e.saveState()
	e.startBettingRound()
	return true
}

// setDealer rotates the dealer button. Mirrors GameEngine._set_dealer.
func (e *GameEngine) setDealer() {
	dealerIdx := -1
	for i, p := range e.Players {
		if p.IsDealer {
			dealerIdx = i
			break
		}
	}
	if dealerIdx < 0 {
		idx := rand.Intn(len(e.Players))
		e.Players[idx].IsDealer = true
		name := e.Players[idx].Name
		e.Game.InitialDealerName = &name
	} else {
		e.Players[dealerIdx].IsDealer = false
		next := (dealerIdx + 1) % len(e.Players)
		e.Players[next].IsDealer = true
	}

	// Rotate the slice so the dealer is first.
	for !e.Players[0].IsDealer {
		first := e.Players[0]
		e.Players = append(e.Players[1:], first)
	}
	e.addNotification(fmt.Sprintf("%s is Dealer.", e.Players[0].Name))
}

// setBlinds posts the small / big blinds and doubles them every other orbit.
// Mirrors GameEngine._set_blinds.
func (e *GameEngine) setBlinds() {
	if e.Players[0].Name == derefString(e.Game.InitialDealerName) {
		e.Game.DealerOrbitCount++
		if e.Game.DealerOrbitCount > 0 && e.Game.DealerOrbitCount%2 == 0 {
			e.Game.SmallBlind *= 2
			e.Game.BigBlind *= 2
			e.addNotification(fmt.Sprintf("Blinds are now %d/%d.", e.Game.SmallBlind, e.Game.BigBlind))
		}
	}
	for _, p := range e.Players {
		p.IsSmallBlind = false
		p.IsBigBlind = false
	}

	sbIdx := 1
	bbIdx := 2
	if len(e.Players) <= 2 {
		sbIdx = 0
		bbIdx = 1
	}

	sbBet := e.placeBet(e.Players[sbIdx], e.Game.SmallBlind)
	bbBet := e.placeBet(e.Players[bbIdx], e.Game.BigBlind)
	e.addNotification(fmt.Sprintf("%s posted small blind of %d.", e.Players[sbIdx].Name, sbBet))
	e.addNotification(fmt.Sprintf("%s posted big blind of %d.", e.Players[bbIdx].Name, bbBet))
	e.Game.Pot += sbBet + bbBet
	e.Players[sbIdx].IsSmallBlind = true
	e.Players[bbIdx].IsBigBlind = true
	e.Game.CurrentBet = e.Game.BigBlind
	e.Game.LastRaise = e.Game.BigBlind
}

// placeBet deducts chips from the player up to their stack and marks them
// all-in if they hit zero. Mirrors GameEngine._place_bet.
func (e *GameEngine) placeBet(p *Player, amount int) int {
	bet := amount
	if bet > p.Chips {
		bet = p.Chips
	}
	p.RoundBet += bet
	p.TotalBet += bet
	p.Chips -= bet
	if p.Chips == 0 {
		p.AllIn = true
	}
	return bet
}

// dealCards deals two hole cards to each player from a freshly shuffled
// deck. Mirrors GameEngine._deal_cards.
func (e *GameEngine) dealCards() {
	e.Game.Deck = ShuffleDeck(FullDeck)
	e.Game.CardGraveyard = []string{}
	for _, p := range e.Players {
		if len(e.Game.Deck) >= 2 {
			p.Cards[0] = e.Game.Deck[0]
			p.Cards[1] = e.Game.Deck[1]
			e.Game.Deck = e.Game.Deck[2:]
		}
		e.Game.CardGraveyard = append(e.Game.CardGraveyard, p.Cards[0], p.Cards[1])
	}
}

// dealCommunityCards burns one card then deals `count` community cards.
// Mirrors GameEngine._deal_community_cards.
func (e *GameEngine) dealCommunityCards(count int) {
	if len(e.Game.Deck) <= count {
		return
	}
	e.Game.Deck = e.Game.Deck[1:]
	for i := 0; i < count; i++ {
		if len(e.Game.Deck) == 0 {
			break
		}
		card := e.Game.Deck[0]
		e.Game.Deck = e.Game.Deck[1:]
		e.Game.CommunityCards = append(e.Game.CommunityCards, card)
		e.Game.CardGraveyard = append(e.Game.CardGraveyard, card)
	}
}

// startBettingRound sets the next player to act and resets per-round state.
// Mirrors GameEngine._start_betting_round.
func (e *GameEngine) startBettingRound() {
	if e.Game.PhaseIndex > 0 {
		e.Game.CurrentBet = 0
		e.Game.LastRaise = e.Game.BigBlind
		for _, p := range e.Players {
			p.RoundBet = 0
		}
	}

	active := []*Player{}
	for _, p := range e.Players {
		if !p.Folded {
			active = append(active, p)
		}
	}
	actionable := []*Player{}
	for _, p := range active {
		if !p.AllIn {
			actionable = append(actionable, p)
		}
	}
	if len(active) <= 1 || len(actionable) <= 1 {
		e.advancePhase()
		return
	}

	if e.Game.PhaseIndex == 0 {
		bbIdx := 0
		for i, p := range e.Players {
			if p.IsBigBlind {
				bbIdx = i
				break
			}
		}
		e.CurrentPlayerIx = (bbIdx + 1) % len(e.Players)
	} else {
		dealerIdx := 0
		for i, p := range e.Players {
			if p.IsDealer {
				dealerIdx = i
				break
			}
		}
		e.CurrentPlayerIx = (dealerIdx + 1) % len(e.Players)
	}
	e.Game.RaisesThisRound = 0
	e.saveState()
}

// processCurrentPlayer is invoked when a bot or human is at the decision
// point. Mirrors GameEngine._process_current_player.
func (e *GameEngine) processCurrentPlayer() {
	if e.Game.GameFinished {
		return
	}
	active := []*Player{}
	for _, p := range e.Players {
		if !p.Folded {
			active = append(active, p)
		}
	}
	actionable := []*Player{}
	for _, p := range active {
		if !p.AllIn {
			actionable = append(actionable, p)
		}
	}
	if len(active) <= 1 || len(actionable) == 0 {
		e.advancePhase()
		return
	}

	idx := e.CurrentPlayerIx % len(e.Players)
	player := e.Players[idx]
	if player.Folded || player.AllIn {
		e.CurrentPlayerIx++
		e.saveState()
		return
	}

	if player.RoundBet >= e.Game.CurrentBet {
		cycles := countActionable(e.Players)
		if cycleCheck(e, idx, cycles) {
			e.advancePhase()
			return
		}
	}

	if player.IsBot {
		e.processBotAction(player)
	} else {
		e.saveState()
	}
}

// AdvanceOneStep is the user-facing step primitive used by the polling
// HTTP endpoint. Mirrors GameEngine.advance_one_step.
func (e *GameEngine) AdvanceOneStep() bool {
	if e.Game.GameFinished {
		return false
	}
	// During intermission the showdown modal owns the flow; callers
	// should use /api/game/next to start the next hand.
	if e.Game.Intermission {
		return true
	}
	if !e.Game.GameStarted {
		e.StartHand()
		return true
	}
	active := []*Player{}
	for _, p := range e.Players {
		if !p.Folded {
			active = append(active, p)
		}
	}
	actionable := []*Player{}
	for _, p := range active {
		if !p.AllIn {
			actionable = append(actionable, p)
		}
	}
	if len(active) <= 1 || len(actionable) == 0 {
		e.advancePhase()
		return true
	}
	idx := e.CurrentPlayerIx % len(e.Players)
	player := e.Players[idx]
	if player.Folded || player.AllIn {
		e.CurrentPlayerIx++
		e.saveState()
		return true
	}
	if player.RoundBet >= e.Game.CurrentBet {
		cycles := countActionable(e.Players)
		if cycleCheck(e, idx, cycles) {
			e.advancePhase()
			return true
		}
	}
	e.processCurrentPlayer()
	return true
}

// processBotAction builds a BotContext for the current player, asks the
// bot to choose an action, and applies the result. Mirrors
// GameEngine._process_bot_action.
func (e *GameEngine) processBotAction(player *Player) {
	playerPtrs := make([]*Player, len(e.Players))
	for i, p := range e.Players {
		playerPtrs[i] = p
	}
	ctx := BotContext{
		CurrentBet:      e.Game.CurrentBet,
		Pot:             e.Game.Pot,
		SmallBlind:      e.Game.SmallBlind,
		BigBlind:        e.Game.BigBlind,
		RaisesThisRound: e.Game.RaisesThisRound,
		CurrentPhaseIdx: e.Game.PhaseIndex,
		Players:         playerPtrs,
		LastRaise:       e.Game.LastRaise,
		CommunityCards:  append([]string{}, e.Game.CommunityCards...),
	}
	decision := ChooseBotAction(player, ctx)
	e.applyDecision(player, decision)
	e.CurrentPlayerIx++
	e.saveState()
}

// applyDecision mutates state in response to a decision returned by either
// the bot or the human endpoint. Mirrors GameEngine._apply_decision.
func (e *GameEngine) applyDecision(player *Player, decision *BotDecision) {
	needToCall := e.Game.CurrentBet - player.RoundBet
	e.updateStats(player, decision.Action)

	switch decision.Action {
	case "fold":
		player.Folded = true
		e.addNotification(fmt.Sprintf("%s folded.", player.Name))
	case "check":
		e.addNotification(fmt.Sprintf("%s checked.", player.Name))
	case "call":
		amt := decision.Amount
		if amt == 0 {
			amt = needToCall
		}
		bet := e.placeBet(player, amt)
		e.Game.Pot += bet
		e.addNotification(fmt.Sprintf("%s called %d.", player.Name, bet))
	case "raise":
		bet := decision.Amount
		if bet == 0 {
			bet = needToCall + e.Game.LastRaise
		}
		amt := e.placeBet(player, bet)
		if amt > needToCall {
			e.Game.CurrentBet = player.RoundBet
			e.Game.LastRaise = amt - needToCall
			e.Game.RaisesThisRound++
		}
		e.Game.Pot += amt
		e.addNotification(fmt.Sprintf("%s raised to %d.", player.Name, amt))
	case "allin":
		bet := decision.Amount
		if bet == 0 {
			bet = player.Chips + needToCall
		}
		amt := e.placeBet(player, bet)
		if amt > needToCall {
			e.Game.CurrentBet = player.RoundBet
			e.Game.LastRaise = amt - needToCall
			e.Game.RaisesThisRound++
		}
		e.Game.Pot += amt
		e.addNotification(fmt.Sprintf("%s went all-in for %d.", player.Name, amt))
	}
}

// updateStats mirrors GameEngine._update_stats.
func (e *GameEngine) updateStats(player *Player, action string) {
	if e.Game.PhaseIndex == 0 {
		if action == "call" || action == "raise" || action == "allin" {
			player.Stats.VPIP++
		}
		if action == "raise" || action == "allin" {
			player.Stats.PFR++
		}
	} else {
		if action == "raise" || action == "allin" {
			player.Stats.AggressiveActs++
		}
		if action == "call" {
			player.Stats.Calls++
		}
	}
	if (action == "raise" || action == "allin") && e.Game.PhaseIndex == 0 {
		for _, p := range e.Players {
			p.BotLine.PreflopAggressor = false
		}
		player.BotLine.PreflopAggressor = true
	}
	if action == "allin" {
		player.Stats.AllIns++
	}
	if action == "fold" {
		player.Stats.Folds++
		if e.Game.PhaseIndex == 0 {
			player.Stats.FoldsPreflop++
		} else {
			player.Stats.FoldsPostflop++
		}
	}
}

// advancePhase progresses the game to the next phase or, if the betting
// round is over, runs the showdown. Mirrors GameEngine._advance_phase.
func (e *GameEngine) advancePhase() {
	active := []*Player{}
	for _, p := range e.Players {
		if !p.Folded {
			active = append(active, p)
		}
	}
	if len(active) <= 1 {
		e.doShowdown()
		return
	}
	e.Game.PhaseIndex++
	if e.Game.PhaseIndex >= len(Phases) {
		e.doShowdown()
		return
	}
	phase := Phases[e.Game.PhaseIndex]
	switch phase {
	case "flop":
		e.dealCommunityCards(3)
		e.addNotification("Flop (3 cards) dealt.")
	case "turn":
		e.dealCommunityCards(1)
		e.addNotification("Turn (4th card) dealt.")
	case "river":
		e.dealCommunityCards(1)
		e.addNotification("River (5th card) dealt.")
	case "showdown":
		e.doShowdown()
		return
	}
	e.saveState()
	e.startBettingRound()
}

// doShowdown splits the pot into side pots and awards each one to the best
// remaining hand. Mirrors GameEngine._do_showdown + _build_side_pots +
// _resolve_side_pots.
func (e *GameEngine) doShowdown() {
	for _, p := range e.Players {
		p.RoundBet = 0
	}
	active := []*Player{}
	for _, p := range e.Players {
		if !p.Folded {
			active = append(active, p)
		}
	}
	contributors := []*Player{}
	for _, p := range e.Players {
		if p.TotalBet > 0 {
			contributors = append(contributors, p)
		}
	}
	hadShowdown := len(active) > 1
	if hadShowdown {
		for _, p := range active {
			p.Stats.Showdowns++
		}
	}
	if len(active) == 1 {
		winner := active[0]
		winner.Stats.HandsWon++
		winner.Chips += e.Game.Pot
		e.addNotification(fmt.Sprintf("%s wins %d!", winner.Name, e.Game.Pot))
		e.Game.LastWinner = &Winner{
			Name:         winner.Name,
			SeatIndex:    winner.SeatIndex,
			Amount:       e.Game.Pot,
			HandName:     e.SolveHandFor(winner),
			WinningCards: cardCodesFor(winner, e.Game.CommunityCards),
			HoleCards:    []string{winner.Cards[0], winner.Cards[1]},
			BoardCards:   append([]string{}, e.Game.CommunityCards...),
			IsTie:        false,
		}
		e.Game.PotAwards = []PotAward{{
			PotIndex: 0,
			Amount:   e.Game.Pot,
			Winner:   winner.Name,
			HandName: e.Game.LastWinner.HandName,
			Cards:    e.Game.LastWinner.WinningCards,
		}}
		e.beginIntermission()
		e.Game.Pot = 0
		e.Game.GameStarted = false
		e.saveState()
		return
	}

	contenders := append([]*Player{}, contributors...)
	sidePots := buildSidePots(contenders)
	e.resolveSidePots(sidePots, active, hadShowdown)
}

// beginIntermission flips the engine into intermission mode: the showdown
// modal is shown to clients and a 5-second countdown starts. The next
// hand is started either by the countdown elapsing (handled in JS) or
// by an explicit /api/game/next call.
func (e *GameEngine) beginIntermission() {
	e.Game.Intermission = true
	e.Game.IntermissionStartedAt = nowMillis()
	e.Game.GameStarted = false
}

// nowMillis returns the current Unix epoch in milliseconds. Pulled out so
// tests can substitute a deterministic clock if needed.
func nowMillis() int64 {
	return timeNow().UnixMilli()
}

// cardCodesFor returns the 5 card codes (uppercase rank + uppercase
// suit, matching the casing used for community cards in the API state)
// that make up the best hand a player can make from their hole + board.
// When the showdown happens before the flop (i.e. everyone else folded
// preflop) there are fewer than 5 community cards, so we return the
// player's two hole cards verbatim.
func cardCodesFor(p *Player, board []string) []string {
	if p == nil || p.Cards[0] == "" || p.Cards[0] == "1B" {
		return nil
	}
	upper := func(s string) string {
		if len(s) < 2 {
			return s
		}
		r := s[0]
		if r >= 'a' && r <= 'z' {
			r -= 'a' - 'A'
		}
		u := s[1]
		if u >= 'a' && u <= 'z' {
			u -= 'a' - 'A'
		}
		return string([]byte{r, u})
	}
	if len(board) < 3 {
		out := []string{}
		if p.Cards[0] != "" {
			out = append(out, upper(p.Cards[0]))
		}
		if p.Cards[1] != "" {
			out = append(out, upper(p.Cards[1]))
		}
		return out
	}
	full := []string{p.Cards[0], p.Cards[1]}
	full = append(full, board...)
	h := Solve(full, StandardRules())
	if h == nil || len(h.Cards) == 0 {
		return nil
	}
	out := make([]string, 0, len(h.Cards))
	for _, c := range h.Cards {
		v := c.Value
		if v == 0 {
			v = c.WildValue
		}
		if v >= 'a' && v <= 'z' {
			v -= 'a' - 'A'
		}
		u := c.Suit
		if u >= 'a' && u <= 'z' {
			u -= 'a' - 'A'
		}
		out = append(out, string([]byte{v, u}))
	}
	return out
}

// sidePot mirrors the dict literal used in engine._build_side_pots.
type sidePot struct {
	amount   int
	eligible []*Player
}

func buildSidePots(contenders []*Player) []sidePot {
	sortedContenders := append([]*Player{}, contenders...)
	sort.SliceStable(sortedContenders, func(i, j int) bool {
		return sortedContenders[i].TotalBet < sortedContenders[j].TotalBet
	})

	pots := []sidePot{}
	prev := 0
	for _, c := range sortedContenders {
		lvl := c.TotalBet
		diff := lvl - prev
		if diff > 0 {
			startIdx := indexOf(sortedContenders, c)
			eligible := append([]*Player{}, sortedContenders[startIdx:]...)
			pots = append(pots, sidePot{amount: diff * len(eligible), eligible: eligible})
			prev = lvl
		}
	}

	i := 0
	for i < len(pots)-1 {
		eligA := []*Player{}
		for _, p := range pots[i].eligible {
			if !p.Folded {
				eligA = append(eligA, p)
			}
		}
		eligB := []*Player{}
		for _, p := range pots[i+1].eligible {
			if !p.Folded {
				eligB = append(eligB, p)
			}
		}
		if len(eligA) == len(eligB) && subset(eligA, eligB) {
			pots[i].amount += pots[i+1].amount
			pots = append(pots[:i+1], pots[i+2:]...)
		} else {
			i++
		}
	}
	return pots
}

func indexOf(arr []*Player, target *Player) int {
	for i, p := range arr {
		if p == target {
			return i
		}
	}
	return 0
}

func subset(a, b []*Player) bool {
	for _, x := range a {
		found := false
		for _, y := range b {
			if x == y {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

func (e *GameEngine) resolveSidePots(pots []sidePot, active []*Player, hadShowdown bool) {
	winnersSet := map[*Player]bool{}
	awards := []PotAward{}
	winnerNames := []string{}

	for potIdx, sp := range pots {
		eligible := []*Player{}
		for _, p := range sp.eligible {
			if !p.Folded {
				eligible = append(eligible, p)
			}
		}
		if len(eligible) == 1 {
			sole := eligible[0]
			sole.Chips += sp.amount
			if !winnersSet[sole] {
				sole.Stats.HandsWon++
				if hadShowdown {
					sole.Stats.ShowdownsWon++
				}
				winnersSet[sole] = true
				winnerNames = append(winnerNames, sole.Name)
			}
			e.addNotification(fmt.Sprintf("%s wins %d.", sole.Name, sp.amount))
			awards = append(awards, PotAward{
				PotIndex: potIdx,
				Amount:   sp.amount,
				Winner:   sole.Name,
				HandName: e.SolveHandFor(sole),
				Cards:    cardCodesFor(sole, e.Game.CommunityCards),
			})
			continue
		}

		type entry struct {
			player *Player
			hand   *Hand
		}
		entries := []entry{}
		for _, p := range eligible {
			cards := []string{p.Cards[0], p.Cards[1]}
			cards = append(cards, e.Game.CommunityCards...)
			hand := Solve(cards, StandardRules())
			if hand == nil {
				continue
			}
			entries = append(entries, entry{player: p, hand: hand})
		}
		if len(entries) == 0 {
			continue
		}

		hands := []*Hand{}
		for _, x := range entries {
			hands = append(hands, x.hand)
		}
		winning := Winners(hands)
		share := sp.amount / len(winning)
		remainder := sp.amount - share*len(winning)
		for _, w := range winning {
			var ent *entry
			for i := range entries {
				if entries[i].hand == w {
					ent = &entries[i]
					break
				}
			}
			if ent == nil {
				continue
			}
			payout := share
			if remainder > 0 {
				payout++
				remainder--
			}
			ent.player.Chips += payout
			if !winnersSet[ent.player] {
				ent.player.Stats.HandsWon++
				if hadShowdown {
					ent.player.Stats.ShowdownsWon++
				}
				winnersSet[ent.player] = true
				winnerNames = append(winnerNames, ent.player.Name)
			}
		}

		if len(winning) == 1 {
			for _, ent := range entries {
				if ent.hand == winning[0] {
					e.addNotification(fmt.Sprintf("%s wins %d with %s.", ent.player.Name, sp.amount, winning[0].Descr))
					awards = append(awards, PotAward{
						PotIndex: potIdx,
						Amount:   sp.amount,
						Winner:   ent.player.Name,
						HandName: winning[0].Descr,
						Cards:    cardCodes(winning[0].Cards),
					})
				}
			}
		} else {
			names := []string{}
			for _, w := range winning {
				for _, ent := range entries {
					if ent.hand == w {
						names = append(names, ent.player.Name)
					}
				}
			}
			e.addNotification(fmt.Sprintf("%s split %d.", strings.Join(names, " & "), sp.amount))
			// Record the award under each tied winner for the animation.
			share := sp.amount / len(winning)
			remainder := sp.amount - share*len(winning)
			for i, w := range winning {
				amt := share
				if i == 0 && remainder > 0 {
					amt += remainder
				}
				for _, ent := range entries {
					if ent.hand == w {
						awards = append(awards, PotAward{
							PotIndex: potIdx,
							Amount:   amt,
							Winner:   ent.player.Name,
							HandName: w.Descr,
							Cards:    cardCodes(w.Cards),
						})
					}
				}
			}
		}
	}
	e.Game.Pot = 0

	// Build the headline Winner entry. For multi-pot splits we pick the
	// pot with the largest payout and use its winner as the headline.
	e.Game.LastWinner = buildHeadlineWinner(winnerNames, awards, e.Players)
	e.Game.PotAwards = awards
	e.beginIntermission()
	e.Game.GameStarted = false
	e.saveState()
}

// buildHeadlineWinner constructs the Winner struct surfaced to the UI.
// For tied pots the headline names all winners; for a single winner it
// picks the highest-paying pot's winner and reports the full hand.
func buildHeadlineWinner(names []string, awards []PotAward, players []*Player) *Winner {
	if len(awards) == 0 {
		return nil
	}
	// Pick the largest award.
	top := awards[0]
	for _, a := range awards[1:] {
		if a.Amount > top.Amount {
			top = a
		}
	}

	// Look up the player record to fill in seat index + hole cards.
	var wp *Player
	for _, p := range players {
		if p.Name == top.Winner {
			wp = p
			break
		}
	}
	winner := &Winner{
		Name:         top.Winner,
		Amount:       top.Amount,
		HandName:     top.HandName,
		WinningCards: append([]string{}, top.Cards...),
		BoardCards:   nil,
		HoleCards:    nil,
		IsTie:        false,
	}
	if wp != nil {
		winner.SeatIndex = wp.SeatIndex
		winner.HoleCards = []string{wp.Cards[0], wp.Cards[1]}
	}

	// Detect ties: if multiple winners split the largest pot, mark as tie.
	ties := []string{}
	for _, a := range awards {
		if a.Amount == top.Amount && a.PotIndex == top.PotIndex {
			ties = append(ties, a.Winner)
		}
	}
	if len(ties) > 1 {
		winner.IsTie = true
		winner.TiedWith = ties
	}
	return winner
}

// HumanAction is the entry point used by the HTTP server when a human
// submits a fold/check/call/raise decision. Mirrors
// GameEngine.human_action.
func (e *GameEngine) HumanAction(playerName, action string, amount int) bool {
	var player *Player
	for _, p := range e.Players {
		if p.Name == playerName && !p.Folded {
			player = p
			break
		}
	}
	if player == nil || player.IsBot || player.AllIn {
		return false
	}

	decision := &BotDecision{Action: action, Amount: amount}
	e.applyDecision(player, decision)
	e.CurrentPlayerIx++
	e.saveState()

	if e.Game.GameFinished {
		return true
	}
	active := []*Player{}
	for _, p := range e.Players {
		if !p.Folded {
			active = append(active, p)
		}
	}
	actionable := []*Player{}
	for _, p := range active {
		if !p.AllIn {
			actionable = append(actionable, p)
		}
	}
	if len(active) <= 1 || len(actionable) <= 1 {
		e.advancePhase()
	} else {
		e.processCurrentPlayer()
	}
	return true
}

// NextHand starts a new hand if the game isn't finished. Mirrors
// GameEngine.next_hand.
func (e *GameEngine) NextHand() {
	if !e.Game.GameFinished {
		e.StartHand()
	}
}

// AddOrRenameSeat either activates a free seat for the supplied name or
// renames an existing seat. Only allowed while the engine is in
// intermission.
func (e *GameEngine) AddOrRenameSeat(name string, seatIndex *int) error {
	if !e.Game.Intermission {
		return errors.New("seat changes only allowed during intermission")
	}
	if name == "" {
		return errors.New("name is required")
	}

	// Renaming an existing seat by name?
	for i := range e.Players {
		if e.Players[i].Name == name {
			return nil // already present
		}
	}

	// Use the requested seat index, else pick the lowest free slot.
	idx := -1
	if seatIndex != nil {
		if *seatIndex < 0 || *seatIndex >= MaxSeats {
			return fmt.Errorf("seat_index %d out of range", *seatIndex)
		}
		idx = *seatIndex
	} else {
		used := map[int]bool{}
		for _, p := range e.Players {
			used[p.SeatIndex] = true
		}
		for i := 0; i < MaxSeats; i++ {
			if !used[i] {
				idx = i
				break
			}
		}
		if idx < 0 {
			return errors.New("no free seats")
		}
	}

	// If the seat is occupied, refuse; rename via separate flow if needed.
	for _, p := range e.Players {
		if p.SeatIndex == idx {
			return fmt.Errorf("seat %d already occupied by %s", idx, p.Name)
		}
	}

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
	e.saveState()
	return nil
}

// RemoveSeat drops the player at the supplied seat index. Only allowed
// during intermission.
func (e *GameEngine) RemoveSeat(seatIndex int) error {
	if !e.Game.Intermission {
		return errors.New("seat changes only allowed during intermission")
	}
	for i, p := range e.Players {
		if p.SeatIndex == seatIndex {
			e.Players = append(e.Players[:i], e.Players[i+1:]...)
			e.saveState()
			return nil
		}
	}
	return fmt.Errorf("seat %d is empty", seatIndex)
}

// saveState writes the engine's current state to its storage layer. Mirrors
// GameEngine.save_state.
func (e *GameEngine) saveState() {
	if e.Game == nil || e.Storage == nil {
		return
	}
	if e.Game.PhaseIndex < len(Phases) {
		e.Game.Phase = Phases[e.Game.PhaseIndex]
	} else {
		e.Game.Phase = Phases[0]
	}
	e.Game.Version++
	if len(e.Game.Notifications) > MaxNotifications {
		e.Game.Notifications = e.Game.Notifications[len(e.Game.Notifications)-MaxNotifications:]
	}
	for i, p := range e.Players {
		p.SeatIndex = i
	}
	if err := e.Storage.SaveGame(e.Game, e.Players); err != nil {
		fmt.Printf("save_state error: %v\n", err)
	}
	if e.OnStateChanged != nil {
		e.OnStateChanged()
	}
}

// SolveHandFor returns the descriptive name of the best 5-card hand a
// player can make from their hole cards + the community cards. The
// returned string is empty when the player is folded, has no cards, or
// the community cards haven't been dealt yet (preflop).
//
// The hand is recomputed here (rather than reading Player.HandName) so
// callers get a fresh evaluation against the *current* board even if the
// engine state was loaded from storage.
func (e *GameEngine) SolveHandFor(p *Player) string {
	if p == nil || p.Folded {
		return ""
	}
	if p.Cards[0] == "" || p.Cards[0] == "1B" || p.Cards[1] == "" || p.Cards[1] == "1B" {
		return ""
	}
	if len(e.Game.CommunityCards) < 3 {
		return ""
	}
	full := []string{p.Cards[0], p.Cards[1]}
	full = append(full, e.Game.CommunityCards...)
	h := Solve(full, StandardRules())
	if h == nil {
		return ""
	}
	return h.Descr
}

// ToDict serializes the engine state into the dict shape returned by
// GameEngine.to_dict. The slot for each player is "1B","1B" if the cards
// aren't revealed (mirrors the open_cards / spectator_mode rule).
func (e *GameEngine) ToDict() map[string]interface{} {
	reveal := e.Game.OpenCardsMode || e.Game.SpectatorMode
	phase := ""
	if e.Game.PhaseIndex < len(Phases) {
		phase = Phases[e.Game.PhaseIndex]
	}
	players := make([]map[string]interface{}, 0, len(e.Players))
	for _, p := range e.Players {
		cards := [2]string{"1B", "1B"}
		if reveal {
			cards = p.Cards
		}
		handName := ""
		if reveal {
			handName = e.SolveHandFor(p)
		}
		players = append(players, map[string]interface{}{
			"name":            p.Name,
			"chips":           p.Chips,
			"round_bet":       p.RoundBet,
			"total_bet":       p.TotalBet,
			"folded":          p.Folded,
			"all_in":          p.AllIn,
			"is_bot":          p.IsBot,
			"dealer":          p.IsDealer,
			"small_blind":     p.IsSmallBlind,
			"big_blind":       p.IsBigBlind,
			"cards":           cards,
			"seat_index":      p.SeatIndex,
			"stats":           p.Stats,
			"win_probability": p.WinProbability,
			"hand_name":       handName,
		})
	}
	notifs := e.Game.Notifications
	if len(notifs) > MaxNotifications {
		notifs = notifs[len(notifs)-MaxNotifications:]
	}
	return map[string]interface{}{
		"phase":                  phase,
		"pot":                    e.Game.Pot,
		"current_bet":            e.Game.CurrentBet,
		"last_raise":             e.Game.LastRaise,
		"small_blind":            e.Game.SmallBlind,
		"big_blind":              e.Game.BigBlind,
		"raises_this_round":      e.Game.RaisesThisRound,
		"dealer_orbit_count":     e.Game.DealerOrbitCount,
		"community_cards":        e.Game.CommunityCards,
		"players":                players,
		"game_started":           e.Game.GameStarted,
		"game_finished":          e.Game.GameFinished,
		"open_cards_mode":        e.Game.OpenCardsMode,
		"spectator_mode":         e.Game.SpectatorMode,
		"total_hands":            e.Game.TotalHands,
		"notifications":          notifs,
		"version":                e.Game.Version,
		"intermission":           e.Game.Intermission,
		"intermission_started_at": e.Game.IntermissionStartedAt,
		"winner":                 e.Game.LastWinner,
		"pot_awards":             e.Game.PotAwards,
	}
}

// countActionable mirrors the `sum(1 for p in players if not p.folded and
// not p.all_in)` expression used in multiple places.
func countActionable(players []*Player) int {
	n := 0
	for _, p := range players {
		if !p.Folded && !p.AllIn {
			n++
		}
	}
	return n
}

// cycleCheck mirrors engine.cycle_check.
func cycleCheck(e *GameEngine, idx, cycles int) bool {
	player := e.Players[idx%len(e.Players)]
	if player.Folded || player.AllIn {
		return false
	}
	if player.RoundBet >= e.Game.CurrentBet {
		if e.Game.PhaseIndex > 0 && e.Game.CurrentBet == 0 {
			return false
		}
		return cycles >= countActionable(e.Players)
	}
	return false
}

func derefString(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// GameState is the in-memory registry that maps table_id to GameEngine.
//
// Mirrors engine.GameState. When the supplied storage is the SQLite
// implementation, each engine is backed by its own database row.
type GameState struct {
	Storage  Storage
	engines  map[string]*GameEngine
	mu       sync.Mutex
}

// NewGameState constructs an empty GameState.
func NewGameState(storage Storage) *GameState {
	return &GameState{Storage: storage, engines: map[string]*GameEngine{}}
}

// GetOrCreate returns the engine for a given table, creating it if needed.
// Mirrors GameState.get_or_create.
func (gs *GameState) GetOrCreate(tableID string) (*GameEngine, bool, error) {
	gs.mu.Lock()
	defer gs.mu.Unlock()
	if eng, ok := gs.engines[tableID]; ok {
		return eng, false, nil
	}
	g, players, created, err := gs.Storage.GetOrCreateGame(tableID)
	if err != nil {
		return nil, false, err
	}
	eng := &GameEngine{
		Game:    g,
		Players: players,
		Storage: gs.Storage,
	}
	gs.engines[tableID] = eng
	return eng, created, nil
}

// Engines returns the registered engines (read-only access).
func (gs *GameState) Engines() map[string]*GameEngine {
	gs.mu.Lock()
	defer gs.mu.Unlock()
	out := make(map[string]*GameEngine, len(gs.engines))
	for k, v := range gs.engines {
		out[k] = v
	}
	return out
}

// SetEngine stores an externally built engine under the given table id.
func (gs *GameState) SetEngine(tableID string, eng *GameEngine) {
	gs.mu.Lock()
	defer gs.mu.Unlock()
	gs.engines[tableID] = eng
}

// Remove drops a table id from the in-memory registry.
func (gs *GameState) Remove(tableID string) {
	gs.mu.Lock()
	defer gs.mu.Unlock()
	delete(gs.engines, tableID)
}