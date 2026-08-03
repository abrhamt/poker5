package poker

import (
	"sort"
	"strings"
)

func solveStraightFlush(h *Hand) {
	h.resetWildCards()
	for suit := range h.Suits {
		flushCards := h.cardsForFlush(suit, false)
		if len(flushCards) < h.Rules.SFQualify {
			continue
		}
		possibleStraight := []Card{}
		for _, c := range flushCards {
			if c.Rank == -1 {
				possibleStraight = append(possibleStraight, c)
			} else {
				possibleStraight = append(possibleStraight, c)
			}
		}
		st2 := &Hand{
			Name:             HandStraight,
			Rules:            h.Rules,
			handValuesLookup: h.handValuesLookup,
			Suits:            map[byte][]Card{},
			Values:           map[int][]Card{},
			Wilds:            []Card{},
		}
		st2.cardPool = sortCards(possibleStraight)
		for _, c := range st2.cardPool {
			if c.Rank == -1 {
				st2.Wilds = append(st2.Wilds, c)
			} else {
				st2.Suits[c.Suit] = append(st2.Suits[c.Suit], c)
				st2.Values[c.Rank] = append(st2.Values[c.Rank], c)
			}
		}
		solveStraight(st2)
		if st2.IsPossible {
			h.Cards = append([]Card{}, st2.Cards...)
			h.SFLength = st2.SFLength
		}
	}
	if len(h.Cards) > 0 && h.Cards[0].Rank == 13 {
		h.Descr = HandRoyalFlush
		h.Name = HandRoyalFlush
	} else if len(h.Cards) >= h.Rules.SFQualify {
		suit := byte('?')
		if len(h.Cards) > 0 {
			suit = h.Cards[0].Suit
		}
		descr := strings.TrimSuffix(h.Cards[0].String(), string(suit)) + string(suit) + " High"
		h.Descr = descr
		h.Name = HandStraightFlush
	}
	h.IsPossible = len(h.Cards) >= h.Rules.SFQualify
}

func solveFourOfAKind(h *Hand) {
	h.resetWildCards()
	for _, rank := range h.ValueOrder {
		if h.numCardsByRank(rank) == 4 {
			base := append([]Card{}, h.Values[rank]...)
			h.Cards = append(h.Cards, base...)
			for i := range h.Wilds {
				if len(h.Cards) >= 4 {
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
			need := h.Rules.CardsInHand - 4
			if need > len(more) {
				need = len(more)
			}
			h.Cards = append(h.Cards, more[:need]...)
			break
		}
	}
	if len(h.Cards) >= 4 {
		suit := byte('?')
		if len(h.Cards) > 0 {
			suit = h.Cards[0].Suit
		}
		h.Descr = strings.TrimSuffix(h.Cards[0].String(), string(suit)) + "'s"
		if h.Rules.NoKickers {
			h.Cards = h.Cards[:4]
		}
	}
	h.IsPossible = len(h.Cards) >= 4
}

func solveFullHouse(h *Hand) {
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
			break
		}
	}
	if len(h.Cards) == 3 {
		for _, rank := range h.ValueOrder {
			cards := h.Values[rank]
			if len(cards) > 0 && h.Cards[0].WildValue == cards[0].WildValue {
				continue
			}
			if h.numCardsByRank(rank) >= 2 {
				h.Cards = append(h.Cards, cards...)
				for i := range h.Wilds {
					if h.Wilds[i].Rank != -1 {
						continue
					}
					if len(cards) > 0 {
						h.Wilds[i].Rank = cards[0].Rank
					} else {
						h.Wilds[i].Rank = len(RankOrder) - 1
					}
					h.Wilds[i].WildValue = RankOrder[h.Wilds[i].Rank]
					h.Cards = append(h.Cards, h.Wilds[i])
				}
				more := h.nextHighest()
				need := h.Rules.CardsInHand - 5
				if need > len(more) {
					need = len(more)
				}
				h.Cards = append(h.Cards, more[:need]...)
				break
			}
		}
	}
	if len(h.Cards) >= 5 {
		top := byte('?')
		bot := byte('?')
		if len(h.Cards) > 0 {
			top = h.Cards[0].Suit
		}
		if len(h.Cards) > 3 {
			bot = h.Cards[3].Suit
		}
		h.Descr = strings.TrimSuffix(h.Cards[0].String(), string(top)) + "'s over " +
			strings.TrimSuffix(h.Cards[3].String(), string(bot)) + "'s"
	}
	h.IsPossible = len(h.Cards) >= 5
}

func solveFlush(h *Hand) {
	h.SFLength = 0
	h.resetWildCards()
	for suit := range h.Suits {
		cards := h.cardsForFlush(suit, true)
		if len(cards) >= h.Rules.SFQualify {
			h.Cards = cards
			break
		}
	}
	if len(h.Cards) >= h.Rules.SFQualify {
		suit := byte('?')
		if len(h.Cards) > 0 {
			suit = h.Cards[0].Suit
		}
		h.Descr = strings.TrimSuffix(h.Cards[0].String(), string(suit)) + string(suit) + " High"
		h.SFLength = len(h.Cards)
		if len(h.Cards) < h.Rules.CardsInHand {
			more := h.nextHighest()
			need := h.Rules.CardsInHand - len(h.Cards)
			if need > len(more) {
				need = len(more)
			}
			h.Cards = append(h.Cards, more[:need]...)
		}
	}
	h.IsPossible = len(h.Cards) >= h.Rules.SFQualify
}

func solveStraight(h *Hand) {
	h.resetWildCards()
	ranks := []int{}
	seen := map[int]bool{}
	for _, c := range h.cardPool {
		if c.Rank >= 0 && !seen[c.Rank] {
			seen[c.Rank] = true
			ranks = append(ranks, c.Rank)
		}
	}
	sort.Sort(sort.Reverse(sort.IntSlice(ranks)))

	best := []Card{}
	for i := 0; i < len(ranks); i++ {
		seq := []Card{}
		curr := ranks[i]
		for _, c := range h.cardPool {
			if c.Rank == curr {
				seq = append(seq, c)
				break
			}
		}
		for j := 1; j < 5; j++ {
			target := curr - j
			found := false
			for _, c := range h.cardPool {
				if c.Rank == target {
					seq = append(seq, c)
					found = true
					break
				}
			}
			if !found {
				break
			}
		}
		if len(seq) == 5 {
			best = seq
			break
		}
	}

	if len(best) < 5 && seen[13] && seen[1] && seen[2] && seen[3] && seen[4] {
		wheelRanks := []int{4, 3, 2, 1, 13}
		seq := []Card{}
		for _, r := range wheelRanks {
			for _, c := range h.cardPool {
				if c.Rank == r {
					seq = append(seq, c)
					break
				}
			}
		}
		if len(seq) == 5 {
			best = seq
		}
	}

	if len(best) >= 5 {
		h.Cards = best
		suit := h.Cards[0].Suit
		h.Descr = strings.TrimSuffix(h.Cards[0].String(), string(suit)) + " High Straight"
		h.IsPossible = true
	} else {
		h.IsPossible = false
	}
}
