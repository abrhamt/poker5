# Golden Poker — frontend

React + TypeScript + Vite + Tailwind v4. This is every player-facing screen;
the Go server in the parent directory owns the API, the SSE hub, and the admin
dashboard.

    npm install
    npm run dev        # http://localhost:5173, proxies /api + /static to :8080
    npm run build      # writes dist/, which the Go server serves in production
    npm run typecheck

## How it fits together

- `src/lib/api.ts` — every call to the Go API. Errors surface as `ApiError`
  carrying the server's own message, so the UI can show it verbatim.
- `src/lib/types.ts` — the JSON shapes, mirrored from `GameEngine.ToDict()` and
  `GameService.GetTableState()`. Keep them in sync when the Go side changes.
- `src/lib/useSSE.ts` — one `EventSource` per channel (a room code, or `lobby`).
  Broadcast payloads are deliberately *not* personalised by the server (they
  hide every hole card), so a `game-state` event is treated as "something
  changed" and the page re-reads its own view of the table.
- `src/lib/useCountdown.ts` — all timers count against absolute epoch-ms
  deadlines from the server, never a decrementing local number.
- `src/components/table/seatLayout.ts` — seats are placed on an ellipse and the
  roster is rotated so the local player always sits at the bottom, which means
  any table size from 2 to 9 lays out without new CSS.

## Styling

Design tokens live in the `@theme` block of `src/index.css` and become real
Tailwind utilities (`bg-panel`, `text-gold-light`, `border-gold`). Repeated
element styles (`btn-gold`, `card`, `field`, `label`) are registered with
`@utility` so they compose with variants; one-off styling stays inline on the
element.
