package handlers

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"sync"
	"testing"

	"github.com/zuse/poker5/go-poker/services"
)

// recordingSender stands in for the SMS provider. Reading the code straight out
// of the database would be simpler, but it would make every assertion about how
// many messages went out — the cooldown, the hourly cap — impossible to write.
type recordingSender struct {
	mu   sync.Mutex
	sent []sentMessage
}

type sentMessage struct {
	Phone   string
	Message string
}

func (s *recordingSender) Send(_ context.Context, phone, message string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sent = append(s.sent, sentMessage{Phone: phone, Message: message})
	return nil
}

func (s *recordingSender) count() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.sent)
}

var codePattern = regexp.MustCompile(`\b\d{6}\b`)

// lastCode returns the code from the most recent message, which is always the
// live one: a resend mints a new code and invalidates the old.
func (s *recordingSender) lastCode(t *testing.T) string {
	t.Helper()
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.sent) == 0 {
		t.Fatalf("expected an OTP to have been sent, none was")
	}
	code := codePattern.FindString(s.sent[len(s.sent)-1].Message)
	if code == "" {
		t.Fatalf("no six-digit code in message %q", s.sent[len(s.sent)-1].Message)
	}
	return code
}

func newTestServer(t *testing.T, db *sql.DB) (*FiberServer, *recordingSender) {
	t.Helper()
	sender := &recordingSender{}
	srv := NewFiberServer(db, WithOTPSender(sender))
	t.Cleanup(srv.Stop)
	return srv, sender
}

// newTestServerNoSMS is for tests that never send a code but still must not be
// able to. NewFiberServer reads .env, so a real GEEZSMS_TOKEN sitting in a
// developer's file would otherwise wire up the live gateway and a test could
// text a real phone.
func newTestServerNoSMS(t *testing.T, db *sql.DB) *FiberServer {
	t.Helper()
	srv, _ := newTestServer(t, db)
	return srv
}

func postJSON(t *testing.T, srv *FiberServer, path string, payload any, cookie string) *http.Response {
	t.Helper()
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal %s payload: %v", path, err)
	}
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if cookie != "" {
		req.Header.Set("Cookie", cookie)
	}
	resp, err := srv.App.Test(req)
	if err != nil {
		t.Fatalf("%s request: %v", path, err)
	}
	return resp
}

// registerAndVerify drives both halves of a signup and returns the raw
// Set-Cookie header from the verification, which is where the session is now
// issued.
func registerAndVerify(t *testing.T, srv *FiberServer, sender *recordingSender, username, phone, password string) string {
	t.Helper()

	resp := postJSON(t, srv, "/api/auth/register", map[string]string{
		"username":     username,
		"phone_number": phone,
		"password":     password,
	}, "")
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("register %s: expected 202, got %d", username, resp.StatusCode)
	}

	resp = postJSON(t, srv, "/api/auth/verify-otp", map[string]string{
		"phone_number": phone,
		"code":         sender.lastCode(t),
	}, "")
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("verify %s: expected 201, got %d", username, resp.StatusCode)
	}

	setCookie := resp.Header.Get("Set-Cookie")
	if setCookie == "" {
		t.Fatalf("expected a session cookie after verification")
	}
	return setCookie
}

func countUsers(t *testing.T, db *sql.DB, username string) int {
	t.Helper()
	var n int
	if err := db.QueryRow("SELECT COUNT(*) FROM users WHERE username = $1", username).Scan(&n); err != nil {
		t.Fatalf("count users: %v", err)
	}
	return n
}

// Registration creates nothing until the phone is proven, then signs the new
// player straight in.
func TestRegistrationOTPFlow(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	srv, sender := newTestServer(t, db)

	resp := postJSON(t, srv, "/api/auth/register", map[string]string{
		"username":     "hanna",
		"phone_number": "0911445566",
		"password":     "secret123",
	}, "")
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("register: expected 202, got %d", resp.StatusCode)
	}
	if sender.count() != 1 {
		t.Fatalf("expected exactly one SMS, got %d", sender.count())
	}
	if n := countUsers(t, db, "hanna"); n != 0 {
		t.Fatalf("expected no users row before verification, found %d", n)
	}

	var started map[string]string
	if err := json.NewDecoder(resp.Body).Decode(&started); err != nil {
		t.Fatalf("decode register response: %v", err)
	}
	if started["phone_number"] != "+251911445566" {
		t.Fatalf("expected the normalized phone back, got %q", started["phone_number"])
	}

	resp = postJSON(t, srv, "/api/auth/verify-otp", map[string]string{
		"phone_number": started["phone_number"],
		"code":         sender.lastCode(t),
	}, "")
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("verify: expected 201, got %d", resp.StatusCode)
	}
	if n := countUsers(t, db, "hanna"); n != 1 {
		t.Fatalf("expected the account to exist after verification, found %d", n)
	}

	cookie := strings.Split(resp.Header.Get("Set-Cookie"), ";")[0]
	meReq := httptest.NewRequest(http.MethodGet, "/api/auth/me", nil)
	meReq.Header.Set("Cookie", cookie)
	meResp, err := srv.App.Test(meReq)
	if err != nil || meResp.StatusCode != http.StatusOK {
		t.Fatalf("expected the verification cookie to authenticate, got %v (%v)", meResp.StatusCode, err)
	}

	// The pending row is gone, so the username is held by the account itself.
	var pending int
	if err := db.QueryRow("SELECT COUNT(*) FROM pending_registrations").Scan(&pending); err != nil {
		t.Fatalf("count pending: %v", err)
	}
	if pending != 0 {
		t.Fatalf("expected the pending registration to be consumed, found %d", pending)
	}

	if resp := postJSON(t, srv, "/api/auth/login", map[string]string{
		"login":    "hanna",
		"password": "secret123",
	}, ""); resp.StatusCode != http.StatusOK {
		t.Fatalf("login with the chosen password: expected 200, got %d", resp.StatusCode)
	}
}

// A completed reset changes the password, signs the resetting device in, and
// evicts every session that existed before it.
func TestPasswordResetOTPFlow(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	srv, sender := newTestServer(t, db)

	oldCookie := strings.Split(registerAndVerify(t, srv, sender, "dawit", "0911778899", "oldpass123"), ";")[0]

	resp := postJSON(t, srv, "/api/auth/forgot-password", map[string]string{
		"phone_number": "0911778899",
	}, "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("forgot-password: expected 200, got %d", resp.StatusCode)
	}

	resp = postJSON(t, srv, "/api/auth/verify-reset-otp", map[string]string{
		"phone_number": "0911778899",
		"code":         sender.lastCode(t),
	}, "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("verify-reset-otp: expected 200, got %d", resp.StatusCode)
	}
	var verified map[string]string
	if err := json.NewDecoder(resp.Body).Decode(&verified); err != nil {
		t.Fatalf("decode verify-reset response: %v", err)
	}
	if verified["reset_token"] == "" {
		t.Fatalf("expected a reset token")
	}

	resp = postJSON(t, srv, "/api/auth/reset-password", map[string]string{
		"reset_token": verified["reset_token"],
		"password":    "newpass456",
	}, "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("reset-password: expected 200, got %d", resp.StatusCode)
	}
	newCookie := strings.Split(resp.Header.Get("Set-Cookie"), ";")[0]
	if newCookie == "" {
		t.Fatalf("expected the resetting device to be signed in")
	}

	if resp := postJSON(t, srv, "/api/auth/login", map[string]string{
		"login":    "dawit",
		"password": "oldpass123",
	}, ""); resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("old password should be dead, got %d", resp.StatusCode)
	}
	if resp := postJSON(t, srv, "/api/auth/login", map[string]string{
		"login":    "dawit",
		"password": "newpass456",
	}, ""); resp.StatusCode != http.StatusOK {
		t.Fatalf("new password should work, got %d", resp.StatusCode)
	}

	// The session that existed before the reset is gone: a reset that leaves
	// an attacker's session alive would accomplish nothing.
	meReq := httptest.NewRequest(http.MethodGet, "/api/auth/me", nil)
	meReq.Header.Set("Cookie", oldCookie)
	meResp, err := srv.App.Test(meReq)
	if err != nil {
		t.Fatalf("me request: %v", err)
	}
	if meResp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected the pre-reset session to be evicted, got %d", meResp.StatusCode)
	}

	meReq = httptest.NewRequest(http.MethodGet, "/api/auth/me", nil)
	meReq.Header.Set("Cookie", newCookie)
	meResp, err = srv.App.Test(meReq)
	if err != nil || meResp.StatusCode != http.StatusOK {
		t.Fatalf("expected the resetting device to stay signed in, got %v (%v)", meResp.StatusCode, err)
	}
}

var _ = services.PurposeRegister
