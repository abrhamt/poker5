package services

import (
	"encoding/json"
	"fmt"
	"sync"
)

type SSEMessage struct {
	Event string `json:"event"`
	Data  string `json:"data"`
}

type SSEHub struct {
	mu    sync.RWMutex
	rooms map[string]map[chan SSEMessage]bool
}

func NewSSEHub() *SSEHub {
	return &SSEHub{
		rooms: make(map[string]map[chan SSEMessage]bool),
	}
}

func (h *SSEHub) Subscribe(tableID string) (chan SSEMessage, func()) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.rooms[tableID] == nil {
		h.rooms[tableID] = make(map[chan SSEMessage]bool)
	}

	ch := make(chan SSEMessage, 16)
	h.rooms[tableID][ch] = true

	unsubscribe := func() {
		h.mu.Lock()
		defer h.mu.Unlock()
		if clients, ok := h.rooms[tableID]; ok {
			delete(clients, ch)
			close(ch)
			if len(clients) == 0 {
				delete(h.rooms, tableID)
			}
		}
	}

	return ch, unsubscribe
}

func (h *SSEHub) Broadcast(tableID string, event string, payload interface{}) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	clients, ok := h.rooms[tableID]
	if !ok || len(clients) == 0 {
		return
	}

	dataBytes, err := json.Marshal(payload)
	if err != nil {
		return
	}

	msg := SSEMessage{
		Event: event,
		Data:  string(dataBytes),
	}

	for ch := range clients {
		select {
		case ch <- msg:
		default:
		}
	}
}

func FormatSSEPayload(msg SSEMessage) string {
	if msg.Event != "" {
		return fmt.Sprintf("event: %s\ndata: %s\n\n", msg.Event, msg.Data)
	}
	return fmt.Sprintf("data: %s\n\n", msg.Data)
}
