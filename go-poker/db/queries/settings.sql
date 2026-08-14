-- name: GetSiteSettings :one
SELECT id, rake_mode, rake_percentage, referral_percentage_pct_mode, referral_percentage_sb_mode, countdown_seconds, updated_at
FROM site_settings
WHERE id = 1 LIMIT 1;

-- name: UpdateSiteSettings :exec
UPDATE site_settings
SET rake_mode = ?,
    rake_percentage = ?,
    referral_percentage_pct_mode = ?,
    referral_percentage_sb_mode = ?,
    countdown_seconds = ?
WHERE id = 1;
