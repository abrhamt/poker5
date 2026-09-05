package utilities

import (
	"errors"
	"strings"
	"testing"
)

const realSMS = `Dear  Yaikob Demissie Jarso You have successfully transferred ETB50.00 from account 1**3928 to account 1**1758 (Yanet Joni And Nigist Ariya). Service charge of ETB 0.50 and VAT(15%) of ETB0.08 and Disaster Recovery(5%) of 0.03 with total of ETB50.61 .Your current balance is ETB16,263.47. Thanks for Banking with CBE. https://mbreciept.cbe.com.et/v2-hfHCxGvIhvqVMTB7hylL  for feedback: https://forms.gle/kGNGQpG3mQCCk3iD6`

func TestExtractCBEReceiptURL(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  string
	}{
		{
			// The feedback link sits after the receipt link in every CBE
			// message, so "the first URL" is not the rule — "the CBE URL" is.
			name:  "full SMS ignores the feedback link",
			input: realSMS,
			want:  "https://mbreciept.cbe.com.et/v2-hfHCxGvIhvqVMTB7hylL",
		},
		{
			name:  "bare link",
			input: "https://mbreciept.cbe.com.et/v2-hfHCxGvIhvqVMTB7hylL",
			want:  "https://mbreciept.cbe.com.et/v2-hfHCxGvIhvqVMTB7hylL",
		},
		{
			name:  "surrounding whitespace",
			input: "  \n https://mbreciept.cbe.com.et/v2-hfHCxGvIhvqVMTB7hylL \n ",
			want:  "https://mbreciept.cbe.com.et/v2-hfHCxGvIhvqVMTB7hylL",
		},
		{
			name:  "trailing full stop is not part of the link",
			input: "sent it: https://mbreciept.cbe.com.et/v2-hfHCxGvIhvqVMTB7hylL.",
			want:  "https://mbreciept.cbe.com.et/v2-hfHCxGvIhvqVMTB7hylL",
		},
		{
			name:  "http is upgraded",
			input: "http://mbreciept.cbe.com.et/v2-hfHCxGvIhvqVMTB7hylL",
			want:  "https://mbreciept.cbe.com.et/v2-hfHCxGvIhvqVMTB7hylL",
		},
		{
			// Two pastes of one receipt must normalize to one row, and a query
			// string is not part of a receipt link.
			name:  "query and fragment are dropped",
			input: "https://mbreciept.cbe.com.et/v2-hfHCxGvIhvqVMTB7hylL?utm=sms#top",
			want:  "https://mbreciept.cbe.com.et/v2-hfHCxGvIhvqVMTB7hylL",
		},
		{
			name:  "host case is normalized, token case is not",
			input: "https://MBReciePT.CBE.COM.ET/v2-hfHCxGvIhvqVMTB7hylL",
			want:  "https://mbreciept.cbe.com.et/v2-hfHCxGvIhvqVMTB7hylL",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ExtractCBEReceiptURL(tc.input)
			if err != nil {
				t.Fatalf("err = %v", err)
			}
			if got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestExtractCBEReceiptURLRejects(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  error
	}{
		{"empty", "", ErrNoReceiptURL},
		{"no link at all", "You have successfully transferred ETB50.00", ErrNoReceiptURL},
		{"only the feedback link", "for feedback: https://forms.gle/kGNGQpG3mQCCk3iD6", ErrNoReceiptURL},
		// A lookalike host is the one an attacker would reach for.
		{"lookalike host", "https://mbreciept.cbe.com.et.evil.example/v2-token", ErrNoReceiptURL},
		{"no receipt token", "https://mbreciept.cbe.com.et/", ErrNoReceiptURL},
		{"absurdly long", "https://mbreciept.cbe.com.et/" + strings.Repeat("x", 300), ErrReceiptURLTooLong},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ExtractCBEReceiptURL(tc.input)
			if !errors.Is(err, tc.want) {
				t.Fatalf("err = %v (url %q), want %v", err, got, tc.want)
			}
		})
	}
}
