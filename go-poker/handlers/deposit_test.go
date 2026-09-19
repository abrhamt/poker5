package handlers

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/zuse/poker5/go-poker/repository"
	"github.com/zuse/poker5/go-poker/services"
)

const (
	testDepositAccountName   = "Yanet Joni And Nigist Ariya"
	testDepositAccountNumber = "1000000011758"
	testReceiptLink          = "https://mbreciept.cbe.com.et/v2-hfHCxGvIhvqVMTB7hylL"
)

type fakeVerifier struct{ receipt *services.Receipt }

func (f *fakeVerifier) Verify(context.Context, string) (*services.Receipt, error) {
	return f.receipt, nil
}

func enableRealDeposits(t *testing.T, db *sql.DB) {
	t.Helper()
	settings := services.NewSettingsService(repository.New(db))
	if err := settings.Update(context.Background(), services.UpdateSettingsRequest{
		RakeMode:             services.RakeModePercentage,
		RakePercentage:       5,
		CountdownSeconds:     15,
		RealDepositsEnabled:  true,
		DepositAccountName:   testDepositAccountName,
		DepositAccountNumber: testDepositAccountNumber,
	}); err != nil {
		t.Fatalf("enable real deposits: %v", err)
	}
}

func decodeJSON(t *testing.T, resp *http.Response) map[string]any {
	t.Helper()
	var body map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	return body
}

// With the toggle off the deposit form is the play-money one, and no bank
// account is advertised — a half-configured account must never be shown as
// somewhere to send real money.
func TestDepositInfoHidesAccountWhileToggleIsOff(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	srv, sender := newTestServer(t, db)
	cookie := registerAndVerify(t, srv, sender, "alice", "0911223344", "password123")

	req := httptest.NewRequest(http.MethodGet, "/api/wallet/deposit-info", nil)
	req.Header.Set("Cookie", cookie)
	resp, err := srv.App.Test(req)
	if err != nil {
		t.Fatalf("deposit-info: %v", err)
	}
	body := decodeJSON(t, resp)
	if body["real_deposits_enabled"] != false {
		t.Fatalf("real_deposits_enabled = %v, want false", body["real_deposits_enabled"])
	}
	if _, present := body["account_number"]; present {
		t.Fatalf("account number was advertised while real deposits are off: %v", body)
	}
}

// The play-money route has to close when real deposits go on: an old bundle or
// a crafted request would otherwise keep minting balance for free.
func TestPlainDepositIsRefusedWhenRealDepositsAreOn(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	srv, sender := newTestServer(t, db)
	cookie := registerAndVerify(t, srv, sender, "alice", "0911223344", "password123")
	enableRealDeposits(t, db)

	resp := postJSON(t, srv, "/api/wallet/deposit", map[string]int64{"amount": 5000}, cookie)
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("plain deposit status = %d, want 403", resp.StatusCode)
	}

	var wallet int64
	if err := db.QueryRow("SELECT wallet FROM users WHERE username = 'alice'").Scan(&wallet); err != nil {
		t.Fatalf("read wallet: %v", err)
	}
	if wallet != 0 {
		t.Fatalf("wallet = %d, want 0", wallet)
	}
}

func TestReceiptDepositEndpoint(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	sender := &recordingSender{}
	verifier := &fakeVerifier{receipt: &services.Receipt{
		Provider:  "cbe",
		Reference: "FT262325H0PP",
		Status:    "completed",
		Currency:  "ETB",
		Amount:    50,
		Payer:     services.ReceiptParty{Name: "Yaikob Demissie Jarso", Account: "1********3928"},
		Receiver:  services.ReceiptParty{Name: testDepositAccountName, Account: "1********1758"},
	}}
	srv := NewFiberServer(db, WithOTPSender(sender), WithReceiptVerifier(verifier))
	t.Cleanup(srv.Stop)

	cookie := registerAndVerify(t, srv, sender, "alice", "0911223344", "password123")
	enableRealDeposits(t, db)

	// The account to pay into now comes back, and it is the only place the
	// client learns it.
	req := httptest.NewRequest(http.MethodGet, "/api/wallet/deposit-info", nil)
	req.Header.Set("Cookie", cookie)
	resp, err := srv.App.Test(req)
	if err != nil {
		t.Fatalf("deposit-info: %v", err)
	}
	if body := decodeJSON(t, resp); body["account_number"] != testDepositAccountNumber {
		t.Fatalf("account_number = %v, want %s", body["account_number"], testDepositAccountNumber)
	}

	// A message with no CBE link in it is refused before anything is stored.
	resp = postJSON(t, srv, "/api/wallet/deposit/receipt", map[string]string{
		"message": "I sent you the money, promise",
	}, cookie)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("linkless paste status = %d, want 400", resp.StatusCode)
	}

	sms := "Dear Yaikob Demissie Jarso You have successfully transferred ETB50.00 ... " +
		testReceiptLink + "  for feedback: https://forms.gle/kGNGQpG3mQCCk3iD6"

	resp = postJSON(t, srv, "/api/wallet/deposit/receipt", map[string]string{"message": sms}, cookie)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("receipt deposit status = %d, want 200", resp.StatusCode)
	}
	body := decodeJSON(t, resp)
	if body["status"] != "credited" || body["amount"] != float64(50) {
		t.Fatalf("outcome = %v, want credited 50", body)
	}

	// The same receipt again is a conflict, and the wallet does not move.
	resp = postJSON(t, srv, "/api/wallet/deposit/receipt", map[string]string{"message": sms}, cookie)
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("replay status = %d, want 409", resp.StatusCode)
	}

	var wallet int64
	if err := db.QueryRow("SELECT wallet FROM users WHERE username = 'alice'").Scan(&wallet); err != nil {
		t.Fatalf("read wallet: %v", err)
	}
	if wallet != 50 {
		t.Fatalf("wallet = %d, want 50", wallet)
	}
}
