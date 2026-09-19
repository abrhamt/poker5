package utilities

import (
	"errors"
	"net/url"
	"regexp"
	"strings"
)

// MaxReceiptURLLength matches the receipt_url column. A paste longer than the
// column would be truncated on write, and a truncated URL is a different
// receipt.
const MaxReceiptURLLength = 255

var (
	ErrNoReceiptURL      = errors.New("no CBE receipt link found in that message — paste the whole SMS, including the https://... link at the end")
	ErrReceiptURLTooLong = errors.New("that receipt link is too long to be a CBE link")
)

// cbeReceiptURLPattern finds the receipt link inside a pasted SMS. The host is
// pinned to cbe.com.et so the feedback link CBE appends to every message
// (forms.gle/...) is never mistaken for the receipt — it appears later in the
// text, but "first URL in the message" is not the rule, "a CBE URL" is.
//
// The path is deliberately loose: the receipt token has changed shape before
// (v2- prefixed today) and pinning it would reject valid receipts the day CBE
// changes it, with the verifier being the thing that actually decides.
//
// Mirrored in frontend/src/lib/cbeReceipt.ts, which runs the same check before
// the request is made. This copy is the one that decides.
var cbeReceiptURLPattern = regexp.MustCompile(`(?i)\bhttps?://[a-z0-9.-]*cbe\.com\.et/[^\s<>"']+`)

// trailingPunctuation is what a sentence leaves stuck to a URL. CBE's own SMS
// ends the link with two spaces, but a user forwarding it by hand often lands
// a full stop against it.
const trailingPunctuation = `.,;:!?)]}'"`

// ExtractCBEReceiptURL pulls the receipt link out of a pasted SMS, or accepts a
// bare link. The returned URL is normalized only in the parts that are
// case-insensitive — scheme and host — because the path holds the receipt
// token, where case is significant.
func ExtractCBEReceiptURL(text string) (string, error) {
	match := cbeReceiptURLPattern.FindString(strings.TrimSpace(text))
	if match == "" {
		return "", ErrNoReceiptURL
	}
	match = strings.TrimRight(match, trailingPunctuation)

	parsed, err := url.Parse(match)
	if err != nil || parsed.Host == "" || strings.Trim(parsed.Path, "/") == "" {
		return "", ErrNoReceiptURL
	}

	// https regardless of what was pasted: the link is about to be handed to a
	// third party, and an http receipt token is readable in transit.
	parsed.Scheme = "https"
	parsed.Host = strings.ToLower(parsed.Host)
	// Query and fragment are not part of a receipt link, and dropping them
	// keeps two pastes of the same receipt from looking like different rows.
	parsed.RawQuery = ""
	parsed.Fragment = ""

	normalized := parsed.String()
	if len(normalized) > MaxReceiptURLLength {
		return "", ErrReceiptURLTooLong
	}
	return normalized, nil
}
