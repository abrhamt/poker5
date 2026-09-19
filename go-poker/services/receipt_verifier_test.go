package services

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// The verifier's real answer for the sample receipt, trimmed to the fields
// this app reads.
const verifierOKBody = `{"provider":"cbe","provider_name":"Commercial Bank of Ethiopia",
"source_url":"https://mbreciept.cbe.com.et/v2-hfHCxGvIhvqVMTB7hylL","reference":"FT262325H0PP",
"status":"completed","status_label":"COMPLETED","currency":"ETB","amount":50.0,"total_debited":50.61,
"payer":{"name":"Yaikob Demissie Jarso","account":"1********3928"},
"receiver":{"name":"Yanet Joni And Nigist Ariya","account":"1********1758"},
"paid_at":"2026-08-20T15:56:00Z","channel":"ANDROID"}`

func testVerifier(endpoint string, client *http.Client) *HTTPReceiptVerifier {
	return &HTTPReceiptVerifier{Endpoint: endpoint, Client: client, Attempts: 3, Backoff: 0}
}

func TestReceiptVerifierParsesResponse(t *testing.T) {
	var gotURL string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotURL = r.URL.Query().Get("url")
		_, _ = w.Write([]byte(verifierOKBody))
	}))
	defer srv.Close()

	receipt, err := testVerifier(srv.URL, srv.Client()).
		Verify(context.Background(), "https://mbreciept.cbe.com.et/v2-hfHCxGvIhvqVMTB7hylL")
	if err != nil {
		t.Fatalf("verify: %v", err)
	}

	if gotURL != "https://mbreciept.cbe.com.et/v2-hfHCxGvIhvqVMTB7hylL" {
		t.Errorf("url param = %q", gotURL)
	}
	if receipt.Reference != "FT262325H0PP" {
		t.Errorf("reference = %q", receipt.Reference)
	}
	if receipt.Amount != 50 {
		t.Errorf("amount = %v, want 50", receipt.Amount)
	}
	if receipt.Receiver.Account != "1********1758" {
		t.Errorf("receiver account = %q", receipt.Receiver.Account)
	}
	if receipt.Status != "completed" {
		t.Errorf("status = %q", receipt.Status)
	}
}

// A bank amount arriving as a string is common enough across providers that it
// must not fail the whole deposit.
func TestReceiptVerifierAcceptsStringAmount(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"reference":"FT1","status":"completed","currency":"ETB","amount":"50.00"}`))
	}))
	defer srv.Close()

	receipt, err := testVerifier(srv.URL, srv.Client()).Verify(context.Background(), "https://mbreciept.cbe.com.et/v2-x")
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if receipt.Amount != 50 {
		t.Fatalf("amount = %v, want 50", receipt.Amount)
	}
}

// The bank was reached and said no. Retrying only makes the user wait longer
// for the same answer.
func TestReceiptVerifierDoesNotRetryARejection(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls++
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":{"code":"receipt_not_found","message":"CBE rejected this receipt link."}}`))
	}))
	defer srv.Close()

	_, err := testVerifier(srv.URL, srv.Client()).Verify(context.Background(), "https://mbreciept.cbe.com.et/v2-bogus")
	if !errors.Is(err, ErrReceiptRejected) {
		t.Fatalf("err = %v, want ErrReceiptRejected", err)
	}
	if calls != 1 {
		t.Fatalf("called %d times, want 1", calls)
	}
	if !strings.Contains(err.Error(), "CBE rejected this receipt link.") {
		t.Errorf("err = %q, want the provider's message carried through", err)
	}
}

// A 5xx says nothing about the receipt, so it is retried and then reported as
// unavailable — which is what puts the paste in the review queue instead of
// throwing it away.
func TestReceiptVerifierRetriesThenReportsUnavailable(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls++
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer srv.Close()

	_, err := testVerifier(srv.URL, srv.Client()).Verify(context.Background(), "https://mbreciept.cbe.com.et/v2-x")
	if !errors.Is(err, ErrVerifierUnavailable) {
		t.Fatalf("err = %v, want ErrVerifierUnavailable", err)
	}
	if calls != 3 {
		t.Fatalf("called %d times, want 3", calls)
	}
}

func TestReceiptVerifierRecoversOnRetry(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls++
		if calls == 1 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		_, _ = w.Write([]byte(verifierOKBody))
	}))
	defer srv.Close()

	receipt, err := testVerifier(srv.URL, srv.Client()).Verify(context.Background(), "https://mbreciept.cbe.com.et/v2-x")
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if receipt.Reference != "FT262325H0PP" {
		t.Fatalf("reference = %q", receipt.Reference)
	}
}

// Without a reference there is nothing to enforce uniqueness on, and a deposit
// that cannot be deduplicated must not be credited.
func TestReceiptVerifierRejectsResponseWithoutReference(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"status":"completed","currency":"ETB","amount":50.0}`))
	}))
	defer srv.Close()

	_, err := testVerifier(srv.URL, srv.Client()).Verify(context.Background(), "https://mbreciept.cbe.com.et/v2-x")
	if !errors.Is(err, ErrReceiptRejected) {
		t.Fatalf("err = %v, want ErrReceiptRejected", err)
	}
}

// A cold receipt is slow to fetch — that is the normal case, not a fault — so
// the per-attempt deadline has to be what governs, and a slow first answer
// must not be reported as an unreachable verifier.
func TestReceiptVerifierTimesOutPerAttemptAndRetries(t *testing.T) {
	var calls int
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls == 1 {
			// Hang past this attempt's deadline, the way a cold CBE fetch does.
			select {
			case <-release:
			case <-r.Context().Done():
			}
			return
		}
		_, _ = w.Write([]byte(verifierOKBody))
	}))
	defer srv.Close()
	defer close(release)

	v := &HTTPReceiptVerifier{
		Endpoint:       srv.URL,
		Client:         srv.Client(),
		Attempts:       3,
		AttemptTimeout: 100 * time.Millisecond,
		Backoff:        time.Millisecond,
	}

	receipt, err := v.Verify(context.Background(), "https://mbreciept.cbe.com.et/v2-hfHCxGvIhvqVMTB7hylL")
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if receipt.Reference != "FT262325H0PP" {
		t.Fatalf("reference = %q", receipt.Reference)
	}
	if calls != 2 {
		t.Fatalf("calls = %d, want 2 — the timed-out attempt should be retried", calls)
	}
}

// The budget is the caller's contract: set a deadline shorter than this and
// the last attempt gets cancelled half-way, which is the retry the user waited
// for and never got.
func TestReceiptVerifierBudgetCoversEveryAttempt(t *testing.T) {
	v := &HTTPReceiptVerifier{Attempts: 3, AttemptTimeout: 30 * time.Second, Backoff: time.Second}
	// 3 attempts plus pauses of 1s and 2s.
	if got, want := v.Budget(), 93*time.Second; got != want {
		t.Fatalf("budget = %s, want %s", got, want)
	}

	v = &HTTPReceiptVerifier{Attempts: 1, AttemptTimeout: 10 * time.Second, Backoff: time.Second}
	if got, want := v.Budget(), 10*time.Second; got != want {
		t.Fatalf("single-attempt budget = %s, want %s", got, want)
	}
}

// A caller whose own deadline is already spent gets an answer instead of a
// second and third attempt that cannot possibly run.
func TestReceiptVerifierStopsWhenTheCallerIsOutOfTime(t *testing.T) {
	var calls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer srv.Close()

	v := &HTTPReceiptVerifier{
		Endpoint:       srv.URL,
		Client:         srv.Client(),
		Attempts:       5,
		AttemptTimeout: time.Second,
		Backoff:        50 * time.Millisecond,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 80*time.Millisecond)
	defer cancel()

	if _, err := v.Verify(ctx, "https://mbreciept.cbe.com.et/v2-hfHCxGvIhvqVMTB7hylL"); !errors.Is(err, ErrVerifierUnavailable) {
		t.Fatalf("err = %v, want ErrVerifierUnavailable", err)
	}
	if calls >= 5 {
		t.Fatalf("calls = %d — attempts kept running past the caller's deadline", calls)
	}
}

// The Cloudflare interstitial that started all this: a 403 carrying an HTML
// "Just a moment..." page. It must not be read as the bank refusing the
// receipt — that would reject a real deposit — and it must not be retried,
// because a challenge does not clear in a second.
func TestReceiptVerifierRecognizesACDNChallenge(t *testing.T) {
	var calls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.Header().Set("Server", "cloudflare")
		w.Header().Set("Cf-Ray", "a2f996efc81ce6b4-ADD")
		w.Header().Set("Content-Type", "text/html; charset=UTF-8")
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`<!DOCTYPE html><html lang="en-US"><head><title>Just a moment...</title>`))
	}))
	defer srv.Close()

	v := &HTTPReceiptVerifier{Endpoint: srv.URL, Client: srv.Client(), Attempts: 3, Backoff: time.Millisecond}
	_, err := v.Verify(context.Background(), "https://mbreciept.cbe.com.et/v2-hfHCxGvIhvqVMTB7hylL")

	if !errors.Is(err, ErrVerifierBlocked) {
		t.Fatalf("err = %v, want ErrVerifierBlocked", err)
	}
	// A blocked call is still an unjudged receipt, so the deposit path has to
	// keep queueing it rather than refusing it.
	if !errors.Is(err, ErrVerifierUnavailable) {
		t.Fatalf("a blocked call must also read as unavailable, got %v", err)
	}
	if errors.Is(err, ErrReceiptRejected) {
		t.Fatalf("a challenge page was mistaken for the bank's verdict: %v", err)
	}
	if calls != 1 {
		t.Fatalf("calls = %d, want 1 — a challenge is not worth retrying", calls)
	}
	// cf-ray is what finds this exact request in the CDN's security log.
	if !strings.Contains(err.Error(), "a2f996efc81ce6b4-ADD") {
		t.Fatalf("error does not carry the cf-ray: %v", err)
	}
}

// A plain JSON 502 from the verifier itself is an outage, not a block, and is
// still worth retrying.
func TestReceiptVerifierTreatsAJSONErrorAsAnOutage(t *testing.T) {
	var calls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte(`{"error":{"message":"upstream bank timed out"}}`))
	}))
	defer srv.Close()

	v := &HTTPReceiptVerifier{Endpoint: srv.URL, Client: srv.Client(), Attempts: 3, Backoff: time.Millisecond}
	_, err := v.Verify(context.Background(), "https://mbreciept.cbe.com.et/v2-hfHCxGvIhvqVMTB7hylL")

	if !errors.Is(err, ErrVerifierUnavailable) || errors.Is(err, ErrVerifierBlocked) {
		t.Fatalf("err = %v, want a plain ErrVerifierUnavailable", err)
	}
	if calls != 3 {
		t.Fatalf("calls = %d, want 3 — an outage is what retries are for", calls)
	}
	if !strings.Contains(err.Error(), "upstream bank timed out") {
		t.Fatalf("error dropped the verifier's own message: %v", err)
	}
}

// The call identifies itself. Go's default User-Agent is a bot signature, and
// the token exists so a WAF rule can match on something better than an IP.
func TestReceiptVerifierIdentifiesItself(t *testing.T) {
	var gotUA, gotKey string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotUA = r.Header.Get("User-Agent")
		gotKey = r.Header.Get("X-Deposit-Key")
		_, _ = w.Write([]byte(verifierOKBody))
	}))
	defer srv.Close()

	v := &HTTPReceiptVerifier{
		Endpoint: srv.URL, Client: srv.Client(), Attempts: 1,
		Token: "s3cret", TokenHeader: "X-Deposit-Key",
	}
	if _, err := v.Verify(context.Background(), "https://mbreciept.cbe.com.et/v2-hfHCxGvIhvqVMTB7hylL"); err != nil {
		t.Fatalf("verify: %v", err)
	}
	if gotUA != defaultVerifierUserAgent {
		t.Fatalf("user-agent = %q, want %q", gotUA, defaultVerifierUserAgent)
	}
	if gotKey != "s3cret" {
		t.Fatalf("token header = %q, want the configured token", gotKey)
	}
}
