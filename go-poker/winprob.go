package poker

import "math/rand"

// EvaluateWinProbability runs a Monte Carlo simulation of `iterations`
// showdowns given the player's hole cards, the visible community cards,
// and the number of opponents still in the hand. The returned value is a
// win probability between 0 and 100 (one decimal place).
//
// Ties award half a pot - i.e. a tie between the player and the strongest
// opponent counts as 0.5 wins (the other 0.5 is implicitly attributed to
// the opponent). This matches the convention used by most poker odds
// calculators.
//
// The RNG is seeded for repeatable test output; production callers can
// ignore this property.
func EvaluateWinProbability(hole, board []string, numOpponents, iterations int) float64 {
	if iterations <= 0 || numOpponents <= 0 || len(hole) != 2 {
		return 0
	}
	rng := rand.New(rand.NewSource(1))
	_ = rng // deterministic; the actual shuffling is done via ShuffleDeck which seeds the global rand

	known := append([]string{}, hole...)
	known = append(known, board...)

	var credit float64
	for i := 0; i < iterations; i++ {
		deck := ShuffleDeck(FullDeck)
		deck = removeCards(deck, known)

		// Deal hole cards to each opponent.
		type opp struct {
			cards []string
			hand  *Hand
		}
		opponents := make([]opp, 0, numOpponents)
		for o := 0; o < numOpponents; o++ {
			if len(deck) < 2 {
				break
			}
			c := []string{deck[0], deck[1]}
			deck = deck[2:]
			full := append([]string{}, c...)
			full = append(full, completeBoard(deck, board)...)
			h := Solve(full, StandardRules())
			if h != nil {
				opponents = append(opponents, opp{cards: c, hand: h})
			}
		}

		// Complete the player's board and solve.
		boardComplete := completeBoard(deck, board)
		playerFull := append([]string{}, hole...)
		playerFull = append(playerFull, boardComplete...)
		playerHand := Solve(playerFull, StandardRules())
		if playerHand == nil {
			continue
		}

		if len(opponents) == 0 {
			credit++
			continue
		}

		// Find the best opponent hand.
		bestOpp := opponents[0].hand
		for _, o := range opponents[1:] {
			if o.hand.Compare(bestOpp) < 0 {
				bestOpp = o.hand
			}
		}
		switch playerHand.Compare(bestOpp) {
		case -1:
			credit++
		case 0:
			// Award half a pot for a chop.
			credit += 0.5
		}
	}

	pct := credit / float64(iterations) * 100
	// One decimal place.
	return float64(int(pct*10+0.5)) / 10
}

// completeBoard returns the supplied `board` plus additional cards drawn
// from `deck` until 5 community cards are present. The deck is *not*
// modified; we walk it with an index so we only consume as many cards as
// we need.
func completeBoard(deck []string, board []string) []string {
	out := append([]string{}, board...)
	if len(out) >= 5 {
		return out
	}
	for i := 0; len(out) < 5 && i < len(deck); i++ {
		out = append(out, deck[i])
	}
	return out
}

// removeCards returns a copy of deck with the supplied cards removed (by
// string equality). Cards not present in the deck are silently ignored.
func removeCards(deck []string, cards []string) []string {
	remove := map[string]int{}
	for _, c := range cards {
		remove[c]++
	}
	out := make([]string, 0, len(deck))
	for _, c := range deck {
		if remove[c] > 0 {
			remove[c]--
			continue
		}
		out = append(out, c)
	}
	return out
}