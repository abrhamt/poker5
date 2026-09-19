-- name: GetSiteSettings :one
SELECT id, rake_mode, rake_percentage, referral_percentage_pct_mode, referral_percentage_sb_mode, countdown_seconds,
       real_deposits_enabled, deposit_account_name, deposit_account_number, gateway_deposits_enabled, updated_at
FROM site_settings
WHERE id = 1 LIMIT 1;

-- name: UpdateSiteSettings :exec
-- updated_at is set here rather than by the column: PostgreSQL has no
-- ON UPDATE CURRENT_TIMESTAMP.
UPDATE site_settings
SET rake_mode = $1,
    rake_percentage = $2,
    referral_percentage_pct_mode = $3,
    referral_percentage_sb_mode = $4,
    countdown_seconds = $5,
    real_deposits_enabled = $6,
    deposit_account_name = $7,
    deposit_account_number = $8,
    gateway_deposits_enabled = $9,
    updated_at = NOW()
WHERE id = 1;
