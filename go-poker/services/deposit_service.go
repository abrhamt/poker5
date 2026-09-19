package services

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"math"
	"regexp"
	"strings"
	"time"

	"github.com/zuse/poker5/go-poker/repository"
	"github.com/zuse/poker5/go-poker/utilities"
)

// Deposit row statuses.
const (
	DepositStatusCredited      = "credited"
	DepositStatusPendingReview = "pending_review"
	DepositStatusRejected      = "rejected"
)

const (
	// MinReceiptDeposit is the floor in whole ETB. A receipt below it is
	// refused rather than credited as zero, so nobody burns a real transfer on
	// a deposit that adds nothing.
	MinReceiptDeposit = 1

	// depositVerifyTimeout is the fallback bound on a whole verification when
	// the verifier does not publish a budget of its own — a fake in a test, or
	// any future implementation. The real HTTP verifier does publish one
	// (HTTPReceiptVerifier.Budget), and that is used instead: a deadline picked
	// here independently of the retry policy is a deadline that cancels the
	// last attempt half-way through, which is exactly the retry the user was
	// made to wait for.
	depositVerifyTimeout = 45 * time.Second

	DepositReviewPageSize = 25

	// Retry policy for queued receipts. The sweep interval is what the user
	// actually feels — a receipt pasted during an outage is credited on its own
	// within about this long of the verifier coming back, without anyone
	// touching the admin queue.
	QueueSweepInterval = 5 * time.Minute

	// queueRetryBackoff is the minimum gap between two attempts on the same
	// row, so a batch that keeps failing does not turn one outage into a
	// retry storm against a service that is already struggling.
	queueRetryBackoff = 10 * time.Minute

	// MaxQueueAttempts stops the automatic retry after roughly a day of
	// failures. The row stays in the admin queue — it is only the machine that
	// gives up, not the deposit.
	MaxQueueAttempts = 24

	// MaxManualDeposit caps what an admin can type into the manual credit box.
	// Nothing about a real transfer needs a ceiling — this is purely a guard
	// against a slipped keystroke minting a fortune, since a manual credit is
	// the one path with no bank figure to check the number against.
	MaxManualDeposit = 1_000_000

	// queueSweepBatch caps one pass, so a backlog is worked through over
	// several sweeps rather than in one long burst of third-party calls.
	queueSweepBatch = 20
)

var (
	ErrRealDepositsDisabled  = errors.New("bank deposits are not enabled")
	ErrRealDepositsRequired  = errors.New("deposits now require a CBE transfer receipt")
	ErrDepositAccountUnset   = errors.New("no deposit account is configured — contact support")
	ErrReceiptAlreadyClaimed = errors.New("that receipt has already been used for a deposit")
	ErrReceiptQueued         = errors.New("that receipt is already waiting for review")
	ErrReceiptWrongAccount   = errors.New("that transfer went to a different account — check the deposit details and send again")
	ErrReceiptNotCompleted   = errors.New("that transfer has not completed at the bank yet")
	ErrReceiptTooSmall       = fmt.Errorf("the minimum deposit is %d ETB", MinReceiptDeposit)
	ErrReceiptWrongCurrency  = errors.New("only ETB transfers can be deposited")
	ErrDepositNotPending     = errors.New("that deposit is not awaiting review")
	ErrManualAmountInvalid   = fmt.Errorf("enter the deposited amount in whole ETB, between %d and %d", MinReceiptDeposit, MaxManualDeposit)
)

// DepositConfig is what the deposit screen needs to know before it can render:
// which form to show, and where to send the money.
type DepositConfig struct {
	RealDepositsEnabled bool   `json:"real_deposits_enabled"`
	AccountName         string `json:"account_name"`
	AccountNumber       string `json:"account_number"`
}

// DepositOutcome reports what happened to a pasted receipt. Queued is not a
// failure: the money is real and the receipt is kept, it just could not be
// checked yet.
type DepositOutcome struct {
	Status        string `json:"status"`
	Amount        int64  `json:"amount"`
	Reference     string `json:"reference,omitempty"`
	TransactionID string `json:"transaction_id,omitempty"`
	Message       string `json:"message"`
}

type DepositService struct {
	q        *repository.Queries
	db       *sql.DB
	wallet   *WalletService
	settings *SettingsService
	verifier ReceiptVerifier
}

func NewDepositService(q *repository.Queries, db *sql.DB, wallet *WalletService, settings *SettingsService, verifier ReceiptVerifier) *DepositService {
	return &DepositService{q: q, db: db, wallet: wallet, settings: settings, verifier: verifier}
}

// verifyContext bounds one verification. A verifier that knows how long its
// own retries take says so; anything else gets the fallback.
func (s *DepositService) verifyContext(ctx context.Context) (context.Context, context.CancelFunc) {
	budget := depositVerifyTimeout
	if b, ok := s.verifier.(interface{ Budget() time.Duration }); ok {
		if d := b.Budget(); d > 0 {
			budget = d
		}
	}
	return context.WithTimeout(ctx, budget)
}

func (s *DepositService) Config(ctx context.Context) (DepositConfig, error) {
	settings, err := s.settings.Get(ctx)
	if err != nil {
		return DepositConfig{}, err
	}
	return DepositConfig{
		RealDepositsEnabled: settings.RealDepositsEnabled,
		AccountName:         settings.DepositAccountName,
		AccountNumber:       settings.DepositAccountNumber,
	}, nil
}

// SubmitReceipt is the whole user-facing deposit path when real deposits are
// on: pull the link out of the pasted SMS, ask the verifier what it is, check
// the money actually arrived in our account, and credit it once.
func (s *DepositService) SubmitReceipt(ctx context.Context, userID int64, pasted string) (*DepositOutcome, error) {
	settings, err := s.settings.Get(ctx)
	if err != nil {
		return nil, err
	}
	if !settings.RealDepositsEnabled {
		return nil, ErrRealDepositsDisabled
	}
	if strings.TrimSpace(settings.DepositAccountName) == "" || strings.TrimSpace(settings.DepositAccountNumber) == "" {
		return nil, ErrDepositAccountUnset
	}

	receiptURL, err := utilities.ExtractCBEReceiptURL(pasted)
	if err != nil {
		return nil, err
	}

	// A URL already on file is answered from the row rather than by asking the
	// verifier again — the answer cannot change, and the row says what
	// happened to it.
	if existing, err := s.q.GetBankDepositByURL(ctx, receiptURL); err == nil {
		switch existing.Status {
		case DepositStatusCredited:
			return nil, ErrReceiptAlreadyClaimed
		case DepositStatusPendingReview:
			return nil, ErrReceiptQueued
		default:
			if existing.Note != "" {
				return nil, errors.New(existing.Note)
			}
			return nil, ErrReceiptAlreadyClaimed
		}
	}

	verifyCtx, cancel := s.verifyContext(ctx)
	defer cancel()

	receipt, err := s.verifier.Verify(verifyCtx, receiptURL)
	if err != nil {
		if errors.Is(err, ErrVerifierUnavailable) {
			// The receipt is unjudged, not bad. Keep it so the user does not
			// have to still have the SMS when the verifier comes back.
			log.Printf("deposit: verifier unavailable for user %d: %v", userID, err)
			return s.queueForReview(ctx, userID, receiptURL, err)
		}
		log.Printf("deposit: receipt rejected for user %d: %v", userID, err)
		return nil, ErrReceiptRejected
	}

	if err := validateReceipt(receipt, settings); err != nil {
		return nil, err
	}
	return s.credit(ctx, userID, receiptURL, receipt, 0)
}

// queueForReview stores an unverifiable paste for an admin to resolve. It
// carries no reference — nothing has read the bank's side of it — which is
// exactly why it cannot credit anything on its own.
func (s *DepositService) queueForReview(ctx context.Context, userID int64, receiptURL string, cause error) (*DepositOutcome, error) {
	// The cause is kept on the row, not just in the log. "Verifier
	// unreachable" is the one note an admin has to act on without being able
	// to reproduce it later, and a timeout, a 502 and a DNS failure call for
	// three different fixes.
	note := "verifier unreachable at submission"
	if cause != nil {
		note = clip("verifier unreachable at submission: "+cause.Error(), 255)
	}
	_, err := s.q.CreateBankDeposit(ctx, repository.CreateBankDepositParams{
		UserID:     userID,
		Provider:   "cbe",
		ReceiptUrl: receiptURL,
		Status:     DepositStatusPendingReview,
		Note:       note,
	})
	if err != nil {
		if isUniqueViolation(err) {
			return nil, ErrReceiptQueued
		}
		return nil, err
	}
	return &DepositOutcome{
		Status:  DepositStatusPendingReview,
		Message: "We couldn't reach the bank verifier just now, so your receipt is queued for review. Your money is safe — you'll be credited shortly, and there's no need to send it again.",
	}, nil
}

// credit claims the receipt and moves the money in one transaction. depositID
// is 0 for a fresh submission and the existing row for one being resolved out
// of the review queue.
//
// The claim is the INSERT/UPDATE itself: the UNIQUE on reference is what makes
// "has this transfer been deposited before?" a question the database answers
// under concurrency, rather than one this code answers with a SELECT that two
// requests can both pass.
func (s *DepositService) credit(ctx context.Context, userID int64, receiptURL string, receipt *Receipt, depositID int64) (*DepositOutcome, error) {
	return s.creditRow(ctx, creditRequest{
		userID:       userID,
		depositID:    depositID,
		receiptURL:   receiptURL,
		provider:     providerOrDefault(receipt.Provider),
		reference:    receipt.Reference,
		amount:       creditableAmount(receipt.Amount),
		payerName:    receipt.Payer.Name,
		payerAccount: receipt.Payer.Account,
		reason:       "cbe_deposit",
	})
}

// creditRequest is one crediting, however it was decided. Both the verified
// path and an admin's manual credit go through it so there is exactly one
// piece of code that moves money against a deposit row.
type creditRequest struct {
	userID    int64
	depositID int64 // 0 for a fresh submission, else the queued row being resolved
	// reference is the bank's transfer id. Empty means there is none — a
	// manual credit where the admin could not read one off the receipt — and
	// it is stored as NULL, not "", so it neither collides with another
	// referenceless row nor claims one.
	reference    string
	receiptURL   string
	provider     string
	amount       int64
	payerName    string
	payerAccount string
	note         string
	reason       string
}

func (s *DepositService) creditRow(ctx context.Context, req creditRequest) (*DepositOutcome, error) {
	if req.amount < MinReceiptDeposit {
		return nil, ErrReceiptTooSmall
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	qtx := s.q.WithTx(tx)

	walletTx, err := s.wallet.DepositWith(ctx, qtx, req.userID, req.amount, req.reason)
	if err != nil {
		return nil, err
	}

	reference := sql.NullString{String: req.reference, Valid: req.reference != ""}
	transactionID := sql.NullString{String: walletTx.TransactionID, Valid: true}

	if req.depositID == 0 {
		_, err = qtx.CreateBankDeposit(ctx, repository.CreateBankDepositParams{
			UserID:        req.userID,
			Provider:      providerOrDefault(req.provider),
			ReceiptUrl:    req.receiptURL,
			Reference:     reference,
			Amount:        req.amount,
			PayerName:     clip(req.payerName, 128),
			PayerAccount:  clip(req.payerAccount, 32),
			Status:        DepositStatusCredited,
			Note:          clip(req.note, 255),
			TransactionID: transactionID,
			ResolvedAt:    time.Now().Unix(),
		})
	} else {
		err = qtx.ResolveBankDeposit(ctx, repository.ResolveBankDepositParams{
			Reference:     reference,
			Amount:        req.amount,
			PayerName:     clip(req.payerName, 128),
			PayerAccount:  clip(req.payerAccount, 32),
			Status:        DepositStatusCredited,
			Note:          clip(req.note, 255),
			TransactionID: transactionID,
			ResolvedAt:    time.Now().Unix(),
			ID:            req.depositID,
		})
	}
	if err != nil {
		if isUniqueViolation(err) {
			// Another row already holds this reference — the same transfer,
			// reached through its other receipt link, or two submissions
			// racing. Nothing is credited: the rollback takes the wallet
			// change with it.
			return nil, ErrReceiptAlreadyClaimed
		}
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}

	return &DepositOutcome{
		Status:        DepositStatusCredited,
		Amount:        req.amount,
		Reference:     req.reference,
		TransactionID: walletTx.TransactionID,
		Message:       fmt.Sprintf("Deposited %d ETB.", req.amount),
	}, nil
}

// PendingReview lists the queue an admin works through.
func (s *DepositService) PendingReview(ctx context.Context, page int) ([]repository.ListBankDepositsForReviewRow, int64, error) {
	if page < 1 {
		page = 1
	}
	total, err := s.q.CountBankDepositsByStatus(ctx, DepositStatusPendingReview)
	if err != nil {
		return nil, 0, err
	}
	rows, err := s.q.ListBankDepositsForReview(ctx, repository.ListBankDepositsForReviewParams{
		Status: DepositStatusPendingReview,
		Limit:  int32(DepositReviewPageSize),
		Offset: int32((page - 1) * DepositReviewPageSize),
	})
	if err != nil {
		return nil, 0, err
	}
	return rows, total, nil
}

// ApproveQueued re-runs verification on a queued receipt and credits it if the
// bank confirms it. Approval deliberately cannot mint money on an admin's
// say-so: the checks that guard a normal deposit — receiver, status, and above
// all the one-reference-one-credit rule — are the same ones here, because a
// receipt that skipped them is a receipt that can be spent twice.
func (s *DepositService) ApproveQueued(ctx context.Context, depositID int64) (*DepositOutcome, error) {
	row, err := s.q.GetBankDepositByID(ctx, depositID)
	if err != nil {
		return nil, err
	}
	if row.Status != DepositStatusPendingReview {
		return nil, ErrDepositNotPending
	}
	settings, err := s.settings.Get(ctx)
	if err != nil {
		return nil, err
	}

	verifyCtx, cancel := s.verifyContext(ctx)
	defer cancel()

	receipt, err := s.verifier.Verify(verifyCtx, row.ReceiptUrl)
	if err != nil {
		if errors.Is(err, ErrVerifierUnavailable) {
			// Still down. The row stays queued rather than being resolved
			// either way — but the reason is kept, because "still unreachable"
			// and "the CDN is blocking us" call for very different next steps.
			log.Printf("deposit: manual re-check of receipt %d could not reach the verifier: %v", row.ID, err)
			_ = s.q.NoteBankDepositAttempt(ctx, repository.NoteBankDepositAttemptParams{
				Note: clip("re-checked by an administrator, still unresolved: "+err.Error(), 255),
				ID:   row.ID,
			})
			return nil, err
		}
		_ = s.rejectRow(ctx, row.ID, "bank rejected this receipt link")
		return nil, ErrReceiptRejected
	}

	if err := validateReceipt(receipt, settings); err != nil {
		_ = s.rejectRow(ctx, row.ID, err.Error())
		return nil, err
	}
	return s.credit(ctx, row.UserID, row.ReceiptUrl, receipt, row.ID)
}

// CreditManually credits a queued receipt on an admin's word, with the amount
// typed in by hand. It exists for the case ApproveQueued cannot solve: the
// player really did transfer the money, but the verifier cannot read that
// particular receipt — a link the bank has aged out, a receipt format the
// scraper chokes on, an outage that outlasts the automatic retries.
//
// This is the one path where money moves without the bank confirming it, so
// what it can skip is drawn narrowly. The row must still be one a player
// actually submitted and still be waiting, the amount is bounded, and the
// admin's name and the fact that no bank check happened are written onto the
// row rather than being left to memory.
//
// reference is optional but wanted: it is the FT number printed on the receipt
// and in the SMS, and passing it keeps the one-reference-one-credit rule
// working, so the same transfer cannot be credited a second time through its
// other receipt link once the verifier recovers. Credited without one, the
// receipt URL is the only thing standing between this transfer and a second
// credit — which is why the note says so.
func (s *DepositService) CreditManually(ctx context.Context, depositID, amount int64, reference, adminName string) (*DepositOutcome, error) {
	if amount < MinReceiptDeposit || amount > MaxManualDeposit {
		return nil, ErrManualAmountInvalid
	}

	row, err := s.q.GetBankDepositByID(ctx, depositID)
	if err != nil {
		return nil, err
	}
	if row.Status != DepositStatusPendingReview {
		return nil, ErrDepositNotPending
	}

	reference = normalizeReference(reference)
	who := strings.TrimSpace(adminName)
	if who == "" {
		who = "an administrator"
	}
	note := fmt.Sprintf("credited manually by %s — amount entered by hand, bank check skipped", who)
	if reference == "" {
		note += ", no bank reference"
	}

	outcome, err := s.creditRow(ctx, creditRequest{
		userID:       row.UserID,
		depositID:    row.ID,
		reference:    reference,
		receiptURL:   row.ReceiptUrl,
		provider:     row.Provider,
		amount:       amount,
		payerName:    row.PayerName,
		payerAccount: row.PayerAccount,
		note:         note,
		reason:       "cbe_deposit_manual",
	})
	if err != nil {
		return nil, err
	}
	log.Printf("deposit: %s manually credited %d ETB to user %d on receipt %d (reference %q)",
		who, amount, row.UserID, row.ID, reference)
	return outcome, nil
}

// normalizeReference puts a hand-typed FT number into the shape the verifier
// reports, so a manual credit and a later automatic one collide on the UNIQUE
// index instead of both going through.
func normalizeReference(reference string) string {
	reference = strings.ToUpper(strings.TrimSpace(reference))
	reference = strings.Join(strings.Fields(reference), "")
	return clip(reference, 64)
}

// RejectQueued closes a queued receipt without crediting it.
func (s *DepositService) RejectQueued(ctx context.Context, depositID int64, note string) error {
	row, err := s.q.GetBankDepositByID(ctx, depositID)
	if err != nil {
		return err
	}
	if row.Status != DepositStatusPendingReview {
		return ErrDepositNotPending
	}
	if strings.TrimSpace(note) == "" {
		note = "rejected by an administrator"
	}
	return s.rejectRow(ctx, row.ID, note)
}

func (s *DepositService) rejectRow(ctx context.Context, id int64, note string) error {
	return s.q.ResolveBankDeposit(ctx, repository.ResolveBankDepositParams{
		Reference:  sql.NullString{},
		Amount:     0,
		Status:     DepositStatusRejected,
		Note:       clip(note, 255),
		ResolvedAt: time.Now().Unix(),
		ID:         id,
	})
}

// validateReceipt answers the only question that matters: did this transfer
// actually put money into our account? A receipt is a public record of
// somebody's transfer — it proves a transfer happened, not that it happened to
// us — so the receiver is checked against the configured account every time.
func validateReceipt(receipt *Receipt, settings repository.SiteSetting) error {
	if !strings.EqualFold(strings.TrimSpace(receipt.Status), "completed") {
		return ErrReceiptNotCompleted
	}
	if receipt.Currency != "" && !strings.EqualFold(strings.TrimSpace(receipt.Currency), "ETB") {
		return ErrReceiptWrongCurrency
	}
	if !namesMatch(settings.DepositAccountName, receipt.Receiver.Name) {
		return ErrReceiptWrongAccount
	}
	if !accountMatches(settings.DepositAccountNumber, receipt.Receiver.Account) {
		return ErrReceiptWrongAccount
	}
	if creditableAmount(receipt.Amount) < MinReceiptDeposit {
		return ErrReceiptTooSmall
	}
	return nil
}

// creditableAmount converts the bank's decimal ETB to whole-ETB wallet units,
// rounding down. Rounding up would credit money that never arrived; the
// fraction lost is under one birr and is the user's own choice of amount.
func creditableAmount(amount float64) int64 {
	if amount <= 0 || math.IsNaN(amount) || math.IsInf(amount, 0) {
		return 0
	}
	// The epsilon is for amounts like 50.00 arriving as 49.999999 through
	// float parsing, which would otherwise be credited as 49.
	return int64(math.Floor(amount + 1e-6))
}

var nonNameChars = regexp.MustCompile(`[^\p{L}\p{N}]+`)

// namesMatch compares account holder names the way a person reads them: case
// and punctuation are noise, and the bank writes the name in a fixed order, so
// the tokens have to be the same set.
func namesMatch(configured, fromReceipt string) bool {
	want := nameTokens(configured)
	got := nameTokens(fromReceipt)
	if len(want) == 0 || len(got) == 0 {
		return false
	}
	if len(want) != len(got) {
		return false
	}
	for i := range want {
		if want[i] != got[i] {
			return false
		}
	}
	return true
}

func nameTokens(name string) []string {
	fields := nonNameChars.Split(strings.ToLower(strings.TrimSpace(name)), -1)
	tokens := make([]string, 0, len(fields))
	for _, f := range fields {
		if f != "" {
			tokens = append(tokens, f)
		}
	}
	return tokens
}

// accountMatches checks the configured account number against the masked form
// the bank prints on a receipt — "1********1758". Only the visible ends can be
// compared, so the length has to line up too: without it, a configured
// "1758" would match any masked number ending the same way.
func accountMatches(configured, masked string) bool {
	configured = strings.TrimSpace(configured)
	masked = strings.TrimSpace(masked)
	if configured == "" || masked == "" {
		return false
	}
	if !strings.Contains(masked, "*") {
		return strings.EqualFold(configured, masked)
	}
	if len(configured) != len(masked) {
		return false
	}
	// Compare position by position, treating every masked position as a match.
	for i := 0; i < len(masked); i++ {
		if masked[i] == '*' {
			continue
		}
		if masked[i] != configured[i] {
			return false
		}
	}
	return true
}

// clip cuts a value to what its column can hold. Unlike truncate it appends
// nothing: an ellipsis would push a value that just fits back over the limit,
// and these strings are stored, not read.
func clip(s string, n int) string {
	runes := []rune(s)
	if len(runes) <= n {
		return s
	}
	return string(runes[:n])
}

func providerOrDefault(provider string) string {
	provider = strings.TrimSpace(strings.ToLower(provider))
	if provider == "" {
		return "cbe"
	}
	return clip(provider, 16)
}

// isUniqueViolation recognizes the database refusing a duplicate. The
// distinction matters: a violation on the reference means "someone already
// claimed this transfer", which the user is told, while any other error is a
// failure that must not be reported as an already-spent receipt.
func isUniqueViolation(err error) bool {
	return repository.IsUniqueViolation(err)
}

// StartQueueWorker runs the retry sweep on a timer until the returned stop
// function is called or ctx is cancelled.
//
// This is what makes the review queue a fallback rather than the normal path:
// a receipt queued during an outage is credited on its own once the verifier
// comes back, and an admin only ever sees the ones that genuinely need a
// human. The first sweep runs immediately, which matters after a restart — the
// outage that queued those rows may well be what the restart fixed.
func (s *DepositService) StartQueueWorker(ctx context.Context, interval time.Duration) (stop func()) {
	if interval <= 0 {
		interval = QueueSweepInterval
	}
	ctx, cancel := context.WithCancel(ctx)

	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			s.RunQueueSweep(ctx)
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
		}
	}()

	return cancel
}

// QueueSweepResult reports one pass, for the log and for tests.
type QueueSweepResult struct {
	Credited  int
	Rejected  int
	Unchanged int
}

// RunQueueSweep re-checks queued receipts once. It is deliberately a plain
// method rather than something only the ticker can reach: a sweep is the same
// operation whether a timer or a test triggers it, and timing is a poor thing
// to write assertions against.
func (s *DepositService) RunQueueSweep(ctx context.Context) QueueSweepResult {
	var result QueueSweepResult

	settings, err := s.settings.Get(ctx)
	if err != nil {
		log.Printf("deposit sweep: could not read settings: %v", err)
		return result
	}
	// Nothing to do while deposits are play money, and re-checking receipts
	// against an account that is no longer configured would reject rows that
	// deserve a human.
	if !settings.RealDepositsEnabled || strings.TrimSpace(settings.DepositAccountNumber) == "" {
		return result
	}

	rows, err := s.q.ListBankDepositsToRetry(ctx, repository.ListBankDepositsToRetryParams{
		Status:        DepositStatusPendingReview,
		Attempts:      MaxQueueAttempts,
		LastAttemptAt: time.Now().Add(-queueRetryBackoff).Unix(),
		Limit:         queueSweepBatch,
	})
	if err != nil {
		log.Printf("deposit sweep: could not list queued receipts: %v", err)
		return result
	}

	for _, row := range rows {
		select {
		case <-ctx.Done():
			return result
		default:
		}

		switch s.retryQueued(ctx, row, settings) {
		case DepositStatusCredited:
			result.Credited++
		case DepositStatusRejected:
			result.Rejected++
		default:
			result.Unchanged++
		}
	}

	if result.Credited > 0 || result.Rejected > 0 {
		log.Printf("deposit sweep: %d credited, %d rejected, %d still waiting",
			result.Credited, result.Rejected, result.Unchanged)
	}
	return result
}

// retryQueued re-checks a single queued receipt and returns the status it
// ended in. Every attempt is recorded first: a crash mid-check must not leave
// the row looking untouched, or a receipt the verifier chokes on would be
// retried on every sweep forever.
func (s *DepositService) retryQueued(ctx context.Context, row repository.BankDeposit, settings repository.SiteSetting) string {
	attemptNote := fmt.Sprintf("automatic re-check %d of %d", row.Attempts+1, MaxQueueAttempts)
	if err := s.q.TouchBankDepositAttempt(ctx, repository.TouchBankDepositAttemptParams{
		LastAttemptAt: time.Now().Unix(),
		Note:          attemptNote,
		ID:            row.ID,
	}); err != nil {
		log.Printf("deposit sweep: could not record attempt on %d: %v", row.ID, err)
		return DepositStatusPendingReview
	}

	verifyCtx, cancel := s.verifyContext(ctx)
	defer cancel()

	receipt, err := s.verifier.Verify(verifyCtx, row.ReceiptUrl)
	if err != nil {
		if errors.Is(err, ErrVerifierUnavailable) {
			// Still down. Leave it queued for the next sweep, with the reason
			// on the row: a queue full of rows that all say the same thing is
			// how a blocked server is told apart from a slow one.
			log.Printf("deposit sweep: receipt %d still unresolved (%s): %v", row.ID, attemptNote, err)
			_ = s.q.NoteBankDepositAttempt(ctx, repository.NoteBankDepositAttemptParams{
				Note: clip(attemptNote+" — "+err.Error(), 255),
				ID:   row.ID,
			})
			return DepositStatusPendingReview
		}
		// The bank has now answered, and the answer is no. That is a verdict,
		// so the row is closed rather than retried until the cap runs out.
		log.Printf("deposit sweep: bank rejected receipt %d: %v", row.ID, err)
		_ = s.rejectRow(ctx, row.ID, "the bank did not recognize this receipt link")
		return DepositStatusRejected
	}

	if err := validateReceipt(receipt, settings); err != nil {
		log.Printf("deposit sweep: receipt %d failed validation: %v", row.ID, err)
		_ = s.rejectRow(ctx, row.ID, err.Error())
		return DepositStatusRejected
	}

	if _, err := s.credit(ctx, row.UserID, row.ReceiptUrl, receipt, row.ID); err != nil {
		if errors.Is(err, ErrReceiptAlreadyClaimed) {
			// The same transfer was credited through its other receipt link
			// while this row sat in the queue.
			_ = s.rejectRow(ctx, row.ID, ErrReceiptAlreadyClaimed.Error())
			return DepositStatusRejected
		}
		log.Printf("deposit sweep: could not credit receipt %d: %v", row.ID, err)
		return DepositStatusPendingReview
	}
	return DepositStatusCredited
}
