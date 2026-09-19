package services

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

const (
	defaultPaymentRouterURL     = "https://router.razielcc.com"
	defaultPaymentRouterTimeout = 15 * time.Second
	paymentRouterMaxBody        = 1 << 20
)

var ErrPaymentRouterUnavailable = errors.New("the payment service is temporarily unavailable")

type PaymentRouterError struct {
	Status  int
	Code    string
	Message string
}

func (e *PaymentRouterError) Error() string {
	if e.Message != "" {
		return e.Message
	}
	return fmt.Sprintf("payment router returned %d", e.Status)
}

type RouterPayment struct {
	ID          string `json:"id"`
	Status      string `json:"status"`
	CheckoutURL string `json:"checkout_url"`
	Reference   string `json:"reference"`
	Amount      string `json:"amount"`
	Currency    string `json:"currency"`
}

type RouterCreatePayment struct {
	Reference   string
	AmountETB   int64
	ReturnURL   string
	Description string
	Phone       string
	Name        string
	ExpiresIn   int
	Metadata    map[string]any
}

type PaymentRouter interface {
	Configured() bool
	WebhookSecret() string
	CreatePayment(ctx context.Context, req RouterCreatePayment) (*RouterPayment, error)
	FetchPayment(ctx context.Context, paymentID string) (*RouterPayment, error)
}

type HTTPPaymentRouter struct {
	BaseURL       string
	APIKey        string
	webhookSecret string
	Client        *http.Client
}

func NewPaymentRouterFromEnv() *HTTPPaymentRouter {
	baseURL := strings.TrimRight(strings.TrimSpace(os.Getenv("PAYMENT_ROUTER_URL")), "/")
	if baseURL == "" {
		baseURL = defaultPaymentRouterURL
	}
	timeout := defaultPaymentRouterTimeout
	if raw := strings.TrimSpace(os.Getenv("PAYMENT_ROUTER_TIMEOUT")); raw != "" {
		if parsed, err := time.ParseDuration(raw); err == nil && parsed > 0 {
			timeout = parsed
		}
	}
	return &HTTPPaymentRouter{
		BaseURL:       baseURL,
		APIKey:        strings.TrimSpace(os.Getenv("PAYMENT_ROUTER_API_KEY")),
		webhookSecret: strings.TrimSpace(os.Getenv("PAYMENT_ROUTER_WEBHOOK_SECRET")),
		Client:        &http.Client{Timeout: timeout},
	}
}

func NewHTTPPaymentRouter(baseURL, apiKey, webhookSecret string, client *http.Client) *HTTPPaymentRouter {
	if client == nil {
		client = &http.Client{Timeout: defaultPaymentRouterTimeout}
	}
	return &HTTPPaymentRouter{
		BaseURL:       strings.TrimRight(baseURL, "/"),
		APIKey:        apiKey,
		webhookSecret: webhookSecret,
		Client:        client,
	}
}

func (r *HTTPPaymentRouter) Configured() bool {
	return r != nil && r.APIKey != "" && r.webhookSecret != "" && r.BaseURL != ""
}

func (r *HTTPPaymentRouter) WebhookSecret() string {
	if r == nil {
		return ""
	}
	return r.webhookSecret
}

func (r *HTTPPaymentRouter) CreatePayment(ctx context.Context, req RouterCreatePayment) (*RouterPayment, error) {
	if !r.Configured() {
		return nil, ErrPaymentRouterUnavailable
	}
	payload := map[string]any{
		"amount":     fmt.Sprintf("%d.00", req.AmountETB),
		"currency":   "ETB",
		"reference":  req.Reference,
		"return_url": req.ReturnURL,
	}
	if req.Description != "" {
		payload["description"] = req.Description
	}
	if req.ExpiresIn > 0 {
		payload["expires_in"] = req.ExpiresIn
	}
	if req.Phone != "" {
		customer := map[string]any{"phone": req.Phone, "phone_updatable": false}
		if req.Name != "" {
			customer["name"] = req.Name
		}
		payload["customer"] = customer
	}
	if len(req.Metadata) > 0 {
		payload["metadata"] = req.Metadata
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, r.BaseURL+"/v1/payments", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Idempotency-Key", req.Reference)
	return r.do(httpReq)
}

func (r *HTTPPaymentRouter) FetchPayment(ctx context.Context, paymentID string) (*RouterPayment, error) {
	if !r.Configured() {
		return nil, ErrPaymentRouterUnavailable
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, r.BaseURL+"/v1/payments/"+paymentID, nil)
	if err != nil {
		return nil, err
	}
	return r.do(httpReq)
}

func (r *HTTPPaymentRouter) do(req *http.Request) (*RouterPayment, error) {
	req.Header.Set("Authorization", "Bearer "+r.APIKey)
	req.Header.Set("Accept", "application/json")

	resp, err := r.Client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrPaymentRouterUnavailable, err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(io.LimitReader(resp.Body, paymentRouterMaxBody))
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrPaymentRouterUnavailable, err)
	}

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return nil, routerError(resp.StatusCode, raw)
	}

	var payment RouterPayment
	if err := json.Unmarshal(raw, &payment); err != nil {
		return nil, fmt.Errorf("%w: unreadable response", ErrPaymentRouterUnavailable)
	}
	if payment.ID == "" {
		return nil, fmt.Errorf("%w: response without a payment id", ErrPaymentRouterUnavailable)
	}
	return &payment, nil
}

func routerError(status int, raw []byte) error {
	var envelope struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	_ = json.Unmarshal(raw, &envelope)
	if status >= 500 && envelope.Error.Code == "" {
		return fmt.Errorf("%w: status %d", ErrPaymentRouterUnavailable, status)
	}
	return &PaymentRouterError{Status: status, Code: envelope.Error.Code, Message: envelope.Error.Message}
}
