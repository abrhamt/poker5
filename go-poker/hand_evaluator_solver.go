package poker

import (
	"strings"
)

func _handRankings() map[string]int {
	return map[string]int{
		HandRoyalFlush:    9,
		HandStraightFlush: 9,
		HandFourOfAKind:   8,
		HandFullHouse:     7,
		HandFlush:         6,
		HandStraight:      5,
		HandThreeOfAKind:  4,
		HandTwoPair:       3,
		HandOnePair:       2,
		HandHighCard:      1,
	}
}

func Solve(cards []string, rules GameRules) *Hand {
	if len(cards) == 0 {
		cards = []string{""}
	}
	if rules.CardsInHand == 0 {
		rules = StandardRules()
	}
	codes := make([]string, len(cards))
	copy(codes, cards)

	if rules.WildValue == 0 {
		seen := map[string]bool{}
		for _, c := range codes {
			if seen[c] {
				return &Hand{Name: HandHighCard, Descr: "Invalid", Rules: rules, IsPossible: false}
			}
			seen[c] = true
		}
	}

	handRanks := _handRankings()

	for _, name := range []string{HandStraightFlush, HandFourOfAKind, HandFullHouse, HandFlush, HandStraight, HandThreeOfAKind, HandTwoPair, HandOnePair, HandHighCard} {
		h := newHand(codes, name, rules, handRanks)
		switch name {
		case HandStraightFlush:
			solveStraightFlush(h)
		case HandFourOfAKind:
			solveFourOfAKind(h)
		case HandFullHouse:
			solveFullHouse(h)
		case HandFlush:
			solveFlush(h)
		case HandStraight:
			solveStraight(h)
		case HandThreeOfAKind:
			solveThreeOfAKind(h)
		case HandTwoPair:
			solveTwoPair(h)
		case HandOnePair:
			solveOnePair(h)
		case HandHighCard:
			solveHighCard(h)
		}
		if h.IsPossible {
			return h
		}
	}
	return &Hand{Name: HandHighCard, Descr: "No Hand", Rules: rules, IsPossible: false}
}

func (h *Hand) resetWildCards() {
	for i := range h.Wilds {
		h.Wilds[i].Rank = -1
		h.Wilds[i].WildValue = h.Rules.WildValue
	}
}

func (h *Hand) nextHighest() []Card {
	used := map[string]bool{}
	for _, c := range h.Cards {
		used[c.ValueSuit()] = true
	}
	out := []Card{}
	for _, c := range h.cardPool {
		if !used[c.ValueSuit()] {
			out = append(out, c)
		}
	}
	return sortCards(out)
}

func solveHighCard(h *Hand) {
	h.resetWildCards()
	h.Cards = sortCards(h.cardPool)
	if len(h.Cards) > h.Rules.CardsInHand {
		h.Cards = h.Cards[:h.Rules.CardsInHand]
	}
	if len(h.Cards) > 0 {
		suit := h.Cards[0].Suit
		h.Descr = strings.TrimSuffix(h.Cards[0].String(), string(suit)) + " High"
	}
	h.IsPossible = true
}

func solveOnePair(h *Hand) {
	h.resetWildCards()
	for _, rank := range h.ValueOrder {
		if h.numCardsByRank(rank) == 2 {
			h.Cards = append([]Card{}, h.Values[rank]...)
			for i := range h.Wilds {
				if len(h.Cards) >= 2 {
					break
				}
				if len(h.Cards) > 0 {
					h.Wilds[i].Rank = h.Cards[0].Rank
				} else {
					h.Wilds[i].Rank = len(RankOrder) - 1
				}
				h.Wilds[i].WildValue = RankOrder[h.Wilds[i].Rank]
				h.Cards = append(h.Cards, h.Wilds[i])
			}
			more := h.nextHighest()
			need := h.Rules.CardsInHand - 2
			if need > len(more) {
				need = len(more)
			}
			h.Cards = append(h.Cards, more[:need]...)
			break
		}
	}
	if len(h.Cards) >= 2 {
		suit := byte('?')
		if len(h.Cards) > 0 {
			suit = h.Cards[0].Suit
		}
		h.Descr = strings.TrimSuffix(h.Cards[0].String(), string(suit)) + "'s"
	}
	h.IsPossible = len(h.Cards) >= 2
}

func solveTwoPair(h *Hand) {
	h.resetWildCards()
	pairs := []int{}
	for _, rank := range h.ValueOrder {
		if h.numCardsByRank(rank) >= 2 {
			pairs = append(pairs, rank)
		}
	}
	if len(pairs) >= 2 {
		h.Cards = append([]Card{}, h.Values[pairs[0]]...)
		h.Cards = append(h.Cards, h.Values[pairs[1]]...)
		more := h.nextHighest()
		if len(more) > 0 {
			h.Cards = append(h.Cards, more[0])
		}
		topSuit := h.Cards[0].Suit
		botSuit := h.Cards[2].Suit
		h.Descr = strings.TrimSuffix(h.Cards[0].String(), string(topSuit)) + "'s & " +
			strings.TrimSuffix(h.Cards[2].String(), string(botSuit)) + "'s"
		h.IsPossible = true
	} else {
		h.IsPossible = false
	}
}

func solveThreeOfAKind(h *Hand) {
	h.resetWildCards()
	for _, rank := range h.ValueOrder {
		if h.numCardsByRank(rank) == 3 {
			h.Cards = append([]Card{}, h.Values[rank]...)
			for i := range h.Wilds {
				if len(h.Cards) >= 3 {
					break
				}
				if len(h.Cards) > 0 {
					h.Wilds[i].Rank = h.Cards[0].Rank
				} else {
					h.Wilds[i].Rank = len(RankOrder) - 1
				}
				h.Wilds[i].WildValue = RankOrder[h.Wilds[i].Rank]
				h.Cards = append(h.Cards, h.Wilds[i])
			}
			more := h.nextHighest()
			need := h.Rules.CardsInHand - 3
			if need > len(more) {
				need = len(more)
			}
			h.Cards = append(h.Cards, more[:need]...)
			break
		}
	}
	if len(h.Cards) >= 3 {
		suit := byte('?')
		if len(h.Cards) > 0 {
			suit = h.Cards[0].Suit
		}
		h.Descr = strings.TrimSuffix(h.Cards[0].String(), string(suit)) + "'s"
	}
	h.IsPossible = len(h.Cards) >= 3
}
