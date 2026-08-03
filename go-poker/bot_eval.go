package poker

import (
	"strings"
)

type HandContext struct {
	IsTopPair  bool
	IsOverPair bool
}

func analyzeHandContext(hole, board []string) HandContext {
	cards := append([]string{}, hole...)
	cards = append(cards, board...)
	hand := Solve(cards, StandardRules())
	boardRanks := []int{}
	for _, c := range board {
		boardRanks = append(boardRanks, NewCard(c).Rank)
	}
	highest := 0
	for _, r := range boardRanks {
		if r > highest {
			highest = r
		}
	}
	pp := isPocketPair(hole)
	isTop := false
	isOver := false
	if hand != nil && hand.Name == HandOnePair && len(hand.Cards) > 0 {
		pairRank := hand.Cards[0].Rank
		isTop = pairRank == highest
		isOver = pp && pairRank > highest
	}
	return HandContext{IsTopPair: isTop, IsOverPair: isOver}
}

type DrawPotential struct {
	FlushDraw    bool
	StraightDraw bool
	Outs         int
}

func analyzeDrawPotential(hole, board []string) DrawPotential {
	all := append([]string{}, hole...)
	all = append(all, board...)
	suits := map[byte]int{}
	for _, c := range all {
		if len(c) < 2 {
			continue
		}
		suits[c[1]]++
	}
	maxSuit := 0
	hasFlush := false
	for _, n := range suits {
		if n > maxSuit {
			maxSuit = n
		}
		if n >= 5 {
			hasFlush = true
		}
	}
	flushDraw := !hasFlush && maxSuit == 4
	flushOuts := 0
	if flushDraw {
		flushOuts = 9
	}

	ranks := []int{}
	for _, c := range all {
		ranks = append(ranks, NewCard(c).Rank)
	}
	hasAce := false
	for _, r := range ranks {
		if r == 13 {
			hasAce = true
		}
	}
	if hasAce {
		ranks = append(ranks, 0)
	}
	unique := []int{}
	seen := map[int]bool{}
	for _, r := range ranks {
		if !seen[r] {
			seen[r] = true
			unique = append(unique, r)
		}
	}
	sortInts(unique)

	hasStraight := false
	missingRanks := map[int]bool{}
	straightOuts := 0
	for start := 1; start <= 10; start++ {
		missing := []int{}
		for r := start; r < start+5; r++ {
			if !seen[r] {
				missing = append(missing, r)
			}
		}
		if len(missing) == 0 {
			hasStraight = true
			break
		}
		if len(missing) == 1 {
			missingRanks[missing[0]] = true
			if missing[0] == start || missing[0] == start+4 {
				straightOuts = 8
			}
		}
	}
	straightDraw := len(missingRanks) > 0
	if hasStraight {
		straightDraw = false
		straightOuts = 0
	} else if straightDraw && straightOuts == 0 {
		if len(missingRanks) >= 2 {
			straightOuts = 8
		} else {
			straightOuts = 4
		}
	}
	return DrawPotential{FlushDraw: flushDraw, StraightDraw: straightDraw, Outs: flushOuts + straightOuts}
}

func sortInts(a []int) {
	for i := 1; i < len(a); i++ {
		for j := i; j > 0 && a[j-1] > a[j]; j-- {
			a[j-1], a[j] = a[j], a[j-1]
		}
	}
}

var rankMap = map[byte]int{
	'2': 2, '3': 3, '4': 4, '5': 5, '6': 6, '7': 7, '8': 8,
	'9': 9, 'T': 10, 'J': 11, 'Q': 12, 'K': 13, 'A': 14,
}

func evaluateBoardTexture(board []string) float64 {
	if len(board) < 3 {
		return 0
	}
	ranks := []int{}
	rankCounts := map[int]int{}
	suitCounts := map[byte]int{}
	for _, c := range board {
		if len(c) < 2 {
			continue
		}
		r := rankMap[c[0]]
		s := c[1]
		ranks = append(ranks, r)
		rankCounts[r]++
		suitCounts[s]++
	}
	maxRankCount := 0
	for _, n := range rankCounts {
		if n > maxRankCount {
			maxRankCount = n
		}
	}
	pairRisk := 0.0
	if maxRankCount > 1 {
		pairRisk = float64(maxRankCount-1) / float64(len(board)-1)
	}
	maxSuitCount := 0
	for _, n := range suitCounts {
		if n > maxSuitCount {
			maxSuitCount = n
		}
	}
	suitRisk := float64(maxSuitCount-1) / float64(len(board)-1)
	rfStraight := append([]int{}, ranks...)
	if containsInt(rfStraight, 14) {
		rfStraight = append(rfStraight, 1)
	}
	unique := []int{}
	seen := map[int]bool{}
	for _, r := range rfStraight {
		if !seen[r] {
			seen[r] = true
			unique = append(unique, r)
		}
	}
	sortInts(unique)
	maxConsecutive := 1
	currentRun := 1
	for i := 1; i < len(unique); i++ {
		if unique[i] == unique[i-1]+1 {
			currentRun++
		} else {
			currentRun = 1
		}
		if currentRun > maxConsecutive {
			maxConsecutive = currentRun
		}
	}
	connectedness := 0.0
	if maxConsecutive >= 3 {
		connectedness = float64(maxConsecutive-2) / float64(len(board)-2)
		if connectedness < 0 {
			connectedness = 0
		}
	}
	textureRisk := (connectedness + suitRisk + pairRisk) / 3
	if textureRisk < 0 {
		textureRisk = 0
	}
	if textureRisk > 1 {
		textureRisk = 1
	}
	return textureRisk
}

func containsInt(a []int, v int) bool {
	for _, x := range a {
		if x == v {
			return true
		}
	}
	return false
}

func preflopHandScore(cardA, cardB string) float64 {
	order := "23456789TJQKA"
	base := map[byte]float64{
		'A': 10, 'K': 8, 'Q': 7, 'J': 6, 'T': 5,
		'9': 4.5, '8': 4, '7': 3.5, '6': 3, '5': 2.5,
		'4': 2, '3': 1.5, '2': 1,
	}
	if len(cardA) < 2 || len(cardB) < 2 {
		return 0
	}
	r1, r2 := cardA[0], cardB[0]
	s1, s2 := cardA[1], cardB[1]
	i1 := strings.IndexByte(order, string(r1)[0])
	i2 := strings.IndexByte(order, string(r2)[0])
	if i1 < 0 || i2 < 0 {
		return 0
	}
	if i1 < i2 {
		r1, r2 = r2, r1
		s1, s2 = s2, s1
		i1, i2 = i2, i1
	}
	score := base[r1]
	if r1 == r2 {
		score *= 2
		if score < 5 {
			score = 5
		}
	}
	if s1 == s2 {
		score += 2
	}
	gap := i1 - i2 - 1
	switch {
	case gap == 1:
		score -= 1
	case gap == 2:
		score -= 2
	case gap == 3:
		score -= 4
	case gap >= 4:
		score -= 5
	}
	if gap <= 1 && i1 < strings.IndexByte(order, 'Q') {
		score += 1
	}
	if score < 0 {
		score = 0
	}
	if score > 10 {
		score = 10
	}
	return score
}
