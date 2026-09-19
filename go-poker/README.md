# go-poker

Direct port of the poker5 Django app to Go. The repository contains the full game
logic (hand evaluator, bot AI, game engine), a PostgreSQL persistence layer, an HTTP
server exposing the same JSON API as the Django views, HTML templates, a CLI demo
binary, and tests mirroring `poker/test_game.py`.

## Layout

    go-poker/
        go.mod                     module + dependency declaration
        models.go                  plain-Go types that mirror models.py
        hand_evaluator.go          poker hand ranking/evaluation (port of hand_evaluator.py)
        bot.go                     bot AI (port of bot.py)
        engine.go                  game engine (port of engine.py)
        storage.go                 live table state in PostgreSQL (pgx)
        db/schema.sql              the only schema definition: sqlc reads it, the server applies it
        db/queries/                sqlc query sources
        repository/                sqlc output + the connection pool
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

`go test ./...` needs a PostgreSQL to run against. It starts a throwaway
container itself, so Docker running is the only requirement; the tests skip
with an explanation if it is not. Point them at a server you already have with
`TEST_DATABASE_URL` instead — they create and drop a database per test, so give
them one they may do that on.

## Database

PostgreSQL only. `DATABASE_URL` is required and has no default: a fallback
connection string would let a misconfigured deployment come up pointed at the
wrong database instead of failing at start-up.

    DATABASE_URL=postgres://poker:secret@localhost:5432/poker?sslmode=disable

The schema lives in exactly one place, `db/schema.sql`: sqlc generates
`repository/` from it, and the server embeds and executes it at start-up, so
there is no second copy to drift. Columns added after a release also go in the
migrations list in `db/schema.go` — `CREATE TABLE IF NOT EXISTS` never alters a
table that already exists.

After editing anything under `db/`:

    sqlc generate

## Run the HTTP server

    DATABASE_URL=postgres://poker:poker@localhost:5432/poker?sslmode=disable \
        go run ./cmd/poker-demo -mode=server -addr=:8080

## Configuration

Read from the environment, or from a `.env` file next to the binary.

| Variable | Default | Purpose |
|---|---|---|
| `DATABASE_URL` | _(required)_ | PostgreSQL connection string. The server refuses to start without it. |
| `SESSION_COOKIE_SECURE` | `false` | Adds `Secure` to the session cookie. **Set to `true` in production**, once the site is behind HTTPS. It defaults off because a `Secure` cookie is dropped over plain HTTP, which would silently break sign-in on an HTTP deployment or when testing against a LAN IP from a phone. The server logs a warning on every start while it is off. |
| `ADMIN_USERNAME` | `admin` | Seeded admin account. |
| `ADMIN_PHONE` | `0900000000` | Seeded admin phone. |
| `ADMIN_PASSWORD` | _(unset)_ | Seeded admin password. Re-applied on **every** start, so the environment is the source of truth — changing it in the database has no effect. Admin seeding is skipped entirely when unset. |
| `POKER_FRONTEND_DIR` | `frontend/dist` | Where the built React bundle is served from. |
| `APP_ENV` | _(unset)_ | Set to `development` to print verification codes in full to the log, which is how signup and password reset are completed locally when `GEEZSMS_TOKEN` is unset. Anywhere else the console sender **refuses to send** and both flows return `502` — deliberately, so a deploy without a real provider fails visibly instead of telling users a code is on its way that does not exist. |
| `GEEZSMS_TOKEN` | _(unset)_ | API token for [GeezSMS](https://geezsms.com), which delivers the verification and password-reset codes. Setting it switches the OTP sender from console to real SMS; leaving it unset keeps the console sender and the `APP_ENV` behaviour above. |
| `GEEZSMS_URL` | `https://api.geezsms.com/api/v1/sms/send` | Provider endpoint. Only worth setting to point at a staging account or a local stub. |
| `RECEIPT_VERIFIER_URL` | `https://vericall.ethiodeploy.com/v1/receipts` | Service that resolves a CBE receipt link to the transfer behind it. Used only when real deposits are switched on — see [Deposits](#deposits). |
| `RECEIPT_VERIFIER_TIMEOUT` | `30s` | How long **one** call to the verifier may take. The first look at a new receipt is a cold fetch of CBE's own site and is the slow case every real deposit pays, so this is generous on purpose — a tight value does not fail fast, it queues receipts the verifier would have answered. |
| `RECEIPT_VERIFIER_ATTEMPTS` | `3` | Tries per verification, not retries on top of one. Only failures a retry can fix are retried; a receipt the bank has rejected is answered immediately. |
| `RECEIPT_VERIFIER_BACKOFF` | `1s` | Pause after the first failed attempt, growing with each one (1s, then 2s). |
| `RECEIPT_VERIFIER_USER_AGENT` | `golden-poker-deposits/1.0` | How this app names itself to the verifier. Worth changing only to tell two deployments apart in the verifier's logs. |
| `RECEIPT_VERIFIER_TOKEN` | _(unset)_ | Sent as a header on every verification. Not authentication — the verifier needs none — but something a CDN in front of it can match a "skip" rule on, so this server gets through on a shared secret rather than on its IP address. |
| `RECEIPT_VERIFIER_TOKEN_HEADER` | `X-Api-Key` | Header the token is sent in. |
| `DEPOSIT_SWEEP_INTERVAL` | `5m` | How often queued receipts are re-checked in the background. A Go duration (`90s`, `5m`). An unparseable value logs a warning and falls back rather than failing start-up. |

## Frontend

Every player-facing screen (login, register, lobby, wallet, table) is a React
app in `frontend/`, talking to the Go server over the JSON API under `/api` and
receiving live table updates over SSE at `/api/events`. Only the admin dashboard
is still server-rendered HTML.

Development — run both, and use the Vite URL. It proxies `/api` and `/static`
at the Go server, so cookies and the SSE stream behave as they do in production:

    DATABASE_URL=postgres://poker:poker@localhost:5432/poker?sslmode=disable \
        go run ./cmd/poker-demo -mode=server -addr=:8080
    cd frontend && npm install && npm run dev     # http://localhost:5173

Production — build the bundle, then let the Go server serve it. Any unmatched
GET falls through to `index.html`, so client-side routes survive a refresh:

    cd frontend && npm install && npm run build   # writes frontend/dist
    DATABASE_URL=postgres://poker:poker@localhost:5432/poker?sslmode=disable \
        go run ./cmd/poker-demo -mode=server -addr=:8080

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

## Deposits

Deposits have two modes, switched from **Site Settings** on the admin
dashboard. The default is play money.

**Play money (default).** The player picks an amount and the wallet is credited
immediately. Nothing leaves the machine. This is what makes local testing and
demos possible, and it is why the toggle defaults here rather than to real
money on a fresh install.

**Real money.** The player transfers to the site's CBE account, then pastes the
confirmation SMS CBE sends them. The link at the end of that message is the
receipt; `RECEIPT_VERIFIER_URL` resolves it to the actual transfer, and the
wallet is credited from what the bank says — not from what the message says,
which is only text the player typed into a box.

Three checks decide whether a receipt becomes money:

- **It was paid to us.** The receiver name and account on the receipt have to
  match the deposit account in Site Settings. A receipt proves a transfer
  happened; it does not prove the transfer was to us, which is why this is
  checked on every deposit rather than trusted from the message.
- **It completed.** A transfer the bank still calls pending is not money.
- **It has not been used before.** Every credited deposit claims its bank
  reference, and the reference is unique across the whole table. This is the
  rule that matters most: one transfer has *more than one* receipt link — the
  one CBE texts the sender and the `encodedReceipt` link the bank returns for
  the same transfer differ, and both resolve to the same reference. Keyed on
  the URL, the second link would credit the same money twice.

Turning real money on requires the account name and number to be filled in
first — enabling it without them would make every deposit fail the receiver
check. Switching it on also closes the play-money route: `POST
/api/wallet/deposit` starts answering `403`, so an old bundle or a crafted
request cannot keep minting balance.

Deposits are **not** matched to the sender's identity. Any valid, unused
receipt paid to the site's account credits whoever pastes it first.

### When the verifier is unreachable

The verifier is a third party and can be down. A paste that cannot be checked
is retried three times — each attempt with its own `RECEIPT_VERIFIER_TIMEOUT`
and a growing pause between them — then stored as **pending review** rather
than refused —
the transfer is real and the player should not have to still have the SMS an
hour later. A queued row carries no bank reference, which is exactly what stops
it from crediting anything on its own.

Queued receipts are then re-checked on their own, every `DEPOSIT_SWEEP_INTERVAL`
(5 minutes by default), with the first sweep on start-up — after a restart, the
outage that queued those rows may well be what the restart fixed. A receipt
pasted during an outage therefore credits itself once the verifier comes back,
with nobody touching the dashboard. The same three checks apply on the retry as
on a fresh paste, so a queued row is not a shortcut past any of them.

Each row is retried at most every 10 minutes and gives up after 24 attempts,
roughly a day. Giving up only stops the machine: the row stays in the admin
queue, labelled with how many attempts it has had.

That leaves the dashboard queue holding what genuinely needs a person.
**Verify & credit** re-runs the whole check against the bank; it is not a
credit instruction, so an admin cannot approve a receipt that was never paid to
us or that has already been spent. If the verifier is still down, the row
simply stays queued. Each row also shows the last thing that went wrong — a
timeout, a `502`, a DNS failure — which is what tells you whether to wait or to
go and look at the verifier.

#### When the verifier's CDN blocks the server

A queued row whose note reads **the receipt verifier's CDN blocked this
server** — usually an HTTP `403` carrying a `Just a moment...` HTML page — is a
different problem from an outage, and no amount of waiting fixes it. The
verifier is up; a CDN in front of it decided this server looked like a bot and
answered the challenge page itself, so the call never reached the verifier at
all. This is why such a call is **not** retried inside a single verification:
a challenge does not clear in a second, and the player should not wait out
three attempts for the same page. The row is queued and the background sweep
retries it minutes later, by which time the rule below may exist.

The fix belongs on the CDN, not here. On Cloudflare, the row's `cf-ray=` value
finds the exact request under **Security → Events**, and the rule that fired is
named there. Then one of:

- a WAF custom rule that **skips** bot protection for this server's IP, or
- a rule keyed on `RECEIPT_VERIFIER_TOKEN` — set the same secret on both sides
  and skip on the header, which survives the server changing address, or
- a DNS-only (grey-cloud) hostname for API traffic.

**Credit manually** is the escape hatch for the receipts none of that reaches:
a link the bank has aged out, a receipt format the scraper cannot read, an
outage that outlasts the automatic retries. The admin types the amount from the
receipt and the player is credited with **no bank check at all** — it is the
one path where money moves on somebody's word, so the row records who did it
and that no verification happened, and the amount is capped at 1,000,000 ETB
against a slipped keystroke.

Enter the **FT reference** with it whenever the receipt shows one. That is what
keeps the one-transfer-one-credit rule intact: credited with the reference, the
same transfer arriving later on its other link is refused as already claimed.
Credited without one, only the receipt URL stands between that transfer and a
second credit.

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
- PostgreSQL access goes through `pgx` via `database/sql`, so the binary stays
  a single static executable with no cgo dependency.