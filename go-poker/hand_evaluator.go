package poker

import (
	"errors"
	"sort"
)

var RankOrder = []byte{'1', '2', '3', '4', '5', '6', '7', '8', '9', 'T', 'J', 'Q', 'K', 'A'}

func rankIndex(r byte) int {
	upper := r
	if upper >= 'a' && upper <= 'z' {
		upper -= 'a' - 'A'
	}
	for i, c := range RankOrder {
		if c == upper {
			return i
		}
	}
	return -1
}

const (
	HandHighCard      = "High Card"
	HandOnePair       = "Pair"
	HandTwoPair       = "Two Pair"
	HandThreeOfAKind  = "Three of a Kind"
	HandStraight      = "Straight"
	HandFlush         = "Flush"
	HandFullHouse     = "Full House"
	HandFourOfAKind   = "Four of a Kind"
	HandFiveOfAKind   = "Five of a Kind"
	HandStraightFlush = "Straight Flush"
	HandRoyalFlush    = "Royal Flush"
)

type Card struct {
	Value     byte
	Suit      byte
	Rank      int
	WildValue byte
}

func (c Card) ValueSuit() string {
	value := c.Value
	if value == 0 {
		value = c.WildValue
	}
	return string([]byte{value, c.Suit})
}

func NewCard(code string) Card {
	if len(code) < 2 {
		return Card{}
	}
	value := code[0]
	if value >= 'a' && value <= 'z' {
		value -= 'a' - 'A'
	}
	suit := code[1]
	if suit >= 'A' && suit <= 'Z' {
		suit += 'a' - 'A'
	}
	return Card{
		Value:     value,
		Suit:      suit,
		Rank:      rankIndex(value),
		WildValue: value,
	}
}

func (c Card) String() string {
	if c.Value == 'T' {
		return "10" + string(c.Suit)
	}
	return string(c.WildValue) + string(c.Suit)
}

func (c Card) Equal(o Card) bool {
	return c.Rank == o.Rank && c.Suit == o.Suit
}

func cardSortKey(c Card) int { return -c.Rank }

func sortCards(cards []Card) []Card {
	out := make([]Card, len(cards))
	copy(out, cards)
	sort.SliceStable(out, func(i, j int) bool { return cardSortKey(out[i]) < cardSortKey(out[j]) })
	return out
}

var ErrDuplicateCards = errors.New("duplicate cards")

type GameRules struct {
	CardsInHand     int
	WildValue       byte
	WildStatus      int
	WheelStatus     int
	SFQualify       int
	LowestQualified []string
	NoKickers       bool
}

func StandardRules() GameRules {
	return GameRules{
		CardsInHand: 5,
		WildValue:   0,
		WildStatus:  1,
		WheelStatus: 0,
		SFQualify:   5,
		NoKickers:   false,
	}
}

type Hand struct {
	Name             string
	Descr            string
	Cards            []Card
	Rank             int
	IsPossible       bool
	SFLength         int
	AlwaysQualifies  bool
	Rules            GameRules
	cardPool         []Card
	Suits            map[byte][]Card
	Values           map[int][]Card
	ValueOrder       []int
	Wilds            []Card
	handValuesLookup map[string]int
}

func newHand(codes []string, name string, rules GameRules, handRanks map[string]int) *Hand {
	h := &Hand{
		Name:             name,
		Rules:            rules,
		handValuesLookup: handRanks,
	}
	h.cardPool = make([]Card, 0, len(codes))
	for _, c := range codes {
		h.cardPool = append(h.cardPool, NewCard(c))
	}
	if rules.WildValue != 0 {
		for i := range h.cardPool {
			if h.cardPool[i].Value == rules.WildValue {
				h.cardPool[i].Rank = -1
			}
		}
	}
	h.cardPool = sortCards(h.cardPool)

	h.Suits = map[byte][]Card{}
	h.Values = map[int][]Card{}
	h.Wilds = []Card{}
	for _, c := range h.cardPool {
		if c.Rank == -1 {
			h.Wilds = append(h.Wilds, c)
			continue
		}
		h.Suits[c.Suit] = append(h.Suits[c.Suit], c)
		h.Values[c.Rank] = append(h.Values[c.Rank], c)
	}

	h.ValueOrder = make([]int, 0, len(h.Values))
	for k := range h.Values {
		h.ValueOrder = append(h.ValueOrder, k)
	}
	sort.Slice(h.ValueOrder, func(i, j int) bool { return h.ValueOrder[i] > h.ValueOrder[j] })

	if rank, ok := handRanks[name]; ok {
		h.Rank = rank
	} else {
		h.Rank = 0
	}
	h.AlwaysQualifies = true
	return h
}

func (h *Hand) Compare(o *Hand) int {
	if h.Rank < o.Rank {
		return 1
	}
	if h.Rank > o.Rank {
		return -1
	}
	for i := 0; i < 5 && i < len(h.Cards) && i < len(o.Cards); i++ {
		if h.Cards[i].Rank < o.Cards[i].Rank {
			return 1
		}
		if h.Cards[i].Rank > o.Cards[i].Rank {
			return -1
		}
	}
	return 0
}

func (h *Hand) LoseTo(o *Hand) bool { return h.Compare(o) > 0 }

func (h *Hand) QualifiesHigh() bool {
	if h.Rules.LowestQualified == nil || h.AlwaysQualifies {
		return true
	}
	q := Solve(h.Rules.LowestQualified, h.Rules)
	return h.Compare(q) <= 0
}

func (h *Hand) numCardsByRank(val int) int {
	cards := h.Values[val]
	count := len(cards)
	for i := range h.Wilds {
		if h.Wilds[i].Rank > -1 {
			continue
		}
		if len(cards) > 0 {
			if h.Rules.WildStatus == 1 || cards[0].Rank == len(RankOrder)-1 {
				count++
			}
		} else if h.Rules.WildStatus == 1 || val == len(RankOrder)-1 {
			count++
		}
	}
	return count
}

func (h *Hand) cardsForFlush(suit byte, setRanks bool) []Card {
	cards := sortCards(h.Suits[suit])
	for i := range h.Wilds {
		if setRanks {
			j := 0
			for j < len(RankOrder) && j < len(cards) {
				if cards[j].Rank == len(RankOrder)-1-j {
					j++
				} else {
					break
				}
			}
			h.Wilds[i].Rank = len(RankOrder) - 1 - j
			h.Wilds[i].WildValue = RankOrder[h.Wilds[i].Rank]
		}
		cards = append(cards, h.Wilds[i])
	}
	return sortCards(cards)
}

func cardCodes(cards []Card) []string {
	out := make([]string, len(cards))
	for i, c := range cards {
		out[i] = c.String()
	}
	return out
}

func Winners(hands []*Hand) []*Hand {
	if len(hands) == 0 {
		return nil
	}
	best := hands[0]
	for _, h := range hands[1:] {
		if h.Compare(best) < 0 {
			best = h
		}
	}
	winners := []*Hand{}
	for _, h := range hands {
		if h.Compare(best) == 0 {
			winners = append(winners, h)
		}
	}
	return winners
}