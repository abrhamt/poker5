package poker

import (
	"errors"
	"sort"
	"strings"
)

// RankOrder is the canonical rank ordering used by Card.Rank. It mirrors
// RANK_ORDER in poker/hand_evaluator.py.
//
// The 0-indexed position in the slice equals the rank value stored on the
// Card struct (so 'A' has rank 13, 'T' has rank 9, '2' has rank 1). The
// leading '1' entry (rank 0) exists for parity with the Python RANK_ORDER
// and is referenced by the wheel-straight logic.
var RankOrder = []byte{'1', '2', '3', '4', '5', '6', '7', '8', '9', 'T', 'J', 'Q', 'K', 'A'}

// rankIndex returns the rank index for a single rank character (case
// insensitive). It returns -1 for unknown ranks.
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

// Hand name constants. These mirror the Name field set on the Python Hand
// subclasses.
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

// Card is a single playing card. Code is the two-character encoding used
// throughout the codebase, e.g. "As", "Td", "2c". The first character is the
// rank, the second is the suit (c/d/h/s).
type Card struct {
	Value     byte // canonical rank character, e.g. 'A', 'T'
	Suit      byte // lowercase suit character
	Rank      int  // 0..12, mirrors RANK_ORDER.index(value)
	WildValue byte // rank character used when the card stands in as a wild
}

// ValueSuit returns the canonical two-character encoding for the card:
// uppercase rank + lowercase suit. Used by the showdown UI to render the
// 5 winning cards.
func (c Card) ValueSuit() string {
	value := c.Value
	if value == 0 {
		value = c.WildValue
	}
	return string([]byte{value, c.Suit})
}

// NewCard constructs a Card from the two-character code. It is the Go
// equivalent of Card.__init__ in hand_evaluator.py.
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

// String renders the card the same way Card.__repr__ does: tens become "10"
// and the suit is kept lowercase.
func (c Card) String() string {
	if c.Value == 'T' {
		return "10" + string(c.Suit)
	}
	return string(c.WildValue) + string(c.Suit)
}

// Equal mirrors Card.__eq__.
func (c Card) Equal(o Card) bool {
	return c.Rank == o.Rank && c.Suit == o.Suit
}

// cardSortKey returns a sort key for cards that mirrors Card.sort_key:
// descending by Rank so the highest card is first.
func cardSortKey(c Card) int { return -c.Rank }

// sortCards sorts a slice of Card by descending rank, matching the Python
// `card_pool.sort(key=Card.sort_key)` calls in hand_evaluator.py.
func sortCards(cards []Card) []Card {
	out := make([]Card, len(cards))
	copy(out, cards)
	sort.SliceStable(out, func(i, j int) bool { return cardSortKey(out[i]) < cardSortKey(out[j]) })
	return out
}

// ErrDuplicateCards is returned by Solve when the standard game rules
// encounter duplicate cards in the input (mirrors the ValueError raised in
// hand_evaluator.py).
var ErrDuplicateCards = errors.New("duplicate cards")

// GameRules captures the rule knobs used by the Python Game class. The
// Go port uses the standard ruleset only; the struct is kept for parity and
// future extension.
type GameRules struct {
	CardsInHand     int
	WildValue       byte
	WildStatus      int
	WheelStatus     int
	SFQualify       int
	LowestQualified []string
	NoKickers       bool
}

// StandardRules is the only ruleset shipped today; it matches
// GAME_RULES['standard'] from the Python code.
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

// Hand is the Go equivalent of the entire Hand class hierarchy in
// hand_evaluator.py. The class hierarchy is flattened into a single struct
// with a Name field, since Go has no inheritance.
//
// Solve picks the best matching hand type and populates Cards / Descr / SF
// length accordingly.
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
	ValueOrder       []int // ranks in descending order, used to iterate Values deterministically
	Wilds            []Card
	handValuesLookup map[string]int
}

// newHand constructs a Hand from raw card codes and a hand-name lookup
// table. This is the internal counterpart of the Python Hand.__init__.
//
// handValuesLookup maps the hand-type name to its rank number (9 = Royal
// Flush, 8 = Four of a Kind, …, 1 = High Card). The name->rank table is
// computed once per Solve call (see _handRankings).
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

	// Collect rank keys in descending order so iteration order is
	// deterministic (Python dicts preserve insertion order, Go maps do not).
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

// Compare returns the same signed ordering as the Python Hand.compare:
//   -1 when the receiver is stronger,
//    1 when the receiver is weaker,
//    0 on a true tie.
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

// LoseTo mirrors Hand.lose_to (returns true when self loses to other).
func (h *Hand) LoseTo(o *Hand) bool { return h.Compare(o) > 0 }

// QualifiesHigh mirrors Hand.qualifies_high: when no lowest-qualified hand is
// configured (the standard ruleset) every hand qualifies.
func (h *Hand) QualifiesHigh() bool {
	if h.Rules.LowestQualified == nil || h.AlwaysQualifies {
		return true
	}
	q := Solve(h.Rules.LowestQualified, h.Rules)
	return h.Compare(q) <= 0
}

// numCardsByRank returns the number of cards in the pool that match the
// supplied rank value, including any wild substitutions (the wild_status==1
// rule from hand_evaluator.py).
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

// cardsForFlush mirrors Hand.get_cards_for_flush. If setRanks is true, wild
// cards take on descending ranks not already occupied by the suit.
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
		cards = sortCards(cards)
	}
	return cards
}

// resetWildCards sets every wild back to rank -1 / its original value.
func (h *Hand) resetWildCards() {
	for i := range h.Wilds {
		h.Wilds[i].Rank = -1
		h.Wilds[i].WildValue = h.Wilds[i].Value
	}
}

// nextHighest returns the cards in the pool that aren't already in the
// chosen hand, sorted by rank descending. It mirrors Hand.next_highest.
func (h *Hand) nextHighest() []Card {
	excluded := map[Card]bool{}
	for _, c := range h.Cards {
		excluded[c] = true
	}
	picks := []Card{}
	for _, c := range h.cardPool {
		if excluded[c] {
			continue
		}
		if h.Rules.WildStatus == 0 && c.Rank == -1 {
			c.WildValue = 'A'
			c.Rank = len(RankOrder) - 1
		}
		picks = append(picks, c)
	}
	return sortCards(picks)
}

// stripWilds is the Go port of Hand.strip_wilds. It returns the wild and
// non-wild cards separately, with each non-wild freshly constructed (since
// the Python implementation wraps str codes into Card objects here).
func stripWilds(cards []Card, rules GameRules) ([]Card, []Card) {
	wilds := []Card{}
	non := []Card{}
	for _, c := range cards {
		if c.Rank == -1 {
			wilds = append(wilds, c)
		} else {
			non = append(non, c)
		}
	}
	return wilds, non
}

// _handRankings returns the canonical name -> rank table used by Solve.
//
// The ranking mirrors _hand_rankings in hand_evaluator.py. Royal Flush and
// Straight Flush share rank 9 and are resolved through the hand name.
func _handRankings() map[string]int {
	return map[string]int{
		HandStraightFlush: 9,
		HandRoyalFlush:    9,
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

// Solve is the Go equivalent of Hand.solve. It walks the standard rankings
// and returns the first hand type that matches.
//
// When the rules argument is nil the standard ruleset is used.
func Solve(cards []string, rules GameRules) *Hand {
	if len(cards) == 0 {
		cards = []string{""}
	}
	if rules.CardsInHand == 0 {
		rules = StandardRules()
	}

	codes := make([]string, len(cards))
	copy(codes, cards)

	// Duplicate check, matches `set(cards) != len(cards)` for standard rules.
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
			solveStraightFlush(h, true)
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

// solveStraightFlush handles both Royal Flush and Straight Flush detection.
// The flag selects which of the two constructors is used.
func solveStraightFlush(h *Hand, _ bool) {
	h.resetWildCards()
	possibleStraight := []Card{}
	nonCards := []Card{}

	for suit := range h.Suits {
		cards := h.cardsForFlush(suit, false)
		if len(cards) >= h.Rules.SFQualify {
			possibleStraight = cards
			break
		}
	}
	if len(possibleStraight) > 0 {
		if h.Rules.WildValue != 0 {
			for suit := range h.Suits {
				if len(possibleStraight) > 0 && possibleStraight[0].Suit != suit {
					nonCards = append(nonCards, h.Suits[suit]...)
					_, nonWilds := stripWilds(nonCards, h.Rules)
					nonCards = nonWilds
				}
			}
		}
		// Re-run Solve for straight on this subset, but reuse solveStraight
		// to avoid recursion.
		st2 := &Hand{
			Name:   HandStraight,
			Rules:  h.Rules,
			Cards:  []Card{},
			Values: map[int][]Card{},
			Suits:  map[byte][]Card{},
			Wilds:  []Card{},
		}
		// Populate the subset hand from `possibleStraight`.
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
			h.Cards = append(h.Cards, nonCards...)
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

func cardCodes(cards []Card) []string {
	out := make([]string, len(cards))
	for i, c := range cards {
		out[i] = c.String()
	}
	return out
}

func solveFourOfAKind(h *Hand) {
	h.resetWildCards()
	for _, rank := range h.ValueOrder {
		if h.numCardsByRank(rank) == 4 {
			base := append([]Card{}, h.Values[rank]...)
			h.Cards = base[:0]
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
					} else if h.Cards[0].Rank == len(RankOrder)-1 && h.Rules.WildStatus == 1 {
						h.Wilds[i].Rank = len(RankOrder) - 2
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

	if h.Rules.WheelStatus == 1 {
		wheel := h.getWheel()
		if len(wheel) > 0 {
			wildCount := 0
			for _, c := range wheel {
				if c.Value == h.Rules.WildValue {
					wildCount++
				}
			}
			for _, c := range wheel {
				if c.Rank == 0 {
					c.Rank = rankIndex('A')
					c.WildValue = 'A'
					if c.Value == '1' {
						c.Value = 'A'
					}
				}
			}
			wheel = sortCards(wheel)
			for i := wildCount; i < len(h.Wilds); i++ {
				if len(wheel) >= h.Rules.CardsInHand {
					break
				}
				h.Wilds[i].Rank = rankIndex('A')
				h.Wilds[i].WildValue = 'A'
				wheel = append(wheel, h.Wilds[i])
			}
			h.Descr = "Straight, Wheel"
			h.SFLength = h.Rules.SFQualify
			if wheel[0].Value == 'A' {
				more := h.nextHighest()
				skip := 1
				end := h.Rules.CardsInHand - len(wheel) + 1
				if skip > len(more) {
					skip = len(more)
				}
				if end > len(more)-skip {
					end = len(more) - skip
				}
				if end < 0 {
					end = 0
				}
				wheel = append(wheel, more[skip:skip+end]...)
			} else {
				more := h.nextHighest()
				end := h.Rules.CardsInHand - len(wheel)
				if end > len(more) {
					end = len(more)
				}
				if end < 0 {
					end = 0
				}
				wheel = append(wheel, more[:end]...)
			}
			h.Cards = wheel
			h.SFLength = h.Rules.SFQualify
			h.IsPossible = true
			return
		}
		h.resetWildCards()
	}

	h.Cards = h.getGaps(h.Rules.SFQualify, len(RankOrder))
	for i := range h.Wilds {
		check := h.getGaps(len(h.Cards), h.Cards[0].Rank+1)
		if len(h.Cards) == len(check) {
			if h.Cards[0].Rank < len(RankOrder)-1 {
				h.Wilds[i].Rank = h.Cards[0].Rank + 1
			} else {
				h.Wilds[i].Rank = h.Cards[len(h.Cards)-1].Rank - 1
			}
			h.Wilds[i].WildValue = RankOrder[h.Wilds[i].Rank]
			h.Cards = append(h.Cards, h.Wilds[i])
		} else {
			for j := 1; j < len(h.Cards); j++ {
				if h.Cards[j-1].Rank-h.Cards[j].Rank > 1 {
					h.Wilds[i].Rank = h.Cards[j-1].Rank - 1
					h.Wilds[i].WildValue = RankOrder[h.Wilds[i].Rank]
					h.Cards = append(h.Cards, h.Wilds[i])
					break
				}
			}
		}
		h.Cards = sortCards(h.Cards)
	}

	if len(h.Cards) >= h.Rules.SFQualify {
		suit := byte('?')
		if len(h.Cards) > 0 {
			suit = h.Cards[0].Suit
		}
		h.Descr = strings.TrimSuffix(h.Cards[0].String(), string(suit)) + " High"
		end := h.Rules.CardsInHand
		if end > len(h.Cards) {
			end = len(h.Cards)
		}
		h.Cards = h.Cards[:end]
		h.SFLength = len(h.Cards)
		if len(h.Cards) < h.Rules.CardsInHand {
			if h.Cards[h.SFLength-1].Rank == 0 {
				more := h.nextHighest()
				skip := 1
				end := h.Rules.CardsInHand - len(h.Cards) + 1
				if skip > len(more) {
					skip = len(more)
				}
				if end > len(more)-skip {
					end = len(more) - skip
				}
				if end < 0 {
					end = 0
				}
				h.Cards = append(h.Cards, more[skip:skip+end]...)
			} else {
				more := h.nextHighest()
				end := h.Rules.CardsInHand - len(h.Cards)
				if end > len(more) {
					end = len(more)
				}
				if end < 0 {
					end = 0
				}
				h.Cards = append(h.Cards, more[:end]...)
			}
		}
	}
	h.IsPossible = len(h.Cards) >= h.Rules.SFQualify
}

// getGaps mirrors Straight.get_gaps. It searches the card pool for a straight
// of `checkHandLength` cards finishing at or below rank index `i`.
func (h *Hand) getGaps(checkHandLength, i int) []Card {
	wildCards, cardsToCheck := stripWilds(h.cardPool, h.Rules)
	for _, c := range cardsToCheck {
		if c.WildValue == 'A' {
			nc := NewCard("1" + string(c.Suit))
			cardsToCheck = append(cardsToCheck, nc)
		}
	}
	cardsToCheck = sortCards(cardsToCheck)

	if checkHandLength == 0 || checkHandLength == h.Rules.SFQualify {
		checkHandLength = h.Rules.SFQualify
		i = len(RankOrder)
	}

	gapCards := []Card{}
	for i > 0 {
		cardsList := []Card{}
		gapCount := 0
		for _, card := range cardsToCheck {
			if card.Rank > i {
				continue
			}
			var diff int
			if len(cardsList) == 0 {
				diff = i - card.Rank
			} else {
				diff = cardsList[len(cardsList)-1].Rank - card.Rank
			}
			if checkHandLength < (gapCount + diff + len(cardsList)) {
				break
			} else if diff > 0 {
				cardsList = append(cardsList, card)
				gapCount += diff - 1
			}
		}
		if len(cardsList) > len(gapCards) {
			gapCards = append([]Card{}, cardsList...)
		}
		if h.Rules.SFQualify-len(gapCards) <= len(wildCards) {
			break
		}
		i--
	}
	return gapCards
}

// getWheel mirrors Straight.get_wheel: returns the 5-card A-2-3-4-5 wheel
// straight, with wilds used to fill in any missing ranks.
func (h *Hand) getWheel() []Card {
	wildCards, cardsToCheck := stripWilds(h.cardPool, h.Rules)
	for _, c := range cardsToCheck {
		if c.WildValue == 'A' {
			nc := NewCard("1" + string(c.Suit))
			cardsToCheck = append(cardsToCheck, nc)
		}
	}
	cardsToCheck = sortCards(cardsToCheck)

	wheelCards := []Card{}
	wildCount := 0
	for i := h.Rules.SFQualify - 1; i >= 0; i-- {
		found := false
		for _, c := range cardsToCheck {
			if c.Rank > i {
				continue
			}
			if c.Rank < i {
				break
			}
			wheelCards = append(wheelCards, c)
			found = true
			break
		}
		if !found {
			if wildCount < len(wildCards) {
				wildCards[wildCount].Rank = i
				wildCards[wildCount].WildValue = RankOrder[i]
				wheelCards = append(wheelCards, wildCards[wildCount])
				wildCount++
			} else {
				return []Card{}
			}
		}
	}
	return wheelCards
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
		if h.Rules.NoKickers {
			h.Cards = h.Cards[:3]
		}
	}
	h.IsPossible = len(h.Cards) >= 3
}

func solveTwoPair(h *Hand) {
	h.resetWildCards()
	pairsFound := 0
	for _, rank := range h.ValueOrder {
		cards := h.Values[rank]
		if pairsFound > 0 && h.numCardsByRank(rank) == 2 {
			h.Cards = append(h.Cards, cards...)
			for i := range h.Wilds {
				if h.Wilds[i].Rank != -1 {
					continue
				}
				if len(cards) > 0 {
					h.Wilds[i].Rank = cards[0].Rank
				} else if h.Cards[0].Rank == len(RankOrder)-1 && h.Rules.WildStatus == 1 {
					h.Wilds[i].Rank = len(RankOrder) - 2
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
		} else if h.numCardsByRank(rank) == 2 {
			h.Cards = append(h.Cards, cards...)
			for i := range h.Wilds {
				if h.Wilds[i].Rank != -1 {
					continue
				}
				if len(cards) > 0 {
					h.Wilds[i].Rank = cards[0].Rank
				} else if h.Cards[0].Rank == len(RankOrder)-1 && h.Rules.WildStatus == 1 {
					h.Wilds[i].Rank = len(RankOrder) - 2
				} else {
					h.Wilds[i].Rank = len(RankOrder) - 1
				}
				h.Wilds[i].WildValue = RankOrder[h.Wilds[i].Rank]
				h.Cards = append(h.Cards, h.Wilds[i])
			}
			pairsFound++
		}
	}
	if len(h.Cards) >= 4 {
		top := byte('?')
		bot := byte('?')
		if len(h.Cards) > 0 {
			top = h.Cards[0].Suit
		}
		if len(h.Cards) > 2 {
			bot = h.Cards[2].Suit
		}
		h.Descr = strings.TrimSuffix(h.Cards[0].String(), string(top)) + "'s & " +
			strings.TrimSuffix(h.Cards[2].String(), string(bot)) + "'s"
		if h.Rules.NoKickers {
			h.Cards = h.Cards[:4]
		}
	}
	h.IsPossible = len(h.Cards) >= 4
}

func solveOnePair(h *Hand) {
	h.resetWildCards()
	for _, rank := range h.ValueOrder {
		if h.numCardsByRank(rank) == 2 {
			h.Cards = append(h.Cards, h.Values[rank]...)
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
		if h.Rules.NoKickers {
			h.Cards = h.Cards[:2]
		}
	}
	h.IsPossible = len(h.Cards) >= 2
}

func solveHighCard(h *Hand) {
	h.Cards = append([]Card{}, h.cardPool[:min(h.Rules.CardsInHand, len(h.cardPool))]...)
	for i := range h.Cards {
		if h.Cards[i].Value == h.Rules.WildValue {
			h.Cards[i].WildValue = 'A'
			h.Cards[i].Rank = rankIndex('A')
		}
	}
	if h.Rules.NoKickers {
		h.Cards = h.Cards[:1]
	}
	h.Cards = sortCards(h.Cards)
	suit := byte('?')
	if len(h.Cards) > 0 {
		suit = h.Cards[0].Suit
	}
	h.Descr = strings.TrimSuffix(h.Cards[0].String(), string(suit)) + " High"
	h.IsPossible = true
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// Winners mirrors Hand.winners. It filters out hands that don't qualify
// under the high-hand rule and returns every hand tied for the best rank.
func Winners(hands []*Hand) []*Hand {
	qualifying := []*Hand{}
	for _, h := range hands {
		if h.QualifiesHigh() {
			qualifying = append(qualifying, h)
		}
	}
	if len(qualifying) == 0 {
		return []*Hand{}
	}
	top := qualifying[0].Rank
	for _, h := range qualifying {
		if h.Rank > top {
			top = h.Rank
		}
	}
	tied := []*Hand{}
	for _, h := range qualifying {
		if h.Rank == top {
			tied = append(tied, h)
		}
	}
	result := []*Hand{}
	for _, h := range tied {
		loses := false
		for _, other := range tied {
			if h.LoseTo(other) {
				loses = true
				break
			}
		}
		if !loses {
			result = append(result, h)
		}
	}
	return result
}