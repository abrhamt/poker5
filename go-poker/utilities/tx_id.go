package utilities

import (
	"crypto/rand"
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

func GenerateReferralCode() (string, error) {
	b := make([]byte, 8)
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
