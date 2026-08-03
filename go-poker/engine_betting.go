package poker

import (
	"fmt"
	"strings"
)

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

	if len(e.Players) == 1 {
		champion := e.Players[0]
		e.addNotification(fmt.Sprintf("%s wins the game!", champion.Name))
		e.Game.GameFinished = true
		e.saveState()
		return false
	}

	if len(e.Game.Deck) < len(e.Players)*2+5 {
		e.Game.Deck = ShuffleDeck(FullDeck)
	}

	e.rotateDealerAndBlinds()
	e.dealHoleCards()
	e.startBettingRound()
	return true
}

func (e *GameEngine) rotateDealerAndBlinds() {
	for _, p := range e.Players {
		p.IsDealer = false
		p.IsSmallBlind = false
		p.IsBigBlind = false
	}

	if e.Game.DealerOrbitCount < 0 {
		e.Game.DealerOrbitCount = 0
	} else {
		e.Game.DealerOrbitCount = (e.Game.DealerOrbitCount + 1) % len(e.Players)
	}
	dealerIdx := e.Game.DealerOrbitCount
	e.Players[dealerIdx].IsDealer = true

	sbIdx := (dealerIdx + 1) % len(e.Players)
	bbIdx := (dealerIdx + 2) % len(e.Players)
	if len(e.Players) == 2 {
		sbIdx = dealerIdx
		bbIdx = (dealerIdx + 1) % len(e.Players)
	}
	e.Players[sbIdx].IsSmallBlind = true
	e.Players[bbIdx].IsBigBlind = true

	sbAmt := e.placeBet(e.Players[sbIdx], e.Game.SmallBlind)
	bbAmt := e.placeBet(e.Players[bbIdx], e.Game.BigBlind)
	e.Game.Pot = sbAmt + bbAmt
	e.Game.CurrentBet = e.Game.BigBlind
	e.Game.LastRaise = e.Game.BigBlind
}

func (e *GameEngine) placeBet(p *Player, amount int) int {
	actual := amount
	if actual > p.Chips {
		actual = p.Chips
	}
	p.Chips -= actual
	p.RoundBet += actual
	p.TotalBet += actual
	if p.Chips == 0 {
		p.AllIn = true
	}
	return actual
}

func (e *GameEngine) dealHoleCards() {
	for _, p := range e.Players {
		if len(e.Game.Deck) >= 2 {
			p.Cards[0] = e.Game.Deck[0]
			p.Cards[1] = e.Game.Deck[1]
			e.Game.Deck = e.Game.Deck[2:]
		}
	}
}

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

func (e *GameEngine) dealCommunityCards(count int) {
	for i := 0; i < count && len(e.Game.Deck) > 0; i++ {
		card := e.Game.Deck[0]
		e.Game.Deck = e.Game.Deck[1:]
		e.Game.CommunityCards = append(e.Game.CommunityCards, card)
		e.Game.CardGraveyard = append(e.Game.CardGraveyard, card)
	}
}

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

func (e *GameEngine) AdvanceOneStep() bool {
	if e.Game.GameFinished {
		return false
	}
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

func (e *GameEngine) processCurrentPlayer() {
	if e.Game.GameFinished {
		return
	}
	idx := e.CurrentPlayerIx % len(e.Players)
	player := e.Players[idx]

	if player.IsBot {
		e.processBotAction(player)
	} else {
		e.saveState()
	}
}

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
	}
}

func countActionable(players []*Player) int {
	cnt := 0
	for _, p := range players {
		if !p.Folded && !p.AllIn {
			cnt++
		}
	}
	return cnt
}

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
	return true
}

func (e *GameEngine) AddOrRenameSeat(name string, seatIndex *int) error {
	if name == "" {
		return fmt.Errorf("name required")
	}
	if e.Game.GameStarted && !e.Game.Intermission && !e.Game.GameFinished {
		return fmt.Errorf("cannot alter seats mid-hand")
	}
	isBot := strings.HasPrefix(strings.ToLower(name), "bot")
	if seatIndex != nil {
		for _, p := range e.Players {
			if p.SeatIndex == *seatIndex {
				p.Name = name
				p.IsBot = isBot
				e.saveState()
				return nil
			}
		}
	}
	if len(e.Players) >= MaxSeats {
		return fmt.Errorf("table full")
	}
	idx := len(e.Players)
	if seatIndex != nil {
		idx = *seatIndex
	}
	p := &Player{
		Name:      name,
		SeatIndex: idx,
		IsBot:     isBot,
		Chips:     StartingChips,
		Cards:     [2]string{"1B", "1B"},
		Stats:     NewStats(),
		BotLine:   NewBotLine(),
	}
	e.Players = append(e.Players, p)
	e.saveState()
	return nil
}

func (e *GameEngine) RemoveSeat(seatIndex int) error {
	for i, p := range e.Players {
		if p.SeatIndex == seatIndex {
			e.Players = append(e.Players[:i], e.Players[i+1:]...)
			e.saveState()
			return nil
		}
	}
	return fmt.Errorf("seat %d is empty", seatIndex)
}

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
