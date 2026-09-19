package utilities

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"math/big"
)

// MinPasswordLength is the floor for every path that sets a password —
// registration and reset alike. Deliberately a length rule and nothing else:
// character-class requirements push people toward "Password1!" and buy less
// than the extra characters they discourage.
const MinPasswordLength = 6

var ErrPasswordTooShort = fmt.Errorf("password must be at least %d characters", MinPasswordLength)

// GenerateOTPCode returns a six-digit code. crypto/rand rather than math/rand:
// a math/rand code is predictable to anyone who knows roughly when it was
// issued, which is exactly what an attacker requesting a reset knows.
func GenerateOTPCode() (string, error) {
	n, err := rand.Int(rand.Reader, big.NewInt(1000000))
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%06d", n.Int64()), nil
}

// GenerateResetToken returns the token handed to the client after a reset code
// is verified, alongside the SHA-256 digest that is what actually gets stored.
// Hashing works here — unlike on the six-digit code, where 10^6 digests fall in
// under a second — because 32 random bytes have the entropy to make it one-way.
func GenerateResetToken() (token string, digest string, err error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", "", err
	}
	token = hex.EncodeToString(b)
	return token, HashResetToken(token), nil
}

func HashResetToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// CodesMatch compares in constant time so that response latency carries no
// information about how much of a guessed code was correct.
func CodesMatch(supplied, stored string) bool {
	if stored == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(supplied), []byte(stored)) == 1
}

func ValidatePassword(password string) error {
	if len([]rune(password)) < MinPasswordLength {
		return ErrPasswordTooShort
	}
	return nil
}
