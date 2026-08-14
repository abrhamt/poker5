package utilities

import (
	"errors"
	"regexp"
	"strings"
)

var (
	ethiopianPhoneRegex = regexp.MustCompile(`^(?:\+251|251|0)?([79]\d{8})$`)
	ErrInvalidPhone     = errors.New("invalid Ethiopian phone number (must start with 09 or 07 followed by 8 digits, e.g. 0912345678 or +251912345678)")
)

func ValidateAndNormalizeEthiopianPhone(phone string) (string, error) {
	cleaned := strings.TrimSpace(phone)
	cleaned = strings.ReplaceAll(cleaned, " ", "")
	cleaned = strings.ReplaceAll(cleaned, "-", "")
	cleaned = strings.ReplaceAll(cleaned, "(", "")
	cleaned = strings.ReplaceAll(cleaned, ")", "")

	matches := ethiopianPhoneRegex.FindStringSubmatch(cleaned)
	if len(matches) < 2 {
		return "", ErrInvalidPhone
	}

	nationalPart := matches[1]
	return "+251" + nationalPart, nil
}
