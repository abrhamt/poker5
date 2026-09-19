# Password reset — design

**Status:** design only, not built. Written 2026-08-19.

## Why this needs a design rather than just code

Accounts here hold real money. A password reset is, by definition, an
account-takeover path that the site offers on purpose — so the interesting part
is not the happy path, it's the constraints that stop it being abused. The code
is perhaps a day's work; the decisions below are what make it safe.

## The constraint that decides the shape

The app collects **no email address** — `users` has `username`, `phone_number`,
`password_hash`, `referral_code`, `role`. Phone numbers are validated and
normalised to Ethiopian format (`utilities.ValidateAndNormalizeEthiopianPhone`)
and are unique.

So the recovery channel is SMS, or it is a human being. There is no third
option that doesn't start with a schema change and a request that every
existing user supply something they never gave us.

## Recommended shape: one flow, two delivery channels

Build the reset *mechanism* once and put delivery behind an interface:

```go
// Delivers a one-time code to a user through some channel.
type CodeDeliverer interface {
    Deliver(ctx context.Context, phone, code string) error
}
```

- `AdminDeliverer` — records the code for an admin to read out to the player.
  No vendor, no cost, works the day it ships.
- `SMSDeliverer` — sends via the gateway. Drops in later without touching the
  flow, the schema, or the endpoints.

That way choosing a gateway is a procurement question that doesn't block the
engineering, and switching later is one implementation, not a rewrite.

## Schema

```sql
CREATE TABLE IF NOT EXISTS password_resets (
    id            BIGINT AUTO_INCREMENT PRIMARY KEY,
    user_id       BIGINT NOT NULL,
    code_hash     VARCHAR(60) NOT NULL,   -- bcrypt of the 6-digit code, never the code
    expires_at    TIMESTAMP NOT NULL,
    consumed_at   TIMESTAMP NULL,         -- single use
    attempts      INT NOT NULL DEFAULT 0, -- verification attempts against this code
    created_at    TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    INDEX idx_reset_user (user_id),
    INDEX idx_reset_expiry (expires_at),
    CONSTRAINT fk_reset_user FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
);
```

Storing `code_hash` rather than the code means a database read — a backup, a
log, a compromised replica — does not hand over live reset codes.

## Endpoints

### `POST /api/auth/forgot-password`
Body: `{"phone_number": "0911223344"}`

1. Normalise the phone. Look up the user.
2. Invalidate any outstanding codes for that user.
3. Generate a 6-digit code from `crypto/rand` (**not** `math/rand`).
4. Store its bcrypt hash with a 10-minute expiry.
5. Hand it to the `CodeDeliverer`.
6. **Always return the same 200 response**, whether or not that phone exists.

Step 6 is not optional. A different response for an unknown number turns this
endpoint into a "does this person have an account here" oracle, and the answer
is worth money to someone.

### `POST /api/auth/reset-password`
Body: `{"phone_number": "...", "code": "123456", "new_password": "..."}`

1. Load the newest unconsumed, unexpired code for that user.
2. Increment `attempts` **before** comparing. Past 5, mark it consumed and
   refuse — a 6-digit code is 10^6 guesses, which is nothing without a cap.
3. Compare with `bcrypt.CompareHashAndPassword`.
4. On success: update `password_hash`, set `consumed_at`, and
   **delete every row in `user_sessions` for that user**.

Step 4's session purge is the point of the whole exercise. Resetting the
password while an attacker's session stays live achieves nothing. Note this gap
exists today in `EnsureAdminUser`, which rewrites the admin password on every
startup and leaves existing sessions untouched.

## Rate limits

Neither endpoint is safe without limits, and **there is no rate limiting
anywhere in the app today** — including on login. Build one small reusable
limiter and apply it to all three:

| Endpoint | Limit |
|---|---|
| `forgot-password` | 3 per phone per hour, 10 per IP per hour |
| `reset-password` | 5 attempts per code (then burn it), 10 per IP per hour |
| `login` | 10 per account per 15 min, 30 per IP per 15 min |

SMS costs money per message, so an unlimited `forgot-password` is also a way to
spend someone else's budget, not just a security hole.

## Admin-assisted reset — the interim path

The admin dashboard already has user search. Add a "reset password" action that
runs the same flow with `AdminDeliverer`.

Be clear about what this is: a button that lets an administrator take over any
account holding real money. It needs, at minimum, an audit row recording who
reset whom and when. It does not scale past a small user base, and it should be
retired once SMS is live rather than kept "just in case".

## SMS gateway options

| Option | Setup | Notes |
|---|---|---|
| Africa's Talking | Days | Developer-friendly API, supports Ethiopia. Fastest path to a working integration. |
| Ethio Telecom bulk SMS | Weeks | Business agreement required. Cheapest per message at volume. |
| Twilio | Hours | Simplest API, but higher cost and delivery into Ethiopia can be unreliable. |

Recommendation: integrate against Africa's Talking first because it unblocks
development, and keep the `CodeDeliverer` seam so moving to Ethio Telecom later
is a swap rather than a migration.

## Open questions

- Should a reset trigger a cool-off on cash-outs? Not pressing today — the
  wallet only supports deposit and cash-out-to-wallet — but it matters the
  moment withdrawals to a real payment rail exist.
- Should players be able to change their password while signed in? Same
  mechanism minus the code; needs the same session purge.
