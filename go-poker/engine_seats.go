package poker

import (
	"fmt"
	"strings"
)

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
