package utilities

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"strconv"
	"strings"
	"time"
)

const (
	RouterSignatureHeader  = "X-Router-Signature"
	RouterTimestampHeader  = "X-Router-Timestamp"
	RouterEventHeader      = "X-Router-Event"
	RouterDeliveryIDHeader = "X-Router-Delivery-Id"

	routerSignatureVersion   = "v1"
	routerSignatureTolerance = 5 * time.Minute
)

func RouterSignature(secret, timestamp string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(timestamp + "."))
	mac.Write(body)
	return hex.EncodeToString(mac.Sum(nil))
}

func VerifyRouterSignature(secret, signatureHeader, timestampHeader string, body []byte, now time.Time) bool {
	if secret == "" || signatureHeader == "" || timestampHeader == "" {
		return false
	}
	timestamp, err := strconv.ParseInt(strings.TrimSpace(timestampHeader), 10, 64)
	if err != nil {
		return false
	}
	skew := now.Unix() - timestamp
	if skew < 0 {
		skew = -skew
	}
	if time.Duration(skew)*time.Second > routerSignatureTolerance {
		return false
	}

	expected := []byte(RouterSignature(secret, strings.TrimSpace(timestampHeader), body))
	for _, entry := range strings.Split(signatureHeader, ",") {
		version, digest, ok := strings.Cut(strings.TrimSpace(entry), "=")
		if !ok || version != routerSignatureVersion {
			continue
		}
		if hmac.Equal([]byte(digest), expected) {
			return true
		}
	}
	return false
}
