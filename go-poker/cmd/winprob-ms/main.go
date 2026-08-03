package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/zuse/poker5/go-poker"
)

type evaluateRequest struct {
	Hole         []string `json:"hole"`
	Board        []string `json:"board"`
	NumOpponents int      `json:"num_opponents"`
	Iterations   int      `json:"iterations"`
}

type evaluateResponse struct {
	WinProbability float64 `json:"win_probability"`
}

func main() {
	addr := flag.String("addr", ":18081", "HTTP listen address")
	maxIter := flag.Int("max-iter", 5000, "Hard cap on iterations per request")
	flag.Parse()

	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"ok":      true,
			"service": "winprob-ms",
			"time":    time.Now().UTC().Format(time.RFC3339),
		})
	})
	mux.HandleFunc("/evaluate", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "POST required", http.StatusMethodNotAllowed)
			return
		}
		var req evaluateRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid json", http.StatusBadRequest)
			return
		}
		if len(req.Hole) != 2 {
			http.Error(w, "hole must contain 2 cards", http.StatusBadRequest)
			return
		}
		if req.NumOpponents <= 0 {
			http.Error(w, "num_opponents must be > 0", http.StatusBadRequest)
			return
		}
		if req.Iterations <= 0 {
			req.Iterations = 1000
		}
		if req.Iterations > *maxIter {
			req.Iterations = *maxIter
		}
		if len(req.Board) > 5 {
			http.Error(w, "board must contain <= 5 cards", http.StatusBadRequest)
			return
		}
		wp := poker.EvaluateWinProbability(req.Hole, req.Board, req.NumOpponents, req.Iterations)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(evaluateResponse{WinProbability: wp})
	})

	log.Printf("winprob-ms listening on %s", *addr)
	log.Printf("http://localhost:18081")
	srv := &http.Server{
		Addr:         *addr,
		Handler:      mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 30 * time.Second,
	}
	if err := srv.ListenAndServe(); err != nil {
		fmt.Fprintf(log.Writer(), "winprob-ms: %v\n", err)
	}
}
