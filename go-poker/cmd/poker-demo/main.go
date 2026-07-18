// Command poker-demo runs the Go port of the poker5 Django app.
//
// Two modes are supported:
//
//	poker-demo -mode=server  [-addr=:8080] [-db=poker.db] [-static=... -templates=...]
//	poker-demo -mode=demo    [-db=]
//
// "server" boots the HTTP layer (matching the Django views/urls); "demo"
// spins up three bots + one human player, plays one hand and prints the
// resulting state.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/zuse/poker5/go-poker"
)

func main() {
	mode := flag.String("mode", "demo", "demo or server")
	addr := flag.String("addr", ":8080", "HTTP listen address (server mode)")
	dbPath := flag.String("db", "poker.db", "SQLite database path (server mode)")
	staticDir := flag.String("static", "static", "static assets directory")
	templatesDir := flag.String("templates", "templates", "html template directory")
	winProbAddr := flag.String("winprob", "", "Win-probability microservice URL (e.g. http://localhost:18081). Empty disables it.")
	winProbIter := flag.Int("winprob-iter", 800, "Iterations per win-probability request")
	winProbMSAddr := flag.String("winprob-ms", "", "If set, run an embedded winprob-ms on this address (server mode only).")
	flag.Parse()

	switch *mode {
	case "server":
		runServer(*addr, *dbPath, *staticDir, *templatesDir, *winProbAddr, *winProbIter, *winProbMSAddr)
	case "demo":
		runDemo()
	default:
		fmt.Fprintf(os.Stderr, "unknown mode %q\n", *mode)
		os.Exit(2)
	}
}

func runServer(addr, dbPath, staticDir, templatesDir, winProbAddr string, winProbIter int, winProbMSAddr string) {
	if err := poker.EnsureDirs(staticDir, templatesDir); err != nil {
		log.Fatalf("ensure dirs: %v", err)
	}
	absDB, err := filepath.Abs(dbPath)
	if err != nil {
		log.Fatalf("abs db path: %v", err)
	}
	storage, err := poker.NewSQLiteStorage(absDB)
	if err != nil {
		log.Fatalf("open sqlite: %v", err)
	}
	defer storage.Close()

	state := poker.NewGameState(storage)
	server, err := poker.NewServer(state, staticDir, templatesDir)
	if err != nil {
		log.Fatalf("init server: %v", err)
	}

	// Optionally spawn the winprob-ms in-process.
	if winProbMSAddr != "" {
		go startEmbeddedWinProbMS(winProbMSAddr, winProbIter)
		time.Sleep(200 * time.Millisecond)
		if winProbAddr == "" {
			winProbAddr = "http://localhost" + winProbMSAddr
		}
	}

	if winProbAddr != "" {
		server.SetWinProbClient(winProbAddr, winProbIter)
		log.Printf("win-probability microservice: %s (iterations=%d)", winProbAddr, winProbIter)
	}

	log.Printf("poker server listening on %s (db=%s, static=%s, templates=%s)",
		addr, absDB, staticDir, templatesDir)
	if err := http.ListenAndServe(addr, server.Routes()); err != nil {
		log.Fatalf("serve: %v", err)
	}
}

// startEmbeddedWinProbMS launches the winprob-ms HTTP server in-process.
// It blocks the calling goroutine until the server exits.
func startEmbeddedWinProbMS(addr string, maxIter int) {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true,"service":"winprob-ms"}`))
	})
	mux.HandleFunc("/evaluate", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "POST required", http.StatusMethodNotAllowed)
			return
		}
		var req struct {
			Hole         []string `json:"hole"`
			Board        []string `json:"board"`
			NumOpponents int      `json:"num_opponents"`
			Iterations   int      `json:"iterations"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid json", http.StatusBadRequest)
			return
		}
		if len(req.Hole) != 2 || req.NumOpponents <= 0 || len(req.Board) > 5 {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		if req.Iterations <= 0 {
			req.Iterations = 1000
		}
		if req.Iterations > maxIter {
			req.Iterations = maxIter
		}
		wp := poker.EvaluateWinProbability(req.Hole, req.Board, req.NumOpponents, req.Iterations)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]float64{"win_probability": wp})
	})
	log.Printf("embedded winprob-ms listening on %s", addr)
	srv := &http.Server{Addr: addr, Handler: mux, ReadTimeout: 10 * time.Second, WriteTimeout: 30 * time.Second}
	if err := srv.ListenAndServe(); err != nil {
		log.Printf("winprob-ms exited: %v", err)
	}
}

func runDemo() {
	storage := poker.NewMemoryStorage()
	state := poker.NewGameState(storage)
	eng, _, err := state.GetOrCreate("demo")
	if err != nil {
		log.Fatalf("get or create: %v", err)
	}

	names := []string{"Alice", "Bot Bob", "Bot Carol", "Bot Dave"}
	eng.InitGame(names)
	eng.StartHand()
	fmt.Printf("=== Hand #%d (phase=%s) ===\n", eng.Game.TotalHands, eng.Game.Phase)

	// Step until the hand resolves.
	maxSteps := 200
	steps := 0
	for !eng.Game.GameFinished && eng.Game.GameStarted && steps < maxSteps {
		eng.AdvanceOneStep()
		steps++
		if eng.Game.Notifications != nil && len(eng.Game.Notifications) > 0 {
			last := eng.Game.Notifications[len(eng.Game.Notifications)-1]
			fmt.Printf("  step %d: %s\n", steps, last)
		}
		time.Sleep(5 * time.Millisecond)
	}
	fmt.Printf("steps taken: %d\n", steps)

	stateMap := eng.ToDict()
	out, _ := json.MarshalIndent(stateMap, "", "  ")
	fmt.Println(string(out))

	stats, _ := json.MarshalIndent(map[string]interface{}{
		"table_id":    eng.Game.TableID,
		"phase":       eng.Game.Phase,
		"pot":         eng.Game.Pot,
		"total_hands": eng.Game.TotalHands,
		"players":     playerSummary(eng.Players),
		"community":   eng.Game.CommunityCards,
		"version":     eng.Game.Version,
	}, "", "  ")
	fmt.Println(string(stats))
}

func playerSummary(players []*poker.Player) []map[string]interface{} {
	out := make([]map[string]interface{}, 0, len(players))
	for _, p := range players {
		out = append(out, map[string]interface{}{
			"name":      p.Name,
			"is_bot":    p.IsBot,
			"chips":     p.Chips,
			"folded":    p.Folded,
			"all_in":    p.AllIn,
			"cards":     p.Cards,
			"total_bet": p.TotalBet,
			"hands":     p.Stats.Hands,
		})
	}
	return out
}