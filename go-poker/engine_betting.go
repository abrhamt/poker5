package poker

import (
	"fmt"
)

// dealtIn reports whether a player takes part in the next hand. Sitting-out
// players keep their seat and their chips but are dealt out entirely — no
// cards, no blinds, no turn — which is what stops an absent player's stack
// draining one orbit at a time.
func (e *GameEngine) dealtIn(p *Player) bool {
	return p.Chips > 0 && !p.SittingOut
}

// DealtInCount is how many seated players would actually take part in a hand.
func (e *GameEngine) DealtInCount() int {
	return len(e.dealtInPlayers())
}

func (e *GameEngine) dealtInPlayers() []*Player {
	out := make([]*Player, 0, len(e.Players))
	for _, p := range e.Players {
		if e.dealtIn(p) {
			out = append(out, p)
		}
	}
	return out
}

func (e *GameEngine) StartHand() bool {
	// Seats can outnumber players: with everyone but one sitting out there is
	// nobody to play against, and the table just waits.
	if len(e.dealtInPlayers()) < 2 {
		return false
	}

	e.Game.GameStarted = true
	e.Game.TotalHands++
	e.Game.PhaseIndex = 0
	e.Game.Phase = Phases[0]
	e.Game.Pot = 0
	e.Game.CurrentBet = 0
	e.Game.LastRaise = e.Game.BigBlind
	e.Game.RaisesThisRound = 0
	e.Game.CommunityCards = []string{}
	e.Game.Intermission = false
	e.Game.LastWinner = nil
	e.Game.PotAwards = nil
	e.Game.Payouts = nil

	for _, p := range e.Players {
		p.Folded = false
		p.AllIn = false
		p.HasActed = false
		p.TotalBet = 0
		p.RoundBet = 0
		p.WinProbability = nil
		p.Cards = [2]string{"1B", "1B"}
		p.BotLine = NewBotLine()
		// Dealt out, so every "is this player still in?" check downstream —
		// turn order, round completion, showdown — skips them for free.
		p.Folded = p.SittingOut
	}

	active := []*Player{}
	for _, p := range e.Players {
		if p.Chips > 0 {
			active = append(active, p)
		} else {
			e.addNotification(NoteSystem, fmt.Sprintf("%s is out of chips!", p.Name))
		}
	}
	e.Players = active

	if len(e.Players) < 2 {
		if len(e.Players) == 1 {
			e.addNotification(NoteResult, fmt.Sprintf("%s wins the game!", e.Players[0].Name))
			e.Game.GameFinished = true
		}
		e.saveState()
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

	// The button and the blinds walk the dealt-in players, never the raw seat
	// list: posting a blind for someone who is sitting out is exactly the leak
	// this is here to close.
	dealt := e.dealtInPlayers()
	if len(dealt) < 2 {
		return
	}

	if e.Game.DealerOrbitCount < 0 {
		e.Game.DealerOrbitCount = 0
	} else {
		e.Game.DealerOrbitCount = (e.Game.DealerOrbitCount + 1) % len(dealt)
	}
	dealerIdx := e.Game.DealerOrbitCount % len(dealt)
	dealt[dealerIdx].IsDealer = true

	sbIdx := (dealerIdx + 1) % len(dealt)
	bbIdx := (dealerIdx + 2) % len(dealt)
	if len(dealt) == 2 {
		sbIdx = dealerIdx
		bbIdx = (dealerIdx + 1) % len(dealt)
	}
	dealt[sbIdx].IsSmallBlind = true
	dealt[bbIdx].IsBigBlind = true

	sbAmt := e.placeBet(dealt[sbIdx], e.Game.SmallBlind)
	bbAmt := e.placeBet(dealt[bbIdx], e.Game.BigBlind)
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
		if p.Folded {
			continue
		}
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
	if e.Game.PhaseIndex >= len(Phases)-1 {
		e.doShowdown()
		return
	}

	e.Game.Phase = Phases[e.Game.PhaseIndex]
	switch e.Game.Phase {
	case "flop":
		e.dealCommunityCards(3)
		e.addNotification(NotePhase, "Flop dealt.")
	case "turn":
		e.dealCommunityCards(1)
		e.addNotification(NotePhase, "Turn dealt.")
	case "river":
		e.dealCommunityCards(1)
		e.addNotification(NotePhase, "River dealt.")
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
	if e.Game.PhaseIndex >= len(Phases)-1 {
		e.doShowdown()
		return
	}

	e.Game.Phase = Phases[e.Game.PhaseIndex]
	for _, p := range e.Players {
		p.HasActed = false
	}

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
	if len(active) <= 1 {
		e.doShowdown()
		return
	}

	actionable := []*Player{}
	for _, p := range active {
		if !p.AllIn {
			actionable = append(actionable, p)
		}
	}
	if len(actionable) <= 1 && e.isAllInSettled() {
		e.advancePhase()
		return
	}

	if e.Game.PhaseIndex == 0 {
		if len(e.Players) == 2 {
			dealerIdx := 0
			for i, p := range e.Players {
				if p.IsDealer {
					dealerIdx = i
					break
				}
			}
			e.CurrentPlayerIx = dealerIdx
		} else {
			bbIdx := 0
			for i, p := range e.Players {
				if p.IsBigBlind {
					bbIdx = i
					break
				}
			}
			e.CurrentPlayerIx = (bbIdx + 1) % len(e.Players)
		}
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

	e.findNextActionablePlayer()
	e.Game.RaisesThisRound = 0
	e.saveState()
}

func (e *GameEngine) isAllInSettled() bool {
	var maxBet int
	for _, p := range e.Players {
		if !p.Folded && p.RoundBet > maxBet {
			maxBet = p.RoundBet
		}
	}
	for _, p := range e.Players {
		if !p.Folded && !p.AllIn && p.RoundBet < maxBet {
			return false
		}
	}
	return true
}

func (e *GameEngine) isBettingRoundComplete() bool {
	activeCount := 0
	for _, p := range e.Players {
		if !p.Folded {
			activeCount++
		}
	}
	if activeCount <= 1 {
		return true
	}

	actionableCount := 0
	for _, p := range e.Players {
		if !p.Folded && !p.AllIn {
			actionableCount++
			if !p.HasActed || p.RoundBet < e.Game.CurrentBet {
				return false
			}
		}
	}

	if actionableCount <= 1 && e.isAllInSettled() {
		return true
	}
	return actionableCount > 0
}

func (e *GameEngine) findNextActionablePlayer() {
	n := len(e.Players)
	if n == 0 {
		return
	}
	for i := 0; i < n; i++ {
		idx := (e.CurrentPlayerIx + i) % n
		p := e.Players[idx]
		if !p.Folded && !p.AllIn {
			e.CurrentPlayerIx = idx
			return
		}
	}
}

func (e *GameEngine) AdvanceOneStep() bool {
	if e.Game.GameFinished || e.Game.Intermission {
		return false
	}
	if !e.Game.GameStarted {
		e.StartHand()
		return true
	}
	if e.isBettingRoundComplete() {
		e.advancePhase()
		return true
	}
	e.processCurrentPlayer()
	return true
}

func (e *GameEngine) processCurrentPlayer() {
	if e.Game.GameFinished || e.Game.Intermission || len(e.Players) == 0 {
		return
	}
	idx := e.CurrentPlayerIx % len(e.Players)
	player := e.Players[idx]

	if player.Folded || player.AllIn {
		e.CurrentPlayerIx++
		e.findNextActionablePlayer()
		idx = e.CurrentPlayerIx % len(e.Players)
		player = e.Players[idx]
	}

	if player.IsBot && !player.Folded && !player.AllIn {
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

	if e.isBettingRoundComplete() {
		e.advancePhase()
	} else {
		e.CurrentPlayerIx++
		e.findNextActionablePlayer()
		e.saveState()
	}
}

func (e *GameEngine) applyDecision(player *Player, decision *BotDecision) {
	needToCall := e.Game.CurrentBet - player.RoundBet
	e.updateStats(player, decision.Action)
	player.HasActed = true

	switch decision.Action {
	case "fold":
		player.Folded = true
		e.addNotification(NoteAction, fmt.Sprintf("%s folded.", player.Name))
	case "check":
		e.addNotification(NoteAction, fmt.Sprintf("%s checked.", player.Name))
	case "call":
		amt := decision.Amount
		if amt <= 0 {
			amt = needToCall
		}
		bet := e.placeBet(player, amt)
		e.Game.Pot += bet
		e.addNotification(NoteAction, fmt.Sprintf("%s called %d.", player.Name, bet))
	case "raise":
		bet := decision.Amount
		if bet <= 0 {
			bet = needToCall + e.Game.LastRaise
		}
		amt := e.placeBet(player, bet)
		if amt > needToCall {
			e.Game.CurrentBet = player.RoundBet
			e.Game.LastRaise = amt - needToCall
			e.Game.RaisesThisRound++
			for _, other := range e.Players {
				if other.Name != player.Name && !other.Folded && !other.AllIn {
					other.HasActed = false
				}
			}
		}
		e.Game.Pot += amt
		e.addNotification(NoteAction, fmt.Sprintf("%s raised to %d.", player.Name, amt))
	}
}

func (e *GameEngine) HumanAction(playerName, action string, amount int) bool {
	var player *Player
	var pIdx int
	for i, p := range e.Players {
		if p.Name == playerName && !p.Folded {
			player = p
			pIdx = i
			break
		}
	}
	if player == nil || player.AllIn {
		return false
	}
	if (e.CurrentPlayerIx % len(e.Players)) != pIdx {
		return false
	}

	decision := &BotDecision{Action: action, Amount: amount}
	e.applyDecision(player, decision)

	if e.isBettingRoundComplete() {
		e.advancePhase()
	} else {
		e.CurrentPlayerIx++
		e.findNextActionablePlayer()
		e.saveState()
		e.processCurrentPlayer()
	}
	return true
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
