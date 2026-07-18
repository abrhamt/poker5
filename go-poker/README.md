# go-poker

Direct port of the poker5 Django app to Go. The repository contains the full game
logic (hand evaluator, bot AI, game engine), a SQLite persistence layer, an HTTP
server exposing the same JSON API as the Django views, HTML templates, a CLI demo
binary, and tests mirroring `poker/test_game.py`.

## Layout

    go-poker/
        go.mod                     module + dependency declaration
        models.go                  plain-Go types that mirror models.py
        hand_evaluator.go          poker hand ranking/evaluation (port of hand_evaluator.py)
        bot.go                     bot AI (port of bot.py)
        engine.go                  game engine (port of engine.py)
        storage.go                 SQLite persistence (modernc.org/sqlite, pure Go)
        server.go                  HTTP server + JSON API + html/template rendering
        templates/                 HTML templates ported from Django
        static/                    JS / CSS / images copied from the Django app
        cmd/poker-demo/main.go     CLI binary that runs a sample game end-to-end
        *_test.go                  Go tests ported from poker/test_game.py

## Build

    cd go-poker
    go mod tidy
    go build ./...
    go test ./...

## Run the HTTP server

    go run ./cmd/poker-demo -mode=server -addr=:8080 -db=poker.db

## Run the CLI demo

    go run ./cmd/poker-demo -mode=demo

The demo spins up three bots + one human, plays one hand, prints the resulting
state and exits.

## Notes on the port

- The hand evaluator, bot, and engine are translated 1-to-1; Python dictionaries
  are replaced with Go structs, integer indices replace `next(...)` lookups,
  and JSON encoded strings (deck / community cards / stats) are kept as
  `[]string` slices and structs internally and only serialized at the storage
  boundary.
- `poker_project` (settings / wsgi / asgi / admin) is dropped; routing is
  handled by `net/http` in `server.go`.
- SQLite access uses `modernc.org/sqlite` so the binary stays a single static
  executable with no cgo dependency.