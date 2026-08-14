package utilities

import (
	"crypto/rand"
	"fmt"
	"math/big"
)

const txIDCharset = "ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"

func GenerateTxID() (string, error) {
	b := make([]byte, 10)
	charsetLen := big.NewInt(int64(len(txIDCharset)))
	for i := range b {
		n, err := rand.Int(rand.Reader, charsetLen)
		if err != nil {
			return "", err
		}
		b[i] = txIDCharset[n.Int64()]
	}
	return string(b), nil
}

// GenerateReferralCode returns a 6-digit numeric code in [100000, 999999],
// so it never starts with a leading zero.
func GenerateReferralCode() (string, error) {
	n, err := rand.Int(rand.Reader, big.NewInt(900000))
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%d", 100000+n.Int64()), nil
}

func GenerateRoomCode() (string, error) {
	const digits = "0123456789"
	b := make([]byte, 5)
	digitsLen := big.NewInt(int64(len(digits)))
	for i := range b {
		n, err := rand.Int(rand.Reader, digitsLen)
		if err != nil {
			return "", err
		}
		b[i] = digits[n.Int64()]
	}
	return string(b), nil
}
