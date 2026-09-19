package utilities

import (
	"strconv"
	"testing"
	"time"
)

const testRouterSecret = "whsec_test"

func signedHeaders(secret string, body []byte, at time.Time) (string, string) {
	ts := strconv.FormatInt(at.Unix(), 10)
	return "v1=" + RouterSignature(secret, ts, body), ts
}

func TestVerifyRouterSignatureAcceptsMatchingDigest(t *testing.T) {
	now := time.Now()
	body := []byte(`{"event":"payment.succeeded"}`)
	sig, ts := signedHeaders(testRouterSecret, body, now)

	if !VerifyRouterSignature(testRouterSecret, sig, ts, body, now) {
		t.Fatal("valid signature was rejected")
	}
}

func TestVerifyRouterSignatureRejectsTamperedBody(t *testing.T) {
	now := time.Now()
	body := []byte(`{"event":"payment.succeeded"}`)
	sig, ts := signedHeaders(testRouterSecret, body, now)

	if VerifyRouterSignature(testRouterSecret, sig, ts, []byte(`{"event":"payment.failed"}`), now) {
		t.Fatal("tampered body was accepted")
	}
}

func TestVerifyRouterSignatureRejectsWrongSecret(t *testing.T) {
	now := time.Now()
	body := []byte(`{}`)
	sig, ts := signedHeaders("whsec_other", body, now)

	if VerifyRouterSignature(testRouterSecret, sig, ts, body, now) {
		t.Fatal("signature from another secret was accepted")
	}
}

func TestVerifyRouterSignatureRejectsStaleTimestamp(t *testing.T) {
	now := time.Now()
	body := []byte(`{}`)
	sig, ts := signedHeaders(testRouterSecret, body, now.Add(-10*time.Minute))

	if VerifyRouterSignature(testRouterSecret, sig, ts, body, now) {
		t.Fatal("stale timestamp was accepted")
	}
}

func TestVerifyRouterSignatureAcceptsAnyEntryDuringRotation(t *testing.T) {
	now := time.Now()
	body := []byte(`{}`)
	ts := strconv.FormatInt(now.Unix(), 10)
	header := "v1=" + RouterSignature("whsec_old", ts, body) + ",v1=" + RouterSignature(testRouterSecret, ts, body)

	if !VerifyRouterSignature(testRouterSecret, header, ts, body, now) {
		t.Fatal("rotated signature header was rejected")
	}
}

func TestVerifyRouterSignatureFailsClosed(t *testing.T) {
	now := time.Now()
	body := []byte(`{}`)
	sig, ts := signedHeaders(testRouterSecret, body, now)

	cases := map[string][4]string{
		"empty secret":    {"", sig, ts},
		"missing header":  {testRouterSecret, "", ts},
		"missing ts":      {testRouterSecret, sig, ""},
		"bad ts":          {testRouterSecret, sig, "soon"},
		"unknown version": {testRouterSecret, "v2=" + sig[3:], ts},
	}
	for name, c := range cases {
		if VerifyRouterSignature(c[0], c[1], c[2], body, now) {
			t.Fatalf("%s: verification passed", name)
		}
	}
}
