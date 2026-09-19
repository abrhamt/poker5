package services

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCreatePaymentSendsCallerPayload(t *testing.T) {
	var got struct {
		auth string
		idem string
		path string
		body map[string]any
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got.auth = r.Header.Get("Authorization")
		got.idem = r.Header.Get("Idempotency-Key")
		got.path = r.URL.Path
		_ = json.NewDecoder(r.Body).Decode(&got.body)
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id": "pay_1", "status": "pending", "checkout_url": "https://checkout.test/1",
			"reference": "GP1", "amount": "150.00", "currency": "ETB",
		})
	}))
	defer server.Close()

	router := NewHTTPPaymentRouter(server.URL, "prk_test", "whsec_test", server.Client())
	payment, err := router.CreatePayment(context.Background(), RouterCreatePayment{
		Reference:   "GP1",
		AmountETB:   150,
		ReturnURL:   "https://poker.test/wallet?deposit=GP1",
		Description: "Wallet deposit",
		Phone:       "+251911223344",
		Name:        "alice",
		ExpiresIn:   3600,
		Metadata:    map[string]any{"user_id": 7},
	})
	if err != nil {
		t.Fatalf("create payment: %v", err)
	}

	if payment.ID != "pay_1" || payment.CheckoutURL != "https://checkout.test/1" {
		t.Fatalf("unexpected payment %+v", payment)
	}
	if got.auth != "Bearer prk_test" || got.idem != "GP1" || got.path != "/v1/payments" {
		t.Fatalf("unexpected request auth=%q idem=%q path=%q", got.auth, got.idem, got.path)
	}
	if got.body["amount"] != "150.00" || got.body["currency"] != "ETB" || got.body["reference"] != "GP1" {
		t.Fatalf("unexpected body %v", got.body)
	}
	customer, _ := got.body["customer"].(map[string]any)
	if customer["phone"] != "+251911223344" || customer["name"] != "alice" || customer["phone_updatable"] != false {
		t.Fatalf("unexpected customer %v", customer)
	}
	if got.body["expires_in"] != float64(3600) {
		t.Fatalf("unexpected expires_in %v", got.body["expires_in"])
	}
}

func TestCreatePaymentSurfacesRouterErrorCode(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte(`{"error":{"code":"gateway_error","message":"FenanPay rejected"}}`))
	}))
	defer server.Close()

	router := NewHTTPPaymentRouter(server.URL, "prk_test", "whsec_test", server.Client())
	_, err := router.CreatePayment(context.Background(), RouterCreatePayment{Reference: "GP2", AmountETB: 10, ReturnURL: "https://p.test"})

	var routerErr *PaymentRouterError
	if !errors.As(err, &routerErr) {
		t.Fatalf("expected PaymentRouterError, got %v", err)
	}
	if routerErr.Code != "gateway_error" || routerErr.Message != "FenanPay rejected" {
		t.Fatalf("unexpected error %+v", routerErr)
	}
}

func TestCreatePaymentWrapsTransportFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	url := server.URL
	server.Close()

	router := NewHTTPPaymentRouter(url, "prk_test", "whsec_test", nil)
	_, err := router.CreatePayment(context.Background(), RouterCreatePayment{Reference: "GP3", AmountETB: 10, ReturnURL: "https://p.test"})
	if !errors.Is(err, ErrPaymentRouterUnavailable) {
		t.Fatalf("expected ErrPaymentRouterUnavailable, got %v", err)
	}
}

func TestFetchPaymentReadsStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/payments/pay_9" {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"id": "pay_9", "status": "succeeded", "reference": "GP9", "amount": "20.00"})
	}))
	defer server.Close()

	router := NewHTTPPaymentRouter(server.URL, "prk_test", "whsec_test", server.Client())
	payment, err := router.FetchPayment(context.Background(), "pay_9")
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if payment.Status != "succeeded" || payment.Reference != "GP9" {
		t.Fatalf("unexpected payment %+v", payment)
	}
}

func TestUnconfiguredRouterRefusesToCall(t *testing.T) {
	router := NewHTTPPaymentRouter("https://router.test", "", "", nil)
	if router.Configured() {
		t.Fatal("router without key reported as configured")
	}
	if _, err := router.CreatePayment(context.Background(), RouterCreatePayment{Reference: "x", AmountETB: 10}); !errors.Is(err, ErrPaymentRouterUnavailable) {
		t.Fatalf("expected ErrPaymentRouterUnavailable, got %v", err)
	}
}
