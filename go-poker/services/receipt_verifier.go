package services

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"
)

// defaultVerifierEndpoint resolves a bank receipt link to the transfer behind
// it. Overridable through RECEIPT_VERIFIER_URL.
const defaultVerifierEndpoint = "https://vericall.ethiodeploy.com/v1/receipts"

// Receipt is the part of the verifier's answer this app acts on. The rest of
// the payload (charges, the bank's raw record) is deliberately not modelled:
// it would grow a struct nothing reads, and the fields that decide whether
// money moves are all here.
type Receipt struct {
	Provider  string `json:"provider"`
	SourceURL string `json:"source_url"`
	Reference string `json:"reference"`
	Status    string `json:"status"`
	Currency  string `json:"currency"`
	Amount    float64
	Payer     ReceiptParty `json:"payer"`
	Receiver  ReceiptParty `json:"receiver"`
	PaidAt    string       `json:"paid_at"`
}

type ReceiptParty struct {
	Name    string `json:"name"`
	Account string `json:"account"`
}

// UnmarshalJSON exists for Amount alone. The verifier sends it as a JSON
// number, but a bank amount arriving as a string ("50.00") is common enough
// across providers that failing the whole deposit over it is not worth it.
func (r *Receipt) UnmarshalJSON(data []byte) error {
	type alias Receipt
	aux := struct {
		Amount json.RawMessage `json:"amount"`
		*alias
	}{alias: (*alias)(r)}
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}
	if len(aux.Amount) == 0 || string(aux.Amount) == "null" {
		return nil
	}
	if err := json.Unmarshal(aux.Amount, &r.Amount); err == nil {
		return nil
	}
	var asString string
	if err := json.Unmarshal(aux.Amount, &asString); err != nil {
		return fmt.Errorf("unreadable amount %s", aux.Amount)
	}
	_, err := fmt.Sscanf(strings.TrimSpace(asString), "%f", &r.Amount)
	return err
}

// The two failure modes a caller has to tell apart. ErrReceiptRejected is the
// bank's verdict and will not change on a retry — the deposit is refused.
// ErrVerifierUnavailable is a fault on our side of the bank and says nothing
// about the receipt, so the paste is queued rather than thrown away.
var (
	ErrReceiptRejected     = errors.New("the bank did not recognize that receipt link")
	ErrVerifierUnavailable = errors.New("receipt verification is temporarily unavailable")

	// ErrVerifierBlocked is the third failure mode, and it is neither the
	// bank's nor the verifier's: a CDN in front of the verifier answered
	// instead of the verifier, challenging this server as a bot. It is a kind
	// of unavailable — nothing was learned about the receipt — but it is worth
	// telling apart, because nothing about the receipt, the bank or the
	// verifier app is wrong, and no amount of retrying is the fix. The fix is
	// a rule on the CDN.
	ErrVerifierBlocked = errors.New("the receipt verifier's CDN blocked this server")
)

// cdnBlock is ErrVerifierBlocked with the evidence attached. It reports itself
// as ErrVerifierUnavailable too, so callers that only care whether the receipt
// was judged keep working unchanged — a blocked call queues the receipt for
// review exactly like an outage does.
type cdnBlock struct {
	trace attemptTrace
}

func (e *cdnBlock) Error() string {
	// The advice goes before the page it came from, because this string is
	// also stored on the deposit row, where it is cut at 255 characters — and
	// what to do about it is worth more of that budget than the CDN's HTML.
	return fmt.Sprintf("%s — %s; allow this server's IP (or a shared header) in the CDN's WAF rules, "+
		"nothing is wrong with the receipt or the verifier app; %s",
		ErrVerifierBlocked, e.trace.headline(), e.trace.bodyDetail())
}

func (e *cdnBlock) Is(target error) bool {
	return target == ErrVerifierBlocked || target == ErrVerifierUnavailable
}

// ReceiptVerifier is the seam between deposits and the third-party verifier,
// so the deposit rules can be tested without a network.
type ReceiptVerifier interface {
	Verify(ctx context.Context, receiptURL string) (*Receipt, error)
}

// Defaults for the retry policy. They are what NewHTTPReceiptVerifier uses
// unless the environment overrides them.
//
// The per-attempt timeout is deliberately generous. The verifier answers a
// receipt it has already seen in well under a second, but the first look at a
// *new* receipt is a cold fetch of CBE's own site, and that is the only kind
// of receipt a player ever pastes: every real deposit pays the slow path. A
// tight timeout there does not fail fast, it just turns a slow success into a
// queued receipt and a "we couldn't reach the bank" message on a verifier that
// is working perfectly.
const (
	defaultVerifierAttempts       = 3
	defaultVerifierAttemptTimeout = 30 * time.Second
	defaultVerifierBackoff        = time.Second
	defaultVerifierUserAgent      = "golden-poker-deposits/1.0"
	defaultVerifierTokenHeader    = "X-Api-Key"
)

// HTTPReceiptVerifier calls the verifier over HTTP, retrying the failures that
// a retry can fix.
type HTTPReceiptVerifier struct {
	Endpoint string
	Client   *http.Client
	// Attempts counts total tries, not retries.
	Attempts int
	// AttemptTimeout bounds one attempt. It is enforced with a context rather
	// than http.Client.Timeout so that Budget can add the attempts up, and so
	// a caller's own deadline and this one cannot silently disagree about
	// which is in charge.
	AttemptTimeout time.Duration
	// Backoff is the pause after the first failed attempt; later pauses grow
	// with the attempt number.
	Backoff time.Duration
	// UserAgent names this app to the verifier and to anything in front of it.
	UserAgent string
	// Token and TokenHeader are for getting past a CDN, not for authenticating
	// to the verifier. Set them to whatever a WAF "skip" rule matches on.
	Token       string
	TokenHeader string
}

func NewHTTPReceiptVerifier() *HTTPReceiptVerifier {
	endpoint := strings.TrimSpace(os.Getenv("RECEIPT_VERIFIER_URL"))
	if endpoint == "" {
		endpoint = defaultVerifierEndpoint
	}
	return &HTTPReceiptVerifier{
		Endpoint: endpoint,
		// No Client.Timeout: each attempt carries its own deadline. The
		// transport limits are the parts of a call that are our side of the
		// wire — connect and TLS — and they stay short, because a stall there
		// is worth abandoning early rather than spending the attempt's whole
		// budget on.
		Client: &http.Client{
			Transport: &http.Transport{
				Proxy:                 http.ProxyFromEnvironment,
				DialContext:           (&net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
				TLSHandshakeTimeout:   10 * time.Second,
				MaxIdleConnsPerHost:   4,
				IdleConnTimeout:       90 * time.Second,
				ExpectContinueTimeout: time.Second,
			},
		},
		Attempts:       envInt("RECEIPT_VERIFIER_ATTEMPTS", defaultVerifierAttempts),
		AttemptTimeout: envDuration("RECEIPT_VERIFIER_TIMEOUT", defaultVerifierAttemptTimeout),
		Backoff:        envDuration("RECEIPT_VERIFIER_BACKOFF", defaultVerifierBackoff),
		UserAgent:      strings.TrimSpace(os.Getenv("RECEIPT_VERIFIER_USER_AGENT")),
		Token:          strings.TrimSpace(os.Getenv("RECEIPT_VERIFIER_TOKEN")),
		TokenHeader:    strings.TrimSpace(os.Getenv("RECEIPT_VERIFIER_TOKEN_HEADER")),
	}
}

func (v *HTTPReceiptVerifier) userAgent() string {
	if ua := strings.TrimSpace(v.UserAgent); ua != "" {
		return ua
	}
	return defaultVerifierUserAgent
}

func (v *HTTPReceiptVerifier) tokenHeader() string {
	if h := strings.TrimSpace(v.TokenHeader); h != "" {
		return h
	}
	return defaultVerifierTokenHeader
}

func (v *HTTPReceiptVerifier) attemptCount() int {
	if v.Attempts < 1 {
		return 1
	}
	return v.Attempts
}

func (v *HTTPReceiptVerifier) attemptTimeout() time.Duration {
	if v.AttemptTimeout <= 0 {
		return defaultVerifierAttemptTimeout
	}
	return v.AttemptTimeout
}

// backoffBefore is the pause before the given attempt (0-based), growing so a
// verifier that is genuinely struggling is not hit three times in three
// seconds.
func (v *HTTPReceiptVerifier) backoffBefore(attempt int) time.Duration {
	if v.Backoff <= 0 || attempt < 1 {
		return 0
	}
	return v.Backoff * time.Duration(attempt)
}

// Budget is how long a full Verify can take: every attempt plus every pause
// between them. Callers use it to set their own deadline, because a deadline
// picked independently is a deadline that cuts the last attempt in half — the
// caller gives up mid-request and the retry it paid for never happens.
func (v *HTTPReceiptVerifier) Budget() time.Duration {
	total := time.Duration(v.attemptCount()) * v.attemptTimeout()
	for attempt := 1; attempt < v.attemptCount(); attempt++ {
		total += v.backoffBefore(attempt)
	}
	return total
}

func envInt(key string, fallback int) int {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < 1 {
		log.Printf("receipt verifier: ignoring %s=%q: %v", key, raw, err)
		return fallback
	}
	return n
}

func envDuration(key string, fallback time.Duration) time.Duration {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback
	}
	d, err := time.ParseDuration(raw)
	if err != nil || d <= 0 {
		log.Printf("receipt verifier: ignoring %s=%q: %v", key, raw, err)
		return fallback
	}
	return d
}

func (v *HTTPReceiptVerifier) Verify(ctx context.Context, receiptURL string) (*Receipt, error) {
	attempts := v.attemptCount()
	started := time.Now()

	var lastErr error
	for attempt := 0; attempt < attempts; attempt++ {
		if pause := v.backoffBefore(attempt); pause > 0 {
			select {
			case <-ctx.Done():
				return nil, fmt.Errorf("%w: %v", ErrVerifierUnavailable, ctx.Err())
			case <-time.After(pause):
			}
		}
		// The caller's deadline can be shorter than the budget this policy
		// asks for. Starting an attempt with nothing left to spend only
		// produces a cancelled request and a misleading error, so stop and
		// report what already went wrong.
		if err := ctx.Err(); err != nil {
			log.Printf("receipt verifier: giving up on %s after %d attempt(s) in %s: caller out of time (%v)",
				receiptURL, attempt, time.Since(started).Round(time.Millisecond), err)
			if lastErr == nil {
				lastErr = err
			}
			break
		}

		receipt, err := v.attemptWithTimeout(ctx, receiptURL)
		if err == nil {
			log.Printf("receipt verifier: %s verified on attempt %d of %d in %s (reference %s, %.2f %s)",
				receiptURL, attempt+1, attempts, time.Since(started).Round(time.Millisecond),
				receipt.Reference, receipt.Amount, receipt.Currency)
			return receipt, nil
		}
		// A verdict is a verdict. Retrying a receipt the bank has already
		// rejected only makes the user wait longer for the same answer.
		if errors.Is(err, ErrReceiptRejected) {
			log.Printf("receipt verifier: %s refused on attempt %d of %d in %s: %v",
				receiptURL, attempt+1, attempts, time.Since(started).Round(time.Millisecond), err)
			return nil, err
		}
		lastErr = err
		log.Printf("receipt verifier: attempt %d of %d for %s failed: %v",
			attempt+1, attempts, receiptURL, err)

		// A CDN challenge does not clear in a second, so the remaining
		// attempts would only make the player wait longer for the same page.
		// It still queues the receipt, and the queue sweep retries it minutes
		// later — by which time a WAF rule may actually have been added.
		if errors.Is(err, ErrVerifierBlocked) {
			log.Printf("receipt verifier: not retrying — the call never reached the verifier")
			break
		}
	}
	log.Printf("receipt verifier: %s unresolved after %s; queueing for review",
		receiptURL, time.Since(started).Round(time.Millisecond))
	if errors.Is(lastErr, ErrVerifierBlocked) {
		return nil, lastErr
	}
	return nil, fmt.Errorf("%w: %v", ErrVerifierUnavailable, lastErr)
}

// attemptWithTimeout gives one attempt its own deadline, capped by whatever
// the caller has left.
func (v *HTTPReceiptVerifier) attemptWithTimeout(ctx context.Context, receiptURL string) (*Receipt, error) {
	attemptCtx, cancel := context.WithTimeout(ctx, v.attemptTimeout())
	defer cancel()
	return v.attempt(attemptCtx, receiptURL)
}

func (v *HTTPReceiptVerifier) attempt(ctx context.Context, receiptURL string) (*Receipt, error) {
	endpoint := v.Endpoint + "?url=" + url.QueryEscape(receiptURL)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	// An explicit User-Agent, because Go's default ("Go-http-client/1.1") is
	// one of the most-filtered signatures on the internet and says nothing
	// about who is calling. Naming ourselves makes this app findable in the
	// verifier's own logs.
	req.Header.Set("User-Agent", v.userAgent())
	// The token is not authentication as far as this app is concerned — the
	// verifier needs none. It exists so a CDN in front of the verifier can be
	// given a rule that lets this server through on something better than its
	// IP address.
	if token := strings.TrimSpace(v.Token); token != "" {
		req.Header.Set(v.tokenHeader(), token)
	}

	started := time.Now()
	resp, err := v.Client.Do(req)
	if err != nil {
		return nil, &transportError{elapsed: time.Since(started), endpoint: v.Endpoint, err: err}
	}
	defer resp.Body.Close()

	body, readErr := io.ReadAll(io.LimitReader(resp.Body, 256<<10))
	trace := traceOf(resp, body, time.Since(started))
	if readErr != nil {
		return nil, fmt.Errorf("reading verifier response failed after %s: %w", trace.elapsed.Round(time.Millisecond), readErr)
	}

	switch {
	case resp.StatusCode == http.StatusOK:
	case looksLikeCDNChallenge(trace):
		// Whatever answered, it was not the verifier. Reported before the
		// status codes below so a challenge served as 403 is never mistaken
		// for the bank refusing a receipt.
		return nil, &cdnBlock{trace: trace}
	case resp.StatusCode == http.StatusNotFound, resp.StatusCode == http.StatusUnprocessableEntity,
		resp.StatusCode == http.StatusBadRequest:
		// The bank was reached and said no. 422 lands here too: the verifier
		// returns it for a URL it cannot parse, which no retry improves.
		return nil, fmt.Errorf("%w: %s", ErrReceiptRejected, verifierErrorDetail(body))
	default:
		// 5xx, 429, a proxy error page — the receipt is unjudged.
		return nil, fmt.Errorf("verifier refused the call: %s", trace)
	}

	var receipt Receipt
	if err := json.Unmarshal(body, &receipt); err != nil {
		return nil, fmt.Errorf("unreadable verifier response: %s", trace)
	}
	if receipt.Reference == "" {
		// Without a reference there is nothing to enforce uniqueness on, and a
		// deposit that cannot be deduplicated must not be credited.
		return nil, fmt.Errorf("%w: the receipt carries no transaction reference", ErrReceiptRejected)
	}
	return &receipt, nil
}

// attemptTrace is what one call to the verifier looked like from here. It is
// built for the log line and the note stored on a queued deposit: when a
// receipt is queued, this is the only account of why that anyone will ever
// have — the call cannot be reproduced later, because by then it works.
type attemptTrace struct {
	elapsed     time.Duration
	status      int
	server      string
	cfRay       string
	cfMitigated string
	contentType string
	retryAfter  string
	body        string
}

func traceOf(resp *http.Response, body []byte, elapsed time.Duration) attemptTrace {
	return attemptTrace{
		elapsed:     elapsed,
		status:      resp.StatusCode,
		server:      resp.Header.Get("Server"),
		cfRay:       resp.Header.Get("Cf-Ray"),
		cfMitigated: resp.Header.Get("Cf-Mitigated"),
		contentType: resp.Header.Get("Content-Type"),
		retryAfter:  resp.Header.Get("Retry-After"),
		body:        truncate(collapseWhitespace(string(body)), 200),
	}
}

func (t attemptTrace) String() string {
	if body := t.bodyDetail(); body != "" {
		return t.headline() + "; " + body
	}
	return t.headline()
}

func (t attemptTrace) bodyDetail() string {
	if t.body == "" {
		return ""
	}
	return "answer was: " + t.body
}

// headline is the trace without the response body: the facts that identify the
// call, short enough to survive being stored on a deposit row.
func (t attemptTrace) headline() string {
	parts := []string{
		fmt.Sprintf("http %d", t.status),
		fmt.Sprintf("in %s", t.elapsed.Round(time.Millisecond)),
	}
	// cf-ray is the one field worth carrying everywhere: it is what identifies
	// this exact request in the CDN's own security log, which is where the
	// rule that blocked it can be found.
	for label, value := range map[string]string{
		"server": t.server, "cf-ray": t.cfRay, "cf-mitigated": t.cfMitigated,
		"content-type": t.contentType, "retry-after": t.retryAfter,
	} {
		if value != "" {
			parts = append(parts, label+"="+value)
		}
	}
	sort.Strings(parts[2:])
	return strings.Join(parts, " ")
}

// transportError is a call that never got an answer at all — DNS, connect,
// TLS, or the attempt deadline running out mid-request. Separated from an
// answered-but-unhappy call because the two are fixed in completely different
// places, and the raw error alone does not say which happened.
type transportError struct {
	elapsed  time.Duration
	endpoint string
	err      error
}

func (e *transportError) Error() string {
	return fmt.Sprintf("no answer from %s after %s: %v", e.endpoint, e.elapsed.Round(time.Millisecond), e.err)
}

func (e *transportError) Unwrap() error { return e.err }

// looksLikeCDNChallenge recognizes an interstitial served in place of the API.
// The giveaway is not the status code — a challenge is a 403, a 503 or a 429
// depending on the product — but that an endpoint which only ever speaks JSON
// answered with an HTML page telling a browser to wait.
func looksLikeCDNChallenge(t attemptTrace) bool {
	if t.status == http.StatusOK {
		return false
	}
	if strings.Contains(strings.ToLower(t.cfMitigated), "challenge") {
		return true
	}
	if !strings.Contains(strings.ToLower(t.contentType), "html") && !strings.Contains(t.body, "<html") {
		return false
	}
	haystack := strings.ToLower(t.body)
	for _, marker := range []string{
		"just a moment",
		"challenge-platform",
		"cf-browser-verification",
		"__cf_chl",
		"enable javascript and cookies to continue",
		"attention required",
		"checking your browser",
		"ddos protection by",
	} {
		if strings.Contains(haystack, marker) {
			return true
		}
	}
	// An HTML error page from a CDN that is not the app itself still means the
	// verifier never saw the call.
	return strings.Contains(strings.ToLower(t.server), "cloudflare")
}

func collapseWhitespace(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

// verifierErrorDetail digs the human-readable line out of the verifier's error
// body, which is either {"error":{"message":...}} or FastAPI's
// {"detail":[{"msg":...}]}.
func verifierErrorDetail(body []byte) string {
	var wrapped struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
		Detail json.RawMessage `json:"detail"`
	}
	if err := json.Unmarshal(body, &wrapped); err != nil {
		return truncate(string(body), 200)
	}
	if wrapped.Error.Message != "" {
		return wrapped.Error.Message
	}
	var details []struct {
		Msg string `json:"msg"`
	}
	if err := json.Unmarshal(wrapped.Detail, &details); err == nil && len(details) > 0 {
		return details[0].Msg
	}
	return truncate(string(body), 200)
}

func truncate(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
