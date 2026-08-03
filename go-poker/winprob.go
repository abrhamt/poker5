package poker

import "math/rand"

func EvaluateWinProbability(hole, board []string, numOpponents, iterations int) float64 {
	if iterations <= 0 || numOpponents <= 0 || len(hole) != 2 {
		return 0
	}
	rng := rand.New(rand.NewSource(1))
	_ = rng

	known := append([]string{}, hole...)
	known = append(known, board...)

	var credit float64
	for i := 0; i < iterations; i++ {
		deck := ShuffleDeck(FullDeck)
		deck = removeCards(deck, known)

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
			credit += 0.5
		}
	}

	pct := credit / float64(iterations) * 100
	return float64(int(pct*10+0.5)) / 10
}

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