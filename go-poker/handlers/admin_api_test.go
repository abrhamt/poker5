package handlers

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/zuse/poker5/go-poker/repository"
)

func adminCookie(t *testing.T, srv *FiberServer) string {
	t.Helper()
	resp := postJSON(t, srv, "/api/auth/login", map[string]string{"login": "admin", "password": "admin-password-123"}, "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("admin login: status %d", resp.StatusCode)
	}
	return strings.Split(resp.Header.Get("Set-Cookie"), ";")[0]
}

func getJSON(t *testing.T, srv *FiberServer, path, cookie string, out any) int {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	if cookie != "" {
		req.Header.Set("Cookie", cookie)
	}
	resp, err := srv.App.Test(req)
	if err != nil {
		t.Fatalf("GET %s: %v", path, err)
	}
	if out != nil {
		body, _ := io.ReadAll(resp.Body)
		if err := json.Unmarshal(body, out); err != nil {
			t.Fatalf("GET %s: decode %s: %v", path, body, err)
		}
	}
	return resp.StatusCode
}

func decode(t *testing.T, resp *http.Response, out any) {
	t.Helper()
	body, _ := io.ReadAll(resp.Body)
	if err := json.Unmarshal(body, out); err != nil {
		t.Fatalf("decode %s: %v", body, err)
	}
}

// The dashboard tells "sign in" apart from "not an admin", and both arrive as
// JSON rather than a redirect to an HTML login page that no longer exists.
func TestAdminAPIRequiresAdmin(t *testing.T) {
	t.Setenv("ADMIN_PASSWORD", "admin-password-123")
	db := setupTestDB(t)
	defer db.Close()
	srv, sender := newTestServer(t, db)

	var body map[string]any
	if status := getJSON(t, srv, "/api/admin/overview", "", &body); status != http.StatusUnauthorized {
		t.Fatalf("signed out: status %d, want 401", status)
	}

	player := strings.Split(registerAndVerify(t, srv, sender, "bob", "0911000001", "password123"), ";")[0]
	if status := getJSON(t, srv, "/api/admin/overview", player, &body); status != http.StatusForbidden {
		t.Fatalf("player: status %d, want 403", status)
	}

	if status := getJSON(t, srv, "/api/admin/overview", adminCookie(t, srv), &body); status != http.StatusOK {
		t.Fatalf("admin: status %d, want 200", status)
	}
}

func TestAdminSettingsRoundTrip(t *testing.T) {
	t.Setenv("ADMIN_PASSWORD", "admin-password-123")
	db := setupTestDB(t)
	defer db.Close()
	srv, _ := newTestServer(t, db)
	cookie := adminCookie(t, srv)

	resp := postJSON(t, srv, "/api/admin/settings", map[string]any{
		"rake_mode":                       "small_blind",
		"rake_percentage":                 7.5,
		"referral_percentage_pct_mode":    12,
		"referral_percentage_sb_mode":     13,
		"countdown_seconds":               30,
		"intermission_seconds":            4,
		"withdrawals_enabled":             true,
		"withdrawal_min_amount":           75,
		"withdrawal_cancel_enabled":       false,
		"withdrawal_reset_cooldown_hours": 48,
	}, cookie)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("save settings: status %d", resp.StatusCode)
	}

	got, err := repository.New(db).GetSiteSettings(t.Context())
	if err != nil {
		t.Fatalf("read settings: %v", err)
	}
	if got.RakeMode != "small_blind" || got.RakePercentage != 7.5 || got.CountdownSeconds != 30 || got.IntermissionSeconds != 4 ||
		!got.WithdrawalsEnabled || got.WithdrawalMinAmount != 75 || got.WithdrawalCancelEnabled || got.WithdrawalResetCooldownHours != 48 {
		t.Fatalf("settings did not persist: %+v", got)
	}

	var out struct {
		Settings map[string]any `json:"settings"`
		Limits   map[string]any `json:"limits"`
	}
	getJSON(t, srv, "/api/admin/settings", cookie, &out)
	if out.Settings["withdrawal_min_amount"] != float64(75) || out.Limits["withdrawal_min_amount"] == nil {
		t.Fatalf("settings payload = %+v", out)
	}

	resp = postJSON(t, srv, "/api/admin/settings", map[string]any{"rake_mode": "nonsense", "countdown_seconds": 30}, cookie)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("invalid settings: status %d, want 400", resp.StatusCode)
	}
}

// The whole path through HTTP: a player asks, the admin sees it in the queue,
// rejects it, and the money is back in the wallet; a second request is then
// completed.
func TestWithdrawalFlowThroughAPI(t *testing.T) {
	t.Setenv("ADMIN_PASSWORD", "admin-password-123")
	db := setupTestDB(t)
	defer db.Close()
	srv, sender := newTestServer(t, db)
	admin := adminCookie(t, srv)
	player := strings.Split(registerAndVerify(t, srv, sender, "carol", "0911000002", "password123"), ";")[0]

	if resp := postJSON(t, srv, "/api/wallet/deposit", map[string]int64{"amount": 1000}, player); resp.StatusCode != http.StatusOK {
		t.Fatalf("deposit: status %d", resp.StatusCode)
	}

	request := map[string]any{"method": "telebirr", "account_number": "+251 911 000 002", "account_name": "Carol Tesfaye", "amount": 300}
	if resp := postJSON(t, srv, "/api/wallet/withdraw", request, player); resp.StatusCode != http.StatusForbidden {
		t.Fatalf("withdraw while disabled: status %d, want 403", resp.StatusCode)
	}

	if resp := postJSON(t, srv, "/api/admin/settings", map[string]any{
		"rake_mode": "percentage", "rake_percentage": 5, "countdown_seconds": 15,
		"withdrawals_enabled": true, "withdrawal_min_amount": 50, "withdrawal_cancel_enabled": true,
	}, admin); resp.StatusCode != http.StatusOK {
		t.Fatalf("enable withdrawals: status %d", resp.StatusCode)
	}

	resp := postJSON(t, srv, "/api/wallet/withdraw", request, player)
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("withdraw: status %d %s", resp.StatusCode, body)
	}
	var created struct {
		Withdrawal struct {
			ID            int64  `json:"id"`
			AccountNumber string `json:"account_number"`
			Status        string `json:"status"`
		} `json:"withdrawal"`
	}
	decode(t, resp, &created)
	if created.Withdrawal.AccountNumber != "0911000002" || created.Withdrawal.Status != "pending" {
		t.Fatalf("created = %+v", created)
	}

	if resp := postJSON(t, srv, "/api/wallet/withdraw", request, player); resp.StatusCode != http.StatusConflict {
		t.Fatalf("second withdraw: status %d, want 409", resp.StatusCode)
	}

	var info struct {
		Enabled     bool             `json:"enabled"`
		MinAmount   int64            `json:"min_amount"`
		Withdrawals []map[string]any `json:"withdrawals"`
	}
	getJSON(t, srv, "/api/wallet/withdraw-info", player, &info)
	if !info.Enabled || info.MinAmount != 50 || len(info.Withdrawals) != 1 {
		t.Fatalf("withdraw-info = %+v", info)
	}

	var queue struct {
		Items []map[string]any `json:"items"`
		Total int64            `json:"total"`
	}
	getJSON(t, srv, "/api/admin/withdrawals?status=pending", admin, &queue)
	if queue.Total != 1 || queue.Items[0]["username"] != "carol" {
		t.Fatalf("admin queue = %+v", queue)
	}

	path := "/api/admin/withdrawals/" + jsonNumber(created.Withdrawal.ID)
	if resp := postJSON(t, srv, path+"/reject", map[string]string{}, admin); resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("reject without reason: status %d, want 400", resp.StatusCode)
	}
	if resp := postJSON(t, srv, path+"/reject", map[string]string{"reason": "Name does not match"}, admin); resp.StatusCode != http.StatusOK {
		t.Fatalf("reject: status %d", resp.StatusCode)
	}
	if resp := postJSON(t, srv, path+"/complete", map[string]string{}, admin); resp.StatusCode != http.StatusConflict {
		t.Fatalf("complete after reject: status %d, want 409", resp.StatusCode)
	}

	var me struct {
		User struct {
			Wallet int64 `json:"wallet"`
		} `json:"user"`
	}
	getJSON(t, srv, "/api/auth/me", player, &me)
	if me.User.Wallet != 1000 {
		t.Fatalf("wallet after rejection = %d, want 1000", me.User.Wallet)
	}

	resp = postJSON(t, srv, "/api/wallet/withdraw", request, player)
	decode(t, resp, &created)
	path = "/api/admin/withdrawals/" + jsonNumber(created.Withdrawal.ID)
	if resp := postJSON(t, srv, path+"/complete", map[string]string{"reference": "TB123"}, admin); resp.StatusCode != http.StatusOK {
		t.Fatalf("complete: status %d", resp.StatusCode)
	}
	getJSON(t, srv, "/api/auth/me", player, &me)
	if me.User.Wallet != 700 {
		t.Fatalf("wallet after completion = %d, want 700", me.User.Wallet)
	}
}

func jsonNumber(n int64) string {
	b, _ := json.Marshal(n)
	return string(b)
}
