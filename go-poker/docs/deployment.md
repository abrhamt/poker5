# Deploying

The Go binary is a **JSON + SSE API only** — it serves no files. The frontend is
a static bundle that Caddy serves straight off disk.

    browser ──TLS──> Caddy ──┬── /api/*   ──> 127.0.0.1:9011 (Go)
                             ├── /admin*  ──> 127.0.0.1:9011 (Go, until the
                             │                admin UI is its own bundle)
                             └── everything else ──> /var/www/html/goldenpoker/dist
                                                     (index.html fallback)

Everything the browser needs ships inside `frontend/dist`:

    dist/
        index.html
        assets/     hashed JS + CSS
        cards/      the card deck, 54 SVGs
        legacy/     admin.css + admin.js, for the server-rendered admin page

Config lives in [`deploy/Caddyfile`](../deploy/Caddyfile) and
[`deploy/poker.service`](../deploy/poker.service).

## 1. Build

From `go-poker/`:

    # Static binary. The PostgreSQL driver (pgx) is pure Go, so cgo is not
    # needed and the result runs on any Linux box regardless of its glibc.
    CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o poker-server ./cmd/poker-demo

    cd frontend && npm ci && npm run build && cd ..

## 2. PostgreSQL

    sudo apt install postgresql          # or your distribution's package
    sudo -u postgres createuser poker --pwprompt
    sudo -u postgres createdb poker --owner=poker

Nothing else: the server creates its own tables on start-up, and re-runs the
migrations every time, so a deploy that adds a column needs no manual step.

## 3. Server layout

    /srv/poker/                     owned by the poker user
        poker-server                the binary
        .env                        secrets and settings, including DATABASE_URL

    /var/www/html/goldenpoker/dist/ owned by caddy, the whole of frontend/dist

Prepare them:

    sudo useradd --system --home /srv/poker --shell /usr/sbin/nologin poker
    sudo mkdir -p /srv/poker /var/www/html/goldenpoker/dist
    sudo chown -R poker:poker /srv/poker
    sudo chown -R caddy:caddy /var/www/html/goldenpoker

## 4. Upload

    # API
    rsync -av poker-server .env root@YOUR_SERVER:/srv/poker/

    # Frontend. The trailing slash on dist/ matters: it copies the *contents*
    # into the web root rather than nesting a dist/ directory inside it.
    rsync -av --delete frontend/dist/ root@YOUR_SERVER:/var/www/html/goldenpoker/dist/

`--delete` clears out asset files from previous builds, whose hashed names no
longer match anything.

## 5. `.env`

    ADMIN_USERNAME=admin
    ADMIN_PHONE=0900000000
    DATABASE_URL=postgres://poker:<a real password>@localhost:5432/poker?sslmode=disable
    ADMIN_PASSWORD=<a real password>
    SESSION_COOKIE_SECURE=true
    GEEZSMS_TOKEN=<your GeezSMS API token>
    PUBLIC_APP_URL=https://goldenpoker.example.com
    PAYMENT_ROUTER_URL=https://router.razielcc.com
    PAYMENT_ROUTER_API_KEY=<API key minted for this service in the router admin>
    PAYMENT_ROUTER_WEBHOOK_SECRET=<the service's webhook secret from the router admin>

`DATABASE_URL` has no default and the server refuses to start without it — a
fallback connection string would let a misconfigured box come up pointed at the
wrong database. The schema is created and migrated on start-up, so an empty
database is all that is needed.

`SESSION_COOKIE_SECURE=true` is the one that matters. Caddy gives you HTTPS, so
the session cookie should carry the `Secure` attribute — without it the cookie
is also valid over plain HTTP. The server logs a warning on every start until
you set it.

`ADMIN_PASSWORD` is re-applied on **every** start, so this file is the source of
truth for the admin password; changing it in the database does nothing.

`GEEZSMS_TOKEN` is what makes verification codes leave the machine. Without it
the server keeps the console sender, which outside `APP_ENV=development`
refuses to send — so registration and password reset both return `502` until
the token is in place.

The four `PAYMENT_ROUTER_*` / `PUBLIC_APP_URL` lines turn on automatic
deposits. Register this site as a service in the RazielPay router admin with
the webhook URL `https://<your domain>/api/webhooks/razielpay`, mint an API key
and copy the webhook secret. `PUBLIC_APP_URL` is where the gateway sends
players back after paying (`/wallet?deposit=<reference>` is appended). With
any of them missing the server logs a notice at start-up and the "Automatic
deposit" option is not offered, whatever the admin toggle says. Once they are
set, switch "Automatic Deposits" on in the admin dashboard. The router's own
integration guide is at `<PAYMENT_ROUTER_URL>/llm.txt`.

Real environment variables win over `.env`, which is why the unit file's
`Environment=` line overrides it regardless of what the file says.

    sudo chown poker:poker /srv/poker/.env && sudo chmod 600 /srv/poker/.env

## 6. systemd

    sudo cp deploy/poker.service /etc/systemd/system/poker.service
    sudo systemctl daemon-reload
    sudo systemctl enable --now poker
    sudo systemctl status poker

Check it came up on loopback and nowhere else:

    curl -s http://127.0.0.1:9011/health    # {"status":"ok",...}
    ss -ltnp | grep 9011                    # must show 127.0.0.1:9011

## 7. Caddy

    sudo cp deploy/Caddyfile /etc/caddy/Caddyfile
    sudo sed -i 's/poker.example.com/YOUR_DOMAIN/' /etc/caddy/Caddyfile
    sudo caddy validate --config /etc/caddy/Caddyfile
    sudo systemctl reload caddy

Point the domain's A record at the server first — Caddy needs to answer an ACME
challenge on :80 before it can issue a certificate. Ports 80 and 443 must be
open; **9011 must not be.**

## 8. Verify

    curl -sI https://YOUR_DOMAIN/                      # 200, Cache-Control: no-store
    curl -sI https://YOUR_DOMAIN/lobby                 # 200 — SPA fallback
    curl -sI https://YOUR_DOMAIN/assets/<hashed>.js    # 200, immutable
    curl -sI https://YOUR_DOMAIN/cards/AS.svg          # 200 image/svg+xml
    curl -sI https://YOUR_DOMAIN/legacy/admin.css      # 200 text/css

    # SSE must stream rather than buffer — this prints straight away:
    curl -N https://YOUR_DOMAIN/api/events?table_id=lobby

    # The session cookie must carry Secure:
    curl -si https://YOUR_DOMAIN/api/auth/login -H 'Content-Type: application/json' \
      -d '{"login":"nobody","password":"x"}' | grep -i set-cookie

    # And the API must not be reachable directly:
    curl -m 5 http://YOUR_DOMAIN:9011/health           # should fail

## Updating

Front end only — no downtime, nothing to restart:

    cd frontend && npm run build && cd ..
    rsync -av --delete frontend/dist/ root@YOUR_SERVER:/var/www/html/goldenpoker/dist/

Back end:

    CGO_ENABLED=0 GOOS=linux go build -o poker-server ./cmd/poker-demo
    rsync -av poker-server root@YOUR_SERVER:/srv/poker/
    ssh root@YOUR_SERVER 'systemctl restart poker'

A restart drops every open SSE connection; browsers reconnect on their own, and
seated players keep their chips because table state is persisted to PostgreSQL. Do
still avoid restarting mid-hand — the turn timer keeps running, and a player who
cannot reconnect in time gets folded.

## When the admin UI becomes a bundle

Two changes, no more:

1. Delete the `reverse_proxy /admin*` block from the Caddyfile.
2. Delete `frontend/public/legacy/` and the server-rendered handlers
   (`view_templates_admin.go`, `view_templates_layout.go`, `RenderDashboard`).

The admin API under `/api/admin/*` already returns JSON and needs no change.

## Troubleshooting

**405 on login, and API GETs returning HTML.** The Caddyfile is written flat
instead of in `handle` blocks. Caddy runs directives in its own order, not the
written one, and `try_files` runs before `reverse_proxy` — so `/api/...` gets
rewritten to `/index.html` before the proxy matches it. `file_server` then
answers the POST with 405 and answers GETs with the HTML shell, which the client
cannot parse. Use the `handle` blocks in `deploy/Caddyfile`.

**502 on /api.** Caddy is right, the API is not running. `curl
localhost:9011/health` on the box, then check `poker.log`.

**The service exits immediately with a database error.** `DATABASE_URL` is
missing, wrong, or PostgreSQL is not accepting the connection. The server pings
the database before it serves anything, so this fails at start-up rather than on
the first request — check the message in `journalctl -u poker`, then try the
same URL with `psql "$DATABASE_URL" -c 'select 1'`.

**The binary exits 127.** "Not found", which for a present file usually means it
was built with cgo enabled and is dynamically linked against a glibc the server
does not have. Rebuild with `CGO_ENABLED=0`; confirm with `file poker-server`,
which should say *statically linked*.

**Signed out constantly, or the session never sticks.** `SESSION_COOKIE_SECURE`
is on but the site is being reached over plain HTTP or a bare IP. The browser
drops a `Secure` cookie on a non-secure origin.

## Notes

- **Back up the database.** It holds wallets. `pg_dump` is safe to run while
  the server is live:

      sudo -u postgres pg_dump poker | gzip > /path/poker-$(date +%F).sql.gz
- The API answers JSON for every unrouted path, including `/`. If you open the
  binary's port directly and get `{"error":"not found"}`, that is correct — the
  app is at the domain Caddy serves.
- `/health` is not exposed through Caddy. Reach it on the box, or add it to the
  proxied paths if an external monitor needs it.
