package services

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"
)

// OTPPurpose distinguishes the two flows that send codes. It keys the throttle
// table, so registration and reset carry independent budgets.
type OTPPurpose string

const (
	PurposeRegister OTPPurpose = "register"
	PurposeReset    OTPPurpose = "reset"
)

// OTPSender is the one seam between this app and an SMS provider. It takes a
// rendered message rather than a code: the wording is a product decision that
// should not move when the transport does, and it will change more often than
// the provider will.
type OTPSender interface {
	Send(ctx context.Context, phone, message string) error
}

// RenderOTPMessage produces the SMS text. The code leads both messages so it
// is readable in a lock-screen notification preview. Reset carries an extra
// line: "someone is trying to reset your password" is information the
// recipient needs if it wasn't them.
func RenderOTPMessage(purpose OTPPurpose, code string) string {
	switch purpose {
	case PurposeReset:
		return fmt.Sprintf("%s is your Golden Poker password reset code. It expires in 5 minutes. If this wasn't you, just ignore this message.", code)
	default:
		return fmt.Sprintf("%s is your Golden Poker verification code. It expires in 5 minutes.", code)
	}
}

// ConsoleSender is the stand-in until a real SMS provider is wired up. It
// prints the message so a developer can complete the flow locally.
//
// Outside development it prints a redacted line and returns an error rather
// than nil. Returning nil would mean the service tells the user "check your
// phone" when no message exists anywhere — a production auth flow silently
// fabricating success. Failing loudly makes a premature deploy obvious on the
// first signup attempt instead of on the first support ticket.
type ConsoleSender struct {
	// Development is true when APP_ENV=development. Set at construction so the
	// behaviour of a running process cannot drift.
	Development bool
}

var ErrSenderNotConfigured = fmt.Errorf("no SMS provider configured: verification codes cannot be delivered")

func NewConsoleSender() *ConsoleSender {
	dev := strings.EqualFold(strings.TrimSpace(os.Getenv("APP_ENV")), "development")
	if dev {
		log.Printf("WARNING: OTP sender is CONSOLE — verification codes are written to the log in full. Development only.")
	} else {
		log.Printf("WARNING: OTP sender is CONSOLE and APP_ENV is not 'development' — every verification and password-reset request will fail until a real SMS provider is wired up.")
	}
	return &ConsoleSender{Development: dev}
}

func (s *ConsoleSender) Send(_ context.Context, phone, message string) error {
	if !s.Development {
		log.Printf("[otp] refusing to send to %s outside development: no SMS provider configured", maskPhone(phone))
		return ErrSenderNotConfigured
	}
	log.Printf("[otp] to %s: %s", phone, message)
	return nil
}

func maskPhone(phone string) string {
	if len(phone) <= 6 {
		return "***"
	}
	return phone[:5] + strings.Repeat("*", len(phone)-8) + phone[len(phone)-3:]
}
