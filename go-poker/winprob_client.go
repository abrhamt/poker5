package poker

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"time"
)

// WinProbClient is the client side of the win-probability microservice.
//
// It is goroutine-safe (the http.Client is) and includes a small in-memory
// cache keyed by hole+board+opponents to avoid hammering the microservice
// with identical requests (the engine re-queries state many times per
// second via /api/state/).
type WinProbClient struct {
	BaseURL    string
	HTTPClient *http.Client
	Iterations int
	Cache      *wpCache
}

// NewWinProbClient returns a client targeting the supplied microservice
// base URL (e.g. http://localhost:18081). A zero value for `iterations`
// defaults to 1000.
func NewWinProbClient(baseURL string, iterations int) *WinProbClient {
	if iterations <= 0 {
		iterations = 1000
	}
	return &WinProbClient{
		BaseURL:    baseURL,
		HTTPClient: &http.Client{Timeout: 10 * time.Second},
		Iterations: iterations,
		Cache:      newWPCache(),
	}
}

type wpRequest struct {
	Hole         []string `json:"hole"`
	Board        []string `json:"board"`
	NumOpponents int      `json:"num_opponents"`
	Iterations   int      `json:"iterations"`
}

type wpResponse struct {
	WinProbability float64 `json:"win_probability"`
}

// Evaluate sends a single Monte Carlo request to the microservice and
// returns the win probability. Errors fall back to 0 so the calling code
// can stay simple; the underlying error is logged for debugging.
func (c *WinProbClient) Evaluate(hole, board []string, numOpponents int) (float64, error) {
	if c == nil || c.BaseURL == "" {
		return 0, errors.New("winprob client not configured")
	}
	key := wpCacheKey(hole, board, numOpponents, c.Iterations)
	if v, ok := c.Cache.Get(key); ok {
		return v, nil
	}
	req := wpRequest{
		Hole:         hole,
		Board:        board,
		NumOpponents: numOpponents,
		Iterations:   c.Iterations,
	}
	body, err := json.Marshal(req)
	if err != nil {
		return 0, err
	}
	resp, err := c.HTTPClient.Post(c.BaseURL+"/evaluate", "application/json", bytes.NewReader(body))
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("winprob: status %d", resp.StatusCode)
	}
	var out wpResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return 0, err
	}
	c.Cache.Put(key, out.WinProbability)
	return out.WinProbability, nil
}

// wpCache is a tiny LRU-ish cache for repeated win-probability queries.
type wpCache struct {
	mu      sync.Mutex
	entries map[string]wpCacheEntry
	max     int
}

type wpCacheEntry struct {
	value     float64
	expiresAt time.Time
}

func newWPCache() *wpCache {
	return &wpCache{entries: map[string]wpCacheEntry{}, max: 256}
}

func (c *wpCache) Get(key string) (float64, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.entries[key]
	if !ok || time.Now().After(e.expiresAt) {
		return 0, false
	}
	return e.value, true
}

func (c *wpCache) Put(key string, value float64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries[key] = wpCacheEntry{value: value, expiresAt: time.Now().Add(30 * time.Second)}
	if len(c.entries) > c.max {
		// Naive eviction: drop the first expired/oldest entry.
		for k := range c.entries {
			if time.Now().After(c.entries[k].expiresAt) || k == key {
				delete(c.entries, k)
				break
			}
		}
	}
}

func wpCacheKey(hole, board []string, numOpponents, iterations int) string {
	var buf bytes.Buffer
	for _, h := range hole {
		buf.WriteString(h)
		buf.WriteByte(',')
	}
	buf.WriteByte('|')
	for _, b := range board {
		buf.WriteString(b)
		buf.WriteByte(',')
	}
	fmt.Fprintf(&buf, "|%d|%d", numOpponents, iterations)
	return buf.String()
}