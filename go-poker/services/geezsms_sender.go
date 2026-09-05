package services

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

// geezSMSEndpoint is the provider's send URL. Overridable through
// GEEZSMS_URL so a staging account or a local stub can be pointed at without
// a rebuild.
const geezSMSEndpoint = "https://api.geezsms.com/api/v1/sms/send"

// GeezSMSSender delivers codes through GeezSMS, an Ethiopian SMS gateway.
//
// It implements OTPSender and knows nothing about what it is sending: the
// wording comes from RenderOTPMessage, so changing the copy never touches this
// file and changing providers never touches the copy.
type GeezSMSSender struct {
	Token    string
	Endpoint string
	Client   *http.Client
}

// NewGeezSMSSender builds a sender from GEEZSMS_TOKEN. It returns nil when the
// token is unset so the caller can fall back to the console sender rather than
// standing up a client that would fail on every request.
func NewGeezSMSSender() *GeezSMSSender {
	token := strings.TrimSpace(os.Getenv("GEEZSMS_TOKEN"))
	if token == "" {
		return nil
	}
	endpoint := strings.TrimSpace(os.Getenv("GEEZSMS_URL"))
	if endpoint == "" {
		endpoint = geezSMSEndpoint
	}
	return &GeezSMSSender{
		Token:    token,
		Endpoint: endpoint,
		// Shorter than the service's own otpSendTimeout so a stalled provider
		// surfaces as a provider error rather than a cancelled context.
		Client: &http.Client{Timeout: 15 * time.Second},
	}
}

func (s *GeezSMSSender) Send(ctx context.Context, phone, message string) error {
	form := url.Values{
		"token": {s.Token},
		// The gateway wants the bare international form, 251XXXXXXXXX, while
		// everything upstream of here stores the canonical +251XXXXXXXXX.
		"phone": {strings.TrimPrefix(phone, "+")},
		"msg":   {message},
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.Endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := s.Client.Do(req)
	if err != nil {
		return fmt.Errorf("geezsms: request failed: %w", err)
	}
	defer resp.Body.Close()

	// Capped: an error page from a proxy in front of the gateway can be
	// arbitrarily large, and none of it belongs in the log.
	body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<10))
	if err != nil {
		return fmt.Errorf("geezsms: reading response: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("geezsms: http %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	// The gateway reports failures in the body with a 200 status, so the
	// status code alone is not evidence that anything was sent.
	var payload struct {
		Error   any    `json:"error"`
		Msg     string `json:"msg"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return fmt.Errorf("geezsms: unreadable response: %s", strings.TrimSpace(string(body)))
	}
	if isTruthy(payload.Error) {
		detail := payload.Msg
		if detail == "" {
			detail = payload.Message
		}
		if detail == "" {
			detail = strings.TrimSpace(string(body))
		}
		return fmt.Errorf("geezsms: send rejected: %s", detail)
	}

	log.Printf("[otp] sent via geezsms to %s", maskPhone(phone))
	return nil
}

// isTruthy reads the gateway's "error" field, which has been observed as both
// a JSON boolean and the strings "true"/"false". Anything unrecognized counts
// as an error: guessing "delivered" on a response we do not understand is the
// one outcome the caller must never be told.
func isTruthy(v any) bool {
	switch t := v.(type) {
	case nil:
		return false
	case bool:
		return t
	case string:
		return !strings.EqualFold(strings.TrimSpace(t), "false") && strings.TrimSpace(t) != "" && strings.TrimSpace(t) != "0"
	case float64:
		return t != 0
	default:
		return true
	}
}
