package services

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/zuse/poker5/go-poker/internal/testdb"
	"github.com/zuse/poker5/go-poker/repository"
	"github.com/zuse/poker5/go-poker/utilities"
)

func TestMain(m *testing.M) {
	os.Exit(testdb.RunMain(m))
}

// stubVerifier stands in for the third party. Receipts are keyed by URL so a
// test can hand out two links for one transfer, which is the case the whole
// reference constraint exists for.
type stubVerifier struct {
	receipts map[string]*Receipt
	err      error
	calls    int
}

func (s *stubVerifier) Verify(_ context.Context, receiptURL string) (*Receipt, error) {
	s.calls++
	if s.err != nil {
		return nil, s.err
	}
	receipt, ok := s.receipts[receiptURL]
	if !ok {
		return nil, fmt.Errorf("%w: unknown link", ErrReceiptRejected)
	}
	return receipt, nil
}

const (
	houseAccountName   = "Yanet Joni And Nigist Ariya"
	houseAccountNumber = "1000000011758" // 13 digits, as CBE numbers are — the mask has to line up
	smsLink            = "https://mbreciept.cbe.com.et/v2-hfHCxGvIhvqVMTB7hylL"
	encodedLink        = "https://mbreciept.cbe.com.et/v2-hfHCxGvIhvqVMsyMRfXF"
)

func sampleReceipt(reference string, amount float64) *Receipt {
	return &Receipt{
		Provider:  "cbe",
		Reference: reference,
		Status:    "completed",
		Currency:  "ETB",
		Amount:    amount,
		Payer:     ReceiptParty{Name: "Yaikob Demissie Jarso", Account: "1********3928"},
		Receiver:  ReceiptParty{Name: houseAccountName, Account: "1********1758"},
	}
}

func newDepositFixture(t *testing.T, verifier ReceiptVerifier) (*DepositService, *repository.Queries, int64) {
	t.Helper()

	db := testdb.New(t)
	q := repository.New(db)
	ctx := context.Background()

	passHash, _ := utilities.HashPassword("password123")
	userID, err := q.CreateUser(ctx, repository.CreateUserParams{
		Username:     "alice",
		PhoneNumber:  "+251911223344",
		PasswordHash: passHash,
		ReferralCode: "ABC123",
		Role:         "player",
	})
	if err != nil {
		t.Fatalf("create user: %v", err)
	}

	settings := NewSettingsService(q)
	if err := settings.Update(ctx, UpdateSettingsRequest{
		RakeMode:             RakeModePercentage,
		RakePercentage:       5,
		CountdownSeconds:     15,
		RealDepositsEnabled:  true,
		DepositAccountName:   houseAccountName,
		DepositAccountNumber: houseAccountNumber,
	}); err != nil {
		t.Fatalf("enable real deposits: %v", err)
	}

	return NewDepositService(q, db, NewWalletService(q), settings, verifier), q, userID
}

func walletBalance(t *testing.T, q *repository.Queries, userID int64) int64 {
	t.Helper()
	user, err := q.GetUserByID(context.Background(), userID)
	if err != nil {
		t.Fatalf("read user: %v", err)
	}
	return user.Wallet
}

func TestSubmitReceiptCreditsOnce(t *testing.T) {
	verifier := &stubVerifier{receipts: map[string]*Receipt{
		smsLink: sampleReceipt("FT262325H0PP", 50),
	}}
	svc, q, userID := newDepositFixture(t, verifier)
	ctx := context.Background()

	// The pasted SMS, link buried in the middle of the text, with CBE's
	// feedback link after it.
	sms := "Dear Yaikob Demissie Jarso You have successfully transferred ETB50.00 from account 1**3928 " +
		"to account 1**1758 (Yanet Joni And Nigist Ariya). Your current balance is ETB16,263.47. " +
		"Thanks for Banking with CBE. " + smsLink + "  for feedback: https://forms.gle/kGNGQpG3mQCCk3iD6"

	outcome, err := svc.SubmitReceipt(ctx, userID, sms)
	if err != nil {
		t.Fatalf("first submit: %v", err)
	}
	if outcome.Status != DepositStatusCredited || outcome.Amount != 50 {
		t.Fatalf("outcome = %+v, want credited 50", outcome)
	}
	if got := walletBalance(t, q, userID); got != 50 {
		t.Fatalf("wallet = %d, want 50", got)
	}

	if _, err := svc.SubmitReceipt(ctx, userID, sms); !errors.Is(err, ErrReceiptAlreadyClaimed) {
		t.Fatalf("second submit err = %v, want ErrReceiptAlreadyClaimed", err)
	}
	if got := walletBalance(t, q, userID); got != 50 {
		t.Fatalf("wallet after replay = %d, want 50", got)
	}
}

// One transfer has more than one receipt link: the one CBE texts the sender,
// and the encodedReceipt link the bank returns for the same transfer. They
// differ, and both resolve to the same reference — so uniqueness keyed on the
// URL, or on (url, reference) together, would credit the money twice.
func TestSubmitReceiptRejectsSecondLinkForSameTransfer(t *testing.T) {
	verifier := &stubVerifier{receipts: map[string]*Receipt{
		smsLink:     sampleReceipt("FT262325H0PP", 50),
		encodedLink: sampleReceipt("FT262325H0PP", 50),
	}}
	svc, q, userID := newDepositFixture(t, verifier)
	ctx := context.Background()

	if _, err := svc.SubmitReceipt(ctx, userID, smsLink); err != nil {
		t.Fatalf("first link: %v", err)
	}
	if _, err := svc.SubmitReceipt(ctx, userID, encodedLink); !errors.Is(err, ErrReceiptAlreadyClaimed) {
		t.Fatalf("second link err = %v, want ErrReceiptAlreadyClaimed", err)
	}
	if got := walletBalance(t, q, userID); got != 50 {
		t.Fatalf("wallet = %d, want 50 — the same transfer was credited twice", got)
	}
}

func TestSubmitReceiptRejectsTransferToAnotherAccount(t *testing.T) {
	receipt := sampleReceipt("FT262325H0PP", 50)
	receipt.Receiver = ReceiptParty{Name: "Someone Else Entirely", Account: "1********9999"}
	verifier := &stubVerifier{receipts: map[string]*Receipt{smsLink: receipt}}
	svc, q, userID := newDepositFixture(t, verifier)

	if _, err := svc.SubmitReceipt(context.Background(), userID, smsLink); !errors.Is(err, ErrReceiptWrongAccount) {
		t.Fatalf("err = %v, want ErrReceiptWrongAccount", err)
	}
	if got := walletBalance(t, q, userID); got != 0 {
		t.Fatalf("wallet = %d, want 0", got)
	}
}

func TestSubmitReceiptRejectsIncompleteTransfer(t *testing.T) {
	receipt := sampleReceipt("FT262325H0PP", 50)
	receipt.Status = "pending"
	verifier := &stubVerifier{receipts: map[string]*Receipt{smsLink: receipt}}
	svc, _, userID := newDepositFixture(t, verifier)

	if _, err := svc.SubmitReceipt(context.Background(), userID, smsLink); !errors.Is(err, ErrReceiptNotCompleted) {
		t.Fatalf("err = %v, want ErrReceiptNotCompleted", err)
	}
}

// A dead verifier says nothing about the receipt, so the paste is kept rather
// than thrown away — and kept without a reference, which is what stops it from
// crediting anything until the bank has actually been asked.
func TestSubmitReceiptQueuesWhenVerifierIsDown(t *testing.T) {
	verifier := &stubVerifier{err: fmt.Errorf("%w: connection refused", ErrVerifierUnavailable)}
	svc, q, userID := newDepositFixture(t, verifier)
	ctx := context.Background()

	outcome, err := svc.SubmitReceipt(ctx, userID, smsLink)
	if err != nil {
		t.Fatalf("submit: %v", err)
	}
	if outcome.Status != DepositStatusPendingReview {
		t.Fatalf("status = %q, want pending_review", outcome.Status)
	}
	if got := walletBalance(t, q, userID); got != 0 {
		t.Fatalf("wallet = %d, want 0 — a queued receipt is not money yet", got)
	}

	// The same paste again is recognized rather than queued twice.
	if _, err := svc.SubmitReceipt(ctx, userID, smsLink); !errors.Is(err, ErrReceiptQueued) {
		t.Fatalf("resubmit err = %v, want ErrReceiptQueued", err)
	}

	pending, total, err := svc.PendingReview(ctx, 1)
	if err != nil {
		t.Fatalf("pending review: %v", err)
	}
	if total != 1 || len(pending) != 1 || pending[0].Username != "alice" {
		t.Fatalf("queue = %d rows (total %d), want 1 for alice", len(pending), total)
	}

	// Verifier recovers; approval re-checks with the bank and credits.
	verifier.err = nil
	verifier.receipts = map[string]*Receipt{smsLink: sampleReceipt("FT262325H0PP", 50)}

	outcome, err = svc.ApproveQueued(ctx, pending[0].BankDeposit.ID)
	if err != nil {
		t.Fatalf("approve: %v", err)
	}
	if outcome.Status != DepositStatusCredited || outcome.Amount != 50 {
		t.Fatalf("outcome = %+v, want credited 50", outcome)
	}
	if got := walletBalance(t, q, userID); got != 50 {
		t.Fatalf("wallet = %d, want 50", got)
	}
	if _, err := svc.ApproveQueued(ctx, pending[0].BankDeposit.ID); !errors.Is(err, ErrDepositNotPending) {
		t.Fatalf("re-approve err = %v, want ErrDepositNotPending", err)
	}
}

// Approval is not a credit instruction: an admin clicking it on a receipt the
// bank refuses must not move money.
func TestApproveQueuedRejectsWhatTheBankRefuses(t *testing.T) {
	verifier := &stubVerifier{err: fmt.Errorf("%w: connection refused", ErrVerifierUnavailable)}
	svc, q, userID := newDepositFixture(t, verifier)
	ctx := context.Background()

	if _, err := svc.SubmitReceipt(ctx, userID, smsLink); err != nil {
		t.Fatalf("submit: %v", err)
	}
	pending, _, _ := svc.PendingReview(ctx, 1)

	// Back up, and the bank does not recognize the link.
	verifier.err = nil
	verifier.receipts = map[string]*Receipt{}

	if _, err := svc.ApproveQueued(ctx, pending[0].BankDeposit.ID); !errors.Is(err, ErrReceiptRejected) {
		t.Fatalf("approve err = %v, want ErrReceiptRejected", err)
	}
	if got := walletBalance(t, q, userID); got != 0 {
		t.Fatalf("wallet = %d, want 0", got)
	}
	row, err := q.GetBankDepositByID(ctx, pending[0].BankDeposit.ID)
	if err != nil {
		t.Fatalf("read row: %v", err)
	}
	if row.Status != DepositStatusRejected {
		t.Fatalf("status = %q, want rejected", row.Status)
	}
}

func TestSubmitReceiptRefusedWhenRealDepositsAreOff(t *testing.T) {
	verifier := &stubVerifier{receipts: map[string]*Receipt{smsLink: sampleReceipt("FT262325H0PP", 50)}}
	svc, q, userID := newDepositFixture(t, verifier)
	ctx := context.Background()

	if err := svc.settings.Update(ctx, UpdateSettingsRequest{
		RakeMode:         RakeModePercentage,
		RakePercentage:   5,
		CountdownSeconds: 15,
	}); err != nil {
		t.Fatalf("disable: %v", err)
	}

	if _, err := svc.SubmitReceipt(ctx, userID, smsLink); !errors.Is(err, ErrRealDepositsDisabled) {
		t.Fatalf("err = %v, want ErrRealDepositsDisabled", err)
	}
	if verifier.calls != 0 {
		t.Fatalf("verifier called %d times while deposits were off", verifier.calls)
	}
	if got := walletBalance(t, q, userID); got != 0 {
		t.Fatalf("wallet = %d, want 0", got)
	}
}

func TestEnablingRealDepositsRequiresAnAccount(t *testing.T) {
	svc, _, _ := newDepositFixture(t, &stubVerifier{})
	err := svc.settings.Update(context.Background(), UpdateSettingsRequest{
		RakeMode:            RakeModePercentage,
		RakePercentage:      5,
		CountdownSeconds:    15,
		RealDepositsEnabled: true,
	})
	if !errors.Is(err, ErrDepositAccountRequired) {
		t.Fatalf("err = %v, want ErrDepositAccountRequired", err)
	}
}

func TestAccountMatches(t *testing.T) {
	cases := []struct {
		configured, masked string
		want               bool
	}{
		{"1000011758", "1********1758", false}, // 10 digits configured, 13 on the receipt
		{"1000011758", "1*****1758", true},
		{"1000011758", "1*****9999", false},
		{"1000011758", "2*****1758", false},
		{"1000011758", "1000011758", true},
		{"1000011758", "", false},
		{"", "1*****1758", false},
		// Without the length check a short configured value would match any
		// number sharing its ends.
		{"1758", "1*****1758", false},
	}
	for _, tc := range cases {
		if got := accountMatches(tc.configured, tc.masked); got != tc.want {
			t.Errorf("accountMatches(%q, %q) = %v, want %v", tc.configured, tc.masked, got, tc.want)
		}
	}
}

func TestNamesMatch(t *testing.T) {
	cases := []struct {
		configured, fromReceipt string
		want                    bool
	}{
		{"Yanet Joni And Nigist Ariya", "YANET JONI AND NIGIST ARIYA", true},
		{"  Yanet   Joni And  Nigist Ariya ", "Yanet Joni And Nigist Ariya", true},
		{"Yanet Joni And Nigist Ariya", "Yanet Joni And Nigist", false},
		{"Yanet Joni And Nigist Ariya", "Yanet Joni And Nigist Ariya Extra", false},
		{"Yanet Joni And Nigist Ariya", "", false},
		{"", "Yanet Joni And Nigist Ariya", false},
	}
	for _, tc := range cases {
		if got := namesMatch(tc.configured, tc.fromReceipt); got != tc.want {
			t.Errorf("namesMatch(%q, %q) = %v, want %v", tc.configured, tc.fromReceipt, got, tc.want)
		}
	}
}

func TestCreditableAmountRoundsDown(t *testing.T) {
	cases := []struct {
		amount float64
		want   int64
	}{
		{50, 50},
		{50.99, 50},
		{49.999999999, 50}, // float noise on a clean 50.00
		{0.4, 0},
		{-5, 0},
	}
	for _, tc := range cases {
		if got := creditableAmount(tc.amount); got != tc.want {
			t.Errorf("creditableAmount(%v) = %d, want %d", tc.amount, got, tc.want)
		}
	}
}

// The sweep is the reason the review queue is a fallback and not the normal
// path: a receipt queued during an outage credits itself once the verifier is
// back, with no admin involved.
func TestQueueSweepCreditsOnceTheVerifierRecovers(t *testing.T) {
	verifier := &stubVerifier{err: fmt.Errorf("%w: connection refused", ErrVerifierUnavailable)}
	svc, q, userID := newDepositFixture(t, verifier)
	ctx := context.Background()

	if _, err := svc.SubmitReceipt(ctx, userID, smsLink); err != nil {
		t.Fatalf("submit: %v", err)
	}

	// Still down: the row is left alone, and the attempt is recorded so the
	// next sweep backs off instead of hammering.
	if got := svc.RunQueueSweep(ctx); got.Unchanged != 1 || got.Credited != 0 {
		t.Fatalf("sweep during outage = %+v, want 1 unchanged", got)
	}
	if got := walletBalance(t, q, userID); got != 0 {
		t.Fatalf("wallet = %d, want 0", got)
	}

	pending, _, _ := svc.PendingReview(ctx, 1)
	row := pending[0].BankDeposit
	if row.Attempts != 1 {
		t.Fatalf("attempts = %d, want 1", row.Attempts)
	}

	// The backoff is real: an immediate second sweep does not touch the row.
	verifier.calls = 0
	if got := svc.RunQueueSweep(ctx); got != (QueueSweepResult{}) {
		t.Fatalf("sweep inside backoff = %+v, want nothing touched", got)
	}
	if verifier.calls != 0 {
		t.Fatalf("verifier called %d times inside the backoff window", verifier.calls)
	}

	// Verifier recovers, and the row comes due.
	verifier.err = nil
	verifier.receipts = map[string]*Receipt{smsLink: sampleReceipt("FT262325H0PP", 50)}
	makeQueueRowDue(t, svc, row.ID)

	if got := svc.RunQueueSweep(ctx); got.Credited != 1 {
		t.Fatalf("sweep after recovery = %+v, want 1 credited", got)
	}
	if got := walletBalance(t, q, userID); got != 50 {
		t.Fatalf("wallet = %d, want 50", got)
	}

	// Nothing left to do, and no double credit on the next pass.
	if got := svc.RunQueueSweep(ctx); got != (QueueSweepResult{}) {
		t.Fatalf("sweep after crediting = %+v, want nothing", got)
	}
	if got := walletBalance(t, q, userID); got != 50 {
		t.Fatalf("wallet after extra sweep = %d, want 50", got)
	}
	stored, err := q.GetBankDepositByID(ctx, row.ID)
	if err != nil {
		t.Fatalf("read row: %v", err)
	}
	if stored.Status != DepositStatusCredited || stored.Reference.String != "FT262325H0PP" {
		t.Fatalf("row = %+v, want credited with its reference", stored)
	}
}

// Once the bank answers, its answer is a verdict — the row is closed rather
// than retried until the attempt cap runs out.
func TestQueueSweepRejectsWhatTheBankRefuses(t *testing.T) {
	verifier := &stubVerifier{err: fmt.Errorf("%w: connection refused", ErrVerifierUnavailable)}
	svc, q, userID := newDepositFixture(t, verifier)
	ctx := context.Background()

	if _, err := svc.SubmitReceipt(ctx, userID, smsLink); err != nil {
		t.Fatalf("submit: %v", err)
	}
	pending, _, _ := svc.PendingReview(ctx, 1)

	verifier.err = nil
	verifier.receipts = map[string]*Receipt{} // bank does not know this link

	if got := svc.RunQueueSweep(ctx); got.Rejected != 1 {
		t.Fatalf("sweep = %+v, want 1 rejected", got)
	}
	if got := walletBalance(t, q, userID); got != 0 {
		t.Fatalf("wallet = %d, want 0", got)
	}
	stored, _ := q.GetBankDepositByID(ctx, pending[0].BankDeposit.ID)
	if stored.Status != DepositStatusRejected {
		t.Fatalf("status = %q, want rejected", stored.Status)
	}
}

// A row the verifier can never resolve must not be re-checked forever. After
// the cap the machine stops, but the row stays in the queue for a person.
func TestQueueSweepGivesUpAfterTheAttemptCap(t *testing.T) {
	verifier := &stubVerifier{err: fmt.Errorf("%w: connection refused", ErrVerifierUnavailable)}
	svc, _, userID := newDepositFixture(t, verifier)
	ctx := context.Background()

	if _, err := svc.SubmitReceipt(ctx, userID, smsLink); err != nil {
		t.Fatalf("submit: %v", err)
	}
	pending, _, _ := svc.PendingReview(ctx, 1)
	id := pending[0].BankDeposit.ID

	for i := 0; i < MaxQueueAttempts; i++ {
		makeQueueRowDue(t, svc, id)
		if got := svc.RunQueueSweep(ctx); got.Unchanged != 1 {
			t.Fatalf("sweep %d = %+v, want 1 unchanged", i, got)
		}
	}

	verifier.calls = 0
	makeQueueRowDue(t, svc, id)
	if got := svc.RunQueueSweep(ctx); got != (QueueSweepResult{}) {
		t.Fatalf("sweep past the cap = %+v, want nothing touched", got)
	}
	if verifier.calls != 0 {
		t.Fatalf("verifier called %d times past the attempt cap", verifier.calls)
	}

	// Still visible to an admin — only the automatic retry gave up.
	if _, total, _ := svc.PendingReview(ctx, 1); total != 1 {
		t.Fatalf("queue total = %d, want the row still waiting for a human", total)
	}
}

// The sweep must not act while deposits are play money: the account it would
// validate against is not in force.
func TestQueueSweepSkipsWhileRealDepositsAreOff(t *testing.T) {
	verifier := &stubVerifier{err: fmt.Errorf("%w: down", ErrVerifierUnavailable)}
	svc, _, userID := newDepositFixture(t, verifier)
	ctx := context.Background()

	if _, err := svc.SubmitReceipt(ctx, userID, smsLink); err != nil {
		t.Fatalf("submit: %v", err)
	}
	if err := svc.settings.Update(ctx, UpdateSettingsRequest{
		RakeMode: RakeModePercentage, RakePercentage: 5, CountdownSeconds: 15,
	}); err != nil {
		t.Fatalf("disable: %v", err)
	}

	verifier.calls = 0
	if got := svc.RunQueueSweep(ctx); got != (QueueSweepResult{}) {
		t.Fatalf("sweep = %+v, want nothing", got)
	}
	if verifier.calls != 0 {
		t.Fatalf("verifier called %d times while deposits were off", verifier.calls)
	}
}

// makeQueueRowDue backdates a row's last attempt so the next sweep picks it up,
// rather than making the test wait out the real backoff.
func makeQueueRowDue(t *testing.T, svc *DepositService, id int64) {
	t.Helper()
	if _, err := svc.db.Exec("UPDATE bank_deposits SET last_attempt_at = 0 WHERE id = $1", id); err != nil {
		t.Fatalf("backdate row %d: %v", id, err)
	}
}

// The escape hatch for a receipt the verifier can never read: the admin types
// the amount, the player is credited, and the row records that no bank check
// happened.
func TestCreditManuallyCreditsTheTypedAmount(t *testing.T) {
	verifier := &stubVerifier{err: fmt.Errorf("%w: connection refused", ErrVerifierUnavailable)}
	svc, q, userID := newDepositFixture(t, verifier)
	ctx := context.Background()

	if _, err := svc.SubmitReceipt(ctx, userID, smsLink); err != nil {
		t.Fatalf("submit: %v", err)
	}
	pending, _, err := svc.PendingReview(ctx, 1)
	if err != nil {
		t.Fatalf("pending review: %v", err)
	}
	depositID := pending[0].BankDeposit.ID

	outcome, err := svc.CreditManually(ctx, depositID, 250, " ft262325h0pp ", "admin")
	if err != nil {
		t.Fatalf("manual credit: %v", err)
	}
	if outcome.Status != DepositStatusCredited || outcome.Amount != 250 {
		t.Fatalf("outcome = %+v, want credited 250", outcome)
	}
	if got := walletBalance(t, q, userID); got != 250 {
		t.Fatalf("wallet = %d, want 250", got)
	}

	row, err := q.GetBankDepositByID(ctx, depositID)
	if err != nil {
		t.Fatalf("read deposit: %v", err)
	}
	// The hand-typed reference is normalized to the form the verifier reports,
	// so a later automatic credit of the same transfer collides with it.
	if row.Reference.String != "FT262325H0PP" {
		t.Fatalf("reference = %q, want the normalized FT number", row.Reference.String)
	}
	if !strings.Contains(row.Note, "admin") {
		t.Fatalf("note = %q, want the admin who credited it", row.Note)
	}
	if row.TransactionID.String == "" {
		t.Fatalf("no wallet transaction recorded on the row")
	}

	// A resolved row is done, however it was resolved.
	if _, err := svc.CreditManually(ctx, depositID, 250, "FT262325H0PP", "admin"); !errors.Is(err, ErrDepositNotPending) {
		t.Fatalf("second manual credit err = %v, want ErrDepositNotPending", err)
	}
}

// A manual credit carrying the FT number still cannot be spent twice: the
// same transfer arriving on its other link hits the reference constraint.
func TestCreditManuallyKeepsTheOneReferenceRule(t *testing.T) {
	verifier := &stubVerifier{err: fmt.Errorf("%w: connection refused", ErrVerifierUnavailable)}
	svc, q, userID := newDepositFixture(t, verifier)
	ctx := context.Background()

	if _, err := svc.SubmitReceipt(ctx, userID, smsLink); err != nil {
		t.Fatalf("submit: %v", err)
	}
	pending, _, _ := svc.PendingReview(ctx, 1)
	if _, err := svc.CreditManually(ctx, pending[0].BankDeposit.ID, 50, "FT262325H0PP", "admin"); err != nil {
		t.Fatalf("manual credit: %v", err)
	}

	// The verifier recovers and the player pastes the encoded link for the
	// very same transfer.
	verifier.err = nil
	verifier.receipts = map[string]*Receipt{encodedLink: sampleReceipt("FT262325H0PP", 50)}

	if _, err := svc.SubmitReceipt(ctx, userID, encodedLink); !errors.Is(err, ErrReceiptAlreadyClaimed) {
		t.Fatalf("second link err = %v, want ErrReceiptAlreadyClaimed", err)
	}
	if got := walletBalance(t, q, userID); got != 50 {
		t.Fatalf("wallet = %d, want 50 — the transfer was credited twice", got)
	}
}

func TestCreditManuallyRefusesImpossibleAmounts(t *testing.T) {
	verifier := &stubVerifier{err: fmt.Errorf("%w: connection refused", ErrVerifierUnavailable)}
	svc, q, userID := newDepositFixture(t, verifier)
	ctx := context.Background()

	if _, err := svc.SubmitReceipt(ctx, userID, smsLink); err != nil {
		t.Fatalf("submit: %v", err)
	}
	pending, _, _ := svc.PendingReview(ctx, 1)
	depositID := pending[0].BankDeposit.ID

	for _, amount := range []int64{0, -100, MaxManualDeposit + 1} {
		if _, err := svc.CreditManually(ctx, depositID, amount, "", "admin"); !errors.Is(err, ErrManualAmountInvalid) {
			t.Fatalf("amount %d err = %v, want ErrManualAmountInvalid", amount, err)
		}
	}
	if got := walletBalance(t, q, userID); got != 0 {
		t.Fatalf("wallet = %d, want 0", got)
	}
}
