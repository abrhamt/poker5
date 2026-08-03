package main

import (
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	_ "github.com/go-sql-driver/mysql"
	_ "modernc.org/sqlite"

	"github.com/zuse/poker5/go-poker"
)

func main() {
	mode := flag.String("mode", "demo", "demo or server")
	addr := flag.String("addr", ":8080", "HTTP listen address")
	dbPath := flag.String("db", "poker.db", "Database path or DSN")
	flag.Parse()

	switch *mode {
	case "server":
		runServer(*addr, *dbPath)
	case "demo":
		runDemo()
	default:
		fmt.Fprintf(os.Stderr, "unknown mode %q\n", *mode)
		os.Exit(2)
	}
}

func runServer(addr, dbPath string) {
	absDB, err := filepath.Abs(dbPath)
	if err != nil {
		log.Fatalf("abs db path: %v", err)
	}
	db, err := sql.Open("sqlite", absDB)
	if err != nil {
		log.Fatalf("open db: %v", err)
	}
	defer db.Close()

	srv := poker.NewFiberServer(db)
	if err := srv.Listen(addr); err != nil {
		log.Fatalf("fiber listen error: %v", err)
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
}