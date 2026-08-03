package poker

import (
	"fmt"
	"sync"
	"time"
)

type MemoryStorage struct {
	mu      sync.Mutex
	games   map[string]*Game
	players map[int64][]*Player
	config  map[string]string
	nextID  int64
}

func NewMemoryStorage() *MemoryStorage {
	return &MemoryStorage{
		games:   map[string]*Game{},
		players: map[int64][]*Player{},
		config:  map[string]string{},
	}
}

func (m *MemoryStorage) GetOrCreateGame(tableID string) (*Game, []*Player, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if g, ok := m.games[tableID]; ok {
		return g, append([]*Player{}, m.players[g.ID]...), false, nil
	}
	m.nextID++
	g := NewGame(tableID)
	g.ID = m.nextID
	m.games[tableID] = g
	m.players[g.ID] = []*Player{}
	return g, []*Player{}, true, nil
}

func (m *MemoryStorage) SaveGame(g *Game, players []*Player) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if g.ID == 0 {
		m.nextID++
		g.ID = m.nextID
		m.games[g.TableID] = g
	}
	for _, p := range players {
		if p.GameID == 0 {
			p.GameID = g.ID
		}
	}
	m.players[g.ID] = append([]*Player{}, players...)
	g.UpdatedAt = time.Now()
	if g.CreatedAt.IsZero() {
		g.CreatedAt = g.UpdatedAt
	}
	return nil
}

func (m *MemoryStorage) LoadGame(tableID string) (*Game, []*Player, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	g, ok := m.games[tableID]
	if !ok {
		return nil, nil, fmt.Errorf("game %q not found", tableID)
	}
	return g, append([]*Player{}, m.players[g.ID]...), nil
}

func (m *MemoryStorage) GetConfig(key string) (string, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	val, ok := m.config[key]
	return val, ok, nil
}

func (m *MemoryStorage) SetConfig(key, value string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.config[key] = value
	return nil
}
