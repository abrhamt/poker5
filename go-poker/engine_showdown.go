package poker

import (
	"fmt"
	"sort"
)

type playerSolve struct {
	player *Player
	hand   *Hand
}

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

func subset(a, b []*Player) bool {
	m := map[*Player]bool{}
	for _, p := range b {
		m[p] = true
	}
	for _, p := range a {
		if !m[p] {
			return false
		}
	}
	return true
}

func indexOf(players []*Player, target *Player) int {
	for i, p := range players {
		if p == target {
			return i
		}
	}
	return 0
}

func nowMillis() int64 {
	return timeNow().UnixMilli()
}

func (e *GameEngine) beginIntermission() {
	e.Game.Intermission = true
	e.Game.IntermissionStartedAt = nowMillis()
	e.Game.GameStarted = false
}

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
		e.saveState()
		return
	}

	contenders := append([]*Player{}, contributors...)
	sidePots := buildSidePots(contenders)
	e.resolveSidePots(sidePots, active, hadShowdown)
}

func (e *GameEngine) resolveSidePots(pots []sidePot, active []*Player, hadShowdown bool) {
	winnersSet := map[*Player]bool{}
	awards := []PotAward{}
	var headlineWinner *Winner

	for potIdx, sp := range pots {
		eligible := []*Player{}
		for _, p := range sp.eligible {
			if !p.Folded {
				eligible = append(eligible, p)
			}
		}
		if len(eligible) == 0 {
			continue
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
			}
			e.addNotification(fmt.Sprintf("%s wins %d.", sole.Name, sp.amount))
			awards = append(awards, PotAward{
				PotIndex: potIdx,
				Amount:   sp.amount,
				Winner:   sole.Name,
				HandName: e.SolveHandFor(sole),
				Cards:    cardCodesFor(sole, e.Game.CommunityCards),
			})
			if headlineWinner == nil || sp.amount > headlineWinner.Amount {
				headlineWinner = &Winner{
					Name:         sole.Name,
					SeatIndex:    sole.SeatIndex,
					Amount:       sp.amount,
					HandName:     "Uncontested",
					WinningCards: cardCodesFor(sole, e.Game.CommunityCards),
					HoleCards:    []string{sole.Cards[0], sole.Cards[1]},
					BoardCards:   append([]string{}, e.Game.CommunityCards...),
				}
			}
			continue
		}

		solves := []playerSolve{}
		for _, p := range eligible {
			full := append([]string{p.Cards[0], p.Cards[1]}, e.Game.CommunityCards...)
			h := Solve(full, StandardRules())
			if h != nil {
				solves = append(solves, playerSolve{player: p, hand: h})
			}
		}
		if len(solves) == 0 {
			continue
		}
		hands := []*Hand{}
		for _, s := range solves {
			hands = append(hands, s.hand)
		}
		winningHands := Winners(hands)
		winners := []*Player{}
		for _, w := range winningHands {
			for _, s := range solves {
				if s.hand.Compare(w) == 0 {
					winners = append(winners, s.player)
				}
			}
		}

		if len(winners) > 0 {
			share := sp.amount / len(winners)
			for _, w := range winners {
				w.Chips += share
				if !winnersSet[w] {
					w.Stats.HandsWon++
					w.Stats.ShowdownsWon++
					winnersSet[w] = true
				}
			}
			bestHand := winningHands[0]
			cards := cardCodes(bestHand.Cards)
			awards = append(awards, PotAward{
				PotIndex: potIdx,
				Amount:   share,
				Winner:   winners[0].Name,
				HandName: bestHand.Descr,
				Cards:    cards,
			})
			if headlineWinner == nil || share > headlineWinner.Amount {
				headlineWinner = &Winner{
					Name:         winners[0].Name,
					SeatIndex:    winners[0].SeatIndex,
					Amount:       share,
					HandName:     bestHand.Descr,
					WinningCards: cards,
					HoleCards:    []string{winners[0].Cards[0], winners[0].Cards[1]},
					BoardCards:   append([]string{}, e.Game.CommunityCards...),
					IsTie:        len(winners) > 1,
				}
			}
		}
	}

	e.Game.PotAwards = awards
	e.Game.LastWinner = headlineWinner
	e.beginIntermission()
	e.Game.Pot = 0
	e.saveState()
}



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
