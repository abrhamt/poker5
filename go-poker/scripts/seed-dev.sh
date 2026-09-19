#!/usr/bin/env bash
# Seeds a running dev server with players you can log in as, so testing a hand
# doesn't start with registering four accounts by hand.
#
#   ./scripts/seed-dev.sh                 # 4 players, all funded
#   ./scripts/seed-dev.sh -n 6            # 6 players
#   ./scripts/seed-dev.sh -s 10           # also seat them at a 10/20 table
#   ./scripts/seed-dev.sh -u http://127.0.0.1:8080
#
# Every player uses the password "password123". Re-running is harmless: accounts
# that already exist are logged in rather than recreated.
set -euo pipefail

BASE="http://127.0.0.1:8080"
COUNT=4
DEPOSIT=10000
SEAT_BLIND=""
PASSWORD="password123"

usage() { sed -n '2,12p' "$0" | sed 's/^# \{0,1\}//'; exit "${1:-0}"; }

while getopts "u:n:d:s:h" opt; do
  case "$opt" in
    u) BASE="$OPTARG" ;;
    n) COUNT="$OPTARG" ;;
    d) DEPOSIT="$OPTARG" ;;
    s) SEAT_BLIND="$OPTARG" ;;
    h) usage 0 ;;
    *) usage 2 ;;
  esac
done

case "$BASE" in
  *localhost*|*127.0.0.1*|*192.168.*|*10.*) ;;
  *) echo "refusing to seed a non-local server: $BASE" >&2; exit 1 ;;
esac

if ! curl -sf "$BASE/health" >/dev/null; then
  echo "no server at $BASE — start it with:" >&2
  echo "  go run ./cmd/poker-demo -mode=server -addr=:8080 -db=poker.db" >&2
  exit 1
fi

jar_dir="$(mktemp -d)"
trap 'rm -rf "$jar_dir"' EXIT

api() { # api <cookie-jar> <method> <path> [json]
  local jar="$1" method="$2" path="$3" body="${4:-}"
  if [ -n "$body" ]; then
    curl -s -b "$jar" -c "$jar" -X "$method" "$BASE$path" \
      -H 'Content-Type: application/json' -d "$body"
  else
    curl -s -b "$jar" -c "$jar" -X "$method" "$BASE$path"
  fi
}

echo "seeding $COUNT players at $BASE"
for i in $(seq 1 "$COUNT"); do
  user="dev$i"
  jar="$jar_dir/$user"
  phone="09000000$(printf '%02d' "$i")"

  api "$jar" POST /api/auth/register \
    "{\"username\":\"$user\",\"phone_number\":\"$phone\",\"password\":\"$PASSWORD\"}" >/dev/null
  # Already registered from a previous run — just sign in.
  api "$jar" POST /api/auth/login \
    "{\"login\":\"$user\",\"password\":\"$PASSWORD\"}" >/dev/null

  api "$jar" POST /api/wallet/deposit "{\"amount\":$DEPOSIT}" >/dev/null
  balance=$(api "$jar" GET /api/auth/me | grep -o '"wallet":[0-9]*' | cut -d: -f2)
  printf '  %-6s / %-13s  phone %s  balance %s\n' "$user" "$PASSWORD" "$phone" "${balance:-?}"
done

if [ -n "$SEAT_BLIND" ]; then
  room=$(api "$jar_dir/dev1" POST /api/rooms/quick-join "{\"small_blind\":$SEAT_BLIND}" \
    | grep -o '"room_code":"[0-9]*"' | cut -d'"' -f4)
  if [ -z "$room" ]; then
    echo "could not open a $SEAT_BLIND/... table" >&2
    exit 1
  fi
  for i in $(seq 1 "$COUNT"); do
    api "$jar_dir/dev$i" POST "/api/table/$room/join" >/dev/null
  done
  echo "seated $COUNT players at table #$room — it will deal on its own once the countdown ends"
fi

echo
echo "sign in at the dev server as any of the above; use a separate browser"
echo "profile or a private window per player, since the session is one cookie."
