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
        frontend/                  React + Vite single-page app (all player screens)
        handlers/                  Fiber v3 JSON API, SSE hub, SPA + admin serving
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

## Configuration

Read from the environment, or from a `.env` file next to the binary.

| Variable | Default | Purpose |
|---|---|---|
| `SESSION_COOKIE_SECURE` | `false` | Adds `Secure` to the session cookie. **Set to `true` in production**, once the site is behind HTTPS. It defaults off because a `Secure` cookie is dropped over plain HTTP, which would silently break sign-in on an HTTP deployment or when testing against a LAN IP from a phone. The server logs a warning on every start while it is off. |
| `ADMIN_USERNAME` | `admin` | Seeded admin account. |
| `ADMIN_PHONE` | `0900000000` | Seeded admin phone. |
| `ADMIN_PASSWORD` | _(unset)_ | Seeded admin password. Re-applied on **every** start, so the environment is the source of truth — changing it in the database has no effect. Admin seeding is skipped entirely when unset. |
| `POKER_FRONTEND_DIR` | `frontend/dist` | Where the built React bundle is served from. |

## Frontend

Every player-facing screen (login, register, lobby, wallet, table) is a React
app in `frontend/`, talking to the Go server over the JSON API under `/api` and
receiving live table updates over SSE at `/api/events`. Only the admin dashboard
is still server-rendered HTML.

Development — run both, and use the Vite URL. It proxies `/api` and `/static`
at the Go server, so cookies and the SSE stream behave as they do in production:

    go run ./cmd/poker-demo -mode=server -addr=:8080 -db=poker.db
    cd frontend && npm install && npm run dev     # http://localhost:5173

Production — build the bundle, then let the Go server serve it. Any unmatched
GET falls through to `index.html`, so client-side routes survive a refresh:

    cd frontend && npm install && npm run build   # writes frontend/dist
    go run ./cmd/poker-demo -mode=server -addr=:8080 -db=poker.db

`frontend/dist` is a build artifact and is not committed. If it is missing, the
page routes answer 503 with a message saying how to build it. Set
`POKER_FRONTEND_DIR` to serve the bundle from somewhere else.

## Deploying

The Go binary is a JSON + SSE API and serves no files; Caddy serves the built
frontend from `/var/www/html/goldenpoker`. Step-by-step guide
in [docs/deployment.md](docs/deployment.md), with the config in
[deploy/](deploy/).

## Testing by hand

The dev server binds every interface, so a phone on the same network can reach
it at `http://<your-lan-ip>:5173` — which is where the mobile-first screens
should actually be checked. Keep `SESSION_COOKIE_SECURE` off for that: a LAN IP
is not a secure origin, and the browser would silently drop the session cookie.

Poker needs several players, so seed them instead of registering by hand:

    ./scripts/seed-dev.sh            # 4 funded players: dev1..dev4 / password123
    ./scripts/seed-dev.sh -n 6 -s 10 # 6 players, seated at a 10/20 table

**One browser profile is one player.** The session is a single cookie, so two
tabs in the same profile are the same person. Use a private window, a second
browser, or a phone for each additional seat.

Notes for driving a hand:

- A table deals on its own once two players are seated and the countdown ends.
  On a private table the host also gets a **Start Now** button.
- Skipping a turn is not neutral: the turn timer checks when it is free and
  folds when it is not, so an idle window will fold its own hand.
- The table forces landscape. On a desktop browser, use the device toolbar or a
  wide, short window — in portrait you get the rotate prompt, which is correct.
- The admin dashboard at `/admin` needs `ADMIN_PASSWORD` set before start-up,
  and admin accounts deliberately cannot take a seat.

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