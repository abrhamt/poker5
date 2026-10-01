package utilities

import (
	"errors"
	"regexp"
	"strings"
)

var (
	cbeAccountRegex = regexp.MustCompile(`^\d{13}$`)
	// Telebirr is Ethio Telecom's wallet, so only 09 numbers can hold one; 07
	// is Safaricom.
	telebirrRegex = regexp.MustCompile(`^(?:\+251|251|0)?(9\d{8})$`)

	ErrInvalidCBEAccount      = errors.New("a CBE account number is 13 digits")
	ErrInvalidTelebirrAccount = errors.New("enter the Telebirr number as 09XXXXXXXX")
)

// NormalizeCBEAccount accepts the account number the way people copy it —
// with spaces or dashes between groups — and returns the bare 13 digits.
func NormalizeCBEAccount(account string) (string, error) {
	cleaned := stripSeparators(account)
	if !cbeAccountRegex.MatchString(cleaned) {
		return "", ErrInvalidCBEAccount
	}
	return cleaned, nil
}

// NormalizeTelebirrAccount returns the number in the local 09XXXXXXXX form,
// which is how it is typed into the Telebirr app when paying it out.
func NormalizeTelebirrAccount(account string) (string, error) {
	matches := telebirrRegex.FindStringSubmatch(stripSeparators(account))
	if len(matches) < 2 {
		return "", ErrInvalidTelebirrAccount
	}
	return "0" + matches[1], nil
}

func stripSeparators(s string) string {
	return strings.NewReplacer(" ", "", "-", "", "(", "", ")", "").Replace(strings.TrimSpace(s))
}
