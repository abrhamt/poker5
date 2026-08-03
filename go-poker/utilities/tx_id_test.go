package utilities

import (
	"testing"
)

func TestGenerateTxID(t *testing.T) {
	id1, err := GenerateTxID()
	if err != nil {
		t.Fatalf("GenerateTxID error: %v", err)
	}
	if len(id1) != 10 {
		t.Fatalf("expected tx_id length 10, got %d (%s)", len(id1), id1)
	}

	id2, err := GenerateTxID()
	if err != nil {
		t.Fatalf("GenerateTxID error: %v", err)
	}
	if id1 == id2 {
		t.Fatalf("expected unique transaction IDs, got duplicate %s", id1)
	}
}

func TestGenerateReferralCode(t *testing.T) {
	code, err := GenerateReferralCode()
	if err != nil {
		t.Fatalf("GenerateReferralCode error: %v", err)
	}
	if len(code) != 8 {
		t.Fatalf("expected referral code length 8, got %d (%s)", len(code), code)
	}
}

func TestPasswordHashing(t *testing.T) {
	pass := "secret123"
	hash, err := HashPassword(pass)
	if err != nil {
		t.Fatalf("HashPassword error: %v", err)
	}
	if !CheckPassword(pass, hash) {
		t.Fatalf("CheckPassword failed for correct password")
	}
	if CheckPassword("wrongpass", hash) {
		t.Fatalf("CheckPassword succeeded for wrong password")
	}
}
