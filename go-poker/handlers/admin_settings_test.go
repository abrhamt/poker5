package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/zuse/poker5/go-poker/repository"
)

// With htmx unavailable the settings form submits as a plain POST. It has to
// save and redirect, not silently drop what was typed.
func TestAdminSettingsWithoutHtmx(t *testing.T) {
	t.Setenv("ADMIN_PASSWORD", "admin-password-123")
	db := setupTestDB(t)
	defer db.Close()
	srv, _ := newTestServer(t, db)

	payload, _ := json.Marshal(map[string]string{"login": "admin", "password": "admin-password-123"})
	req := httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader(string(payload)))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := srv.App.Test(req)
	cookie := strings.Split(resp.Header.Get("Set-Cookie"), ";")[0]

	form := "rake_mode=small_blind&rake_percentage=7.5&referral_percentage_pct_mode=12&referral_percentage_sb_mode=13&countdown_seconds=30&real_deposits_enabled=false&deposit_account_name=&deposit_account_number="
	req = httptest.NewRequest(http.MethodPost, "/api/admin/settings", strings.NewReader(form))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Cookie", cookie)
	// No HX-Request header: this is the browser's own form submission.
	resp, err := srv.App.Test(req)
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	if resp.StatusCode != http.StatusFound && resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("status = %d, want a redirect", resp.StatusCode)
	}
	if loc := resp.Header.Get("Location"); loc != "/admin" {
		t.Errorf("Location = %q, want /admin", loc)
	}

	got, err := repository.New(db).GetSiteSettings(t.Context())
	if err != nil {
		t.Fatalf("read settings: %v", err)
	}
	if got.RakeMode != "small_blind" || got.RakePercentage != 7.5 || got.CountdownSeconds != 30 {
		t.Fatalf("settings did not persist without htmx: %+v", got)
	}
}

// The dashboard must not depend on a third-party CDN for the script that makes
// every control on it work.
func TestAdminPageServesHtmxFromOwnOrigin(t *testing.T) {
	html := renderBaseLayout("Admin", "<p>x</p>", "admin")
	if strings.Contains(html, "unpkg.com") {
		t.Error("dashboard still loads htmx from unpkg")
	}
	if !strings.Contains(html, `src="/legacy/htmx.min.js"`) {
		t.Error("dashboard does not load htmx from /legacy/")
	}
}
