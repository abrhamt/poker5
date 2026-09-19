package handlers

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/zuse/poker5/go-poker/repository"
	"github.com/zuse/poker5/go-poker/services"
	"github.com/zuse/poker5/go-poker/utilities"
)

const testWebhookSecret = "whsec_handler_test"

type stubRouter struct {
	configured bool
	created    []services.RouterCreatePayment
}

func (s *stubRouter) Configured() bool      { return s.configured }
func (s *stubRouter) WebhookSecret() string { return testWebhookSecret }

func (s *stubRouter) CreatePayment(_ context.Context, req services.RouterCreatePayment) (*services.RouterPayment, error) {
	s.created = append(s.created, req)
	return &services.RouterPayment{ID: "pay_" + req.Reference, Status: "pending", CheckoutURL: "https://checkout.test/" + req.Reference, Reference: req.Reference}, nil
}

func (s *stubRouter) FetchPayment(context.Context, string) (*services.RouterPayment, error) {
	return nil, services.ErrPaymentRouterUnavailable
}

func newGatewayTestServer(t *testing.T, db *sql.DB, router *stubRouter) (*FiberServer, *recordingSender) {
	t.Helper()
	t.Setenv("PUBLIC_APP_URL", "https://poker.test")
	sender := &recordingSender{}
	srv := NewFiberServer(db, WithOTPSender(sender), WithPaymentRouter(router))
	t.Cleanup(srv.Stop)
	return srv, sender
}

func enableGatewayDeposits(t *testing.T, db *sql.DB, receiptsToo bool) {
	t.Helper()
	settings := services.NewSettingsService(repository.New(db))
	req := services.UpdateSettingsRequest{
		RakeMode:               services.RakeModePercentage,
		RakePercentage:         5,
		CountdownSeconds:       15,
		GatewayDepositsEnabled: true,
	}
	if receiptsToo {
		req.RealDepositsEnabled = true
		req.DepositAccountName = testDepositAccountName
		req.DepositAccountNumber = testDepositAccountNumber
	}
	if err := settings.Update(context.Background(), req); err != nil {
		t.Fatalf("enable gateway deposits: %v", err)
	}
}

func signedCallback(t *testing.T, srv *FiberServer, secret string, payload any) *http.Response {
	t.Helper()
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal callback: %v", err)
	}
	ts := strconv.FormatInt(time.Now().Unix(), 10)
	req := httptest.NewRequest(http.MethodPost, "/api/webhooks/razielpay", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(utilities.RouterTimestampHeader, ts)
	req.Header.Set(utilities.RouterSignatureHeader, "v1="+utilities.RouterSignature(secret, ts, body))
	req.Header.Set(utilities.RouterDeliveryIDHeader, "whd_test")
	resp, err := srv.App.Test(req)
	if err != nil {
		t.Fatalf("callback request: %v", err)
	}
	return resp
}

func TestDepositInfoAdvertisesGatewayMethod(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	srv, sender := newGatewayTestServer(t, db, &stubRouter{configured: true})
	cookie := registerAndVerify(t, srv, sender, "alice", "0911223344", "password123")
	enableGatewayDeposits(t, db, false)

	req := httptest.NewRequest(http.MethodGet, "/api/wallet/deposit-info", nil)
	req.Header.Set("Cookie", cookie)
	resp, err := srv.App.Test(req)
	if err != nil {
		t.Fatalf("deposit-info: %v", err)
	}
	body := decodeJSON(t, resp)

	if body["real_deposits_enabled"] != true {
		t.Fatalf("real_deposits_enabled = %v, want true", body["real_deposits_enabled"])
	}
	methods, _ := body["methods"].([]any)
	if len(methods) != 1 || methods[0] != "gateway" {
		t.Fatalf("methods = %v, want [gateway]", body["methods"])
	}
	if _, present := body["account_number"]; present {
		t.Fatalf("account number advertised while receipts are off: %v", body)
	}
	if body["gateway_min_amount"] != float64(services.MinGatewayDeposit) || body["gateway_max_amount"] != float64(services.MaxGatewayDeposit) {
		t.Fatalf("unexpected limits %v", body)
	}
}

func TestDepositInfoHidesGatewayWhenRouterIsNotConfigured(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	srv, sender := newGatewayTestServer(t, db, &stubRouter{configured: false})
	cookie := registerAndVerify(t, srv, sender, "alice", "0911223344", "password123")
	enableGatewayDeposits(t, db, true)

	req := httptest.NewRequest(http.MethodGet, "/api/wallet/deposit-info", nil)
	req.Header.Set("Cookie", cookie)
	resp, _ := srv.App.Test(req)
	body := decodeJSON(t, resp)

	methods, _ := body["methods"].([]any)
	if len(methods) != 1 || methods[0] != "receipt" {
		t.Fatalf("methods = %v, want [receipt]", body["methods"])
	}
}

func TestPlainDepositIsRefusedWhenGatewayIsOn(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	srv, sender := newGatewayTestServer(t, db, &stubRouter{configured: true})
	cookie := registerAndVerify(t, srv, sender, "alice", "0911223344", "password123")
	enableGatewayDeposits(t, db, false)

	resp := postJSON(t, srv, "/api/wallet/deposit", map[string]any{"amount": 500}, cookie)
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("play-money deposit: expected 403, got %d", resp.StatusCode)
	}
}

func TestGatewayDepositFlowCreditsOnSignedCallback(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	router := &stubRouter{configured: true}
	srv, sender := newGatewayTestServer(t, db, router)
	cookie := registerAndVerify(t, srv, sender, "alice", "0911223344", "password123")
	enableGatewayDeposits(t, db, false)

	resp := postJSON(t, srv, "/api/wallet/deposit/gateway", map[string]any{"amount": 250}, cookie)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("start: expected 200, got %d", resp.StatusCode)
	}
	started := decodeJSON(t, resp)
	reference, _ := started["reference"].(string)
	if reference == "" || started["checkout_url"] != "https://checkout.test/"+reference {
		t.Fatalf("unexpected start response %v", started)
	}
	if router.created[0].ReturnURL != "https://poker.test/wallet?deposit="+reference {
		t.Fatalf("unexpected return url %s", router.created[0].ReturnURL)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/wallet/deposit/gateway/"+reference, nil)
	req.Header.Set("Cookie", cookie)
	resp, _ = srv.App.Test(req)
	if status := decodeJSON(t, resp)["status"]; status != services.GatewayDepositStatusPending {
		t.Fatalf("status before callback = %v", status)
	}

	callback := map[string]any{
		"id": "whd_1", "event": services.RouterEventSucceeded, "attempt": 1,
		"data": map[string]any{"id": "pay_" + reference, "reference": reference, "status": "succeeded", "amount": "250.00"},
	}
	resp = signedCallback(t, srv, testWebhookSecret, callback)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("callback: expected 200, got %d", resp.StatusCode)
	}

	req = httptest.NewRequest(http.MethodGet, "/api/wallet/deposit/gateway/"+reference, nil)
	req.Header.Set("Cookie", cookie)
	resp, _ = srv.App.Test(req)
	after := decodeJSON(t, resp)
	if after["status"] != services.GatewayDepositStatusCredited || after["amount"] != float64(250) {
		t.Fatalf("status after callback = %v", after)
	}

	user, err := srv.Queries.GetUserByUsername(context.Background(), "alice")
	if err != nil {
		t.Fatalf("read user: %v", err)
	}
	if user.Wallet != 250 {
		t.Fatalf("wallet = %d, want 250", user.Wallet)
	}
}

func TestGatewayDepositRejectsBadAmount(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	srv, sender := newGatewayTestServer(t, db, &stubRouter{configured: true})
	cookie := registerAndVerify(t, srv, sender, "alice", "0911223344", "password123")
	enableGatewayDeposits(t, db, false)

	resp := postJSON(t, srv, "/api/wallet/deposit/gateway", map[string]any{"amount": 5}, cookie)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
}

func TestGatewayDepositRequiresToggle(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	srv, sender := newGatewayTestServer(t, db, &stubRouter{configured: true})
	cookie := registerAndVerify(t, srv, sender, "alice", "0911223344", "password123")

	resp := postJSON(t, srv, "/api/wallet/deposit/gateway", map[string]any{"amount": 50}, cookie)
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", resp.StatusCode)
	}
}

func TestRouterCallbackRejectsBadSignature(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	srv, _ := newGatewayTestServer(t, db, &stubRouter{configured: true})

	resp := signedCallback(t, srv, "whsec_wrong", map[string]any{"event": services.RouterEventSucceeded, "data": map[string]any{"reference": "GPX"}})
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", resp.StatusCode)
	}
}

func TestRouterCallbackAcknowledgesTestAndRejectsMalformed(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	srv, _ := newGatewayTestServer(t, db, &stubRouter{configured: true})

	resp := signedCallback(t, srv, testWebhookSecret, map[string]any{"event": services.RouterEventTest, "data": map[string]any{"object": "test"}})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("test event: expected 200, got %d", resp.StatusCode)
	}
	resp = signedCallback(t, srv, testWebhookSecret, map[string]any{"event": services.RouterEventSucceeded})
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("missing data: expected 400, got %d", resp.StatusCode)
	}
}
