package handlers

import (
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/zuse/poker5/go-poker/repository"
	"github.com/zuse/poker5/go-poker/services"
	"github.com/zuse/poker5/go-poker/utilities"
)

// AdminHandler serves the JSON API behind the React admin dashboard. Every
// route is mounted behind RequireAdmin.
type AdminHandler struct {
	admin       *services.AdminService
	settings    *services.SettingsService
	auth        *services.AuthService
	deposits    *services.DepositService
	withdrawals *services.WithdrawalService
}

func NewAdminHandler(admin *services.AdminService, settings *services.SettingsService, auth *services.AuthService, deposits *services.DepositService, withdrawals *services.WithdrawalService) *AdminHandler {
	return &AdminHandler{admin: admin, settings: settings, auth: auth, deposits: deposits, withdrawals: withdrawals}
}

// RequireAdmin is Fiber middleware that only lets users with role=admin
// through. Signed out is a 401 and signed in as a player is a 403, so the
// dashboard can tell "log in" apart from "you are not allowed here".
func (h *AdminHandler) RequireAdmin(c fiber.Ctx) error {
	token := c.Cookies("poker_session")
	if token == "" {
		token = c.Get("Authorization")
	}

	user, err := h.auth.GetUserByToken(c.Context(), token)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "unauthorized"})
	}
	if user.Role != "admin" {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "admin access required"})
	}

	c.Locals("admin_user", user)
	return c.Next()
}

func adminName(c fiber.Ctx) string {
	if user, ok := c.Locals("admin_user").(*repository.User); ok {
		return user.Username
	}
	return ""
}

func jsonError(c fiber.Ctx, status int, err error) error {
	return c.Status(status).JSON(fiber.Map{"error": err.Error()})
}

func pathID(c fiber.Ctx) (int64, bool) {
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	return id, err == nil && id > 0
}

func queryPage(c fiber.Ctx) int {
	page, _ := strconv.Atoi(c.Query("page", "1"))
	if page < 1 {
		page = 1
	}
	return page
}

func pageJSON(items any, total int64, page, pageSize int) fiber.Map {
	return fiber.Map{"items": items, "total": total, "page": page, "page_size": pageSize}
}

// unixTime turns the BIGINT unix columns into a JSON time, with 0 — "not yet"
// — as null rather than 1970.
func unixTime(seconds int64) *time.Time {
	if seconds == 0 {
		return nil
	}
	t := time.Unix(seconds, 0).UTC()
	return &t
}

// --- Overview -------------------------------------------------------------

func (h *AdminHandler) Overview(c fiber.Ctx) error {
	ctx := c.Context()

	earnings, err := h.admin.GetEarnings(ctx)
	if err != nil {
		return jsonError(c, fiber.StatusInternalServerError, errors.New("failed to load earnings"))
	}
	_, pendingDeposits, err := h.deposits.PendingReview(ctx, 1)
	if err != nil {
		return jsonError(c, fiber.StatusInternalServerError, errors.New("failed to load deposits"))
	}
	pendingWithdrawals, err := h.withdrawals.PendingCount(ctx)
	if err != nil {
		return jsonError(c, fiber.StatusInternalServerError, errors.New("failed to load withdrawals"))
	}
	pendingWithdrawalTotal, err := h.withdrawals.PendingTotal(ctx)
	if err != nil {
		return jsonError(c, fiber.StatusInternalServerError, errors.New("failed to load withdrawals"))
	}
	_, users, err := h.admin.SearchUsers(ctx, "", 1)
	if err != nil {
		return jsonError(c, fiber.StatusInternalServerError, errors.New("failed to load users"))
	}

	return c.JSON(fiber.Map{
		"earnings": fiber.Map{
			"house_balance":       earnings.HouseBalance,
			"rake_today":          earnings.RakeToday,
			"rake_week":           earnings.RakeWeek,
			"rake_month":          earnings.RakeMonth,
			"referral_paid_today": earnings.ReferralPaidToday,
			"referral_paid_week":  earnings.ReferralPaidWeek,
			"referral_paid_month": earnings.ReferralPaidMonth,
		},
		"pending_deposits":         pendingDeposits,
		"pending_withdrawals":      pendingWithdrawals,
		"pending_withdrawal_total": pendingWithdrawalTotal,
		"total_users":              users,
	})
}

// --- Settings -------------------------------------------------------------

func settingsJSON(s repository.SiteSetting) fiber.Map {
	return fiber.Map{
		"rake_mode":                       s.RakeMode,
		"rake_percentage":                 s.RakePercentage,
		"referral_percentage_pct_mode":    s.ReferralPercentagePctMode,
		"referral_percentage_sb_mode":     s.ReferralPercentageSbMode,
		"countdown_seconds":               s.CountdownSeconds,
		"intermission_seconds":            s.IntermissionSeconds,
		"real_deposits_enabled":           s.RealDepositsEnabled,
		"deposit_account_name":            s.DepositAccountName,
		"deposit_account_number":          s.DepositAccountNumber,
		"gateway_deposits_enabled":        s.GatewayDepositsEnabled,
		"withdrawals_enabled":             s.WithdrawalsEnabled,
		"withdrawal_min_amount":           s.WithdrawalMinAmount,
		"withdrawal_cancel_enabled":       s.WithdrawalCancelEnabled,
		"withdrawal_reset_cooldown_hours": s.WithdrawalResetCooldownHours,
		"public_refill_enabled":           s.PublicRefillEnabled,
		"updated_at":                      s.UpdatedAt,
	}
}

// settingsLimits travels with the settings so the form validates against the
// same bounds the server enforces, rather than a copy that can drift.
var settingsLimits = fiber.Map{
	"rake_percentage":                 fiber.Map{"min": services.MinRakePercentage, "max": services.MaxRakePercentage},
	"referral_percentage":             fiber.Map{"min": services.MinReferralPercentage, "max": services.MaxReferralPercentage},
	"countdown_seconds":               fiber.Map{"min": services.MinCountdownSeconds, "max": services.MaxCountdownSeconds},
	"intermission_seconds":            fiber.Map{"min": services.MinIntermissionSeconds, "max": services.MaxIntermissionSeconds},
	"deposit_account_name":            fiber.Map{"max_length": services.MaxDepositAccountName},
	"deposit_account_number":          fiber.Map{"max_length": services.MaxDepositAccountNumber},
	"withdrawal_min_amount":           fiber.Map{"min": services.MinWithdrawalMinAmount, "max": services.MaxWithdrawalMinAmount},
	"withdrawal_reset_cooldown_hours": fiber.Map{"min": 0, "max": services.MaxWithdrawalCooldownHours},
}

func (h *AdminHandler) GetSettings(c fiber.Ctx) error {
	s, err := h.settings.Get(c.Context())
	if err != nil {
		return jsonError(c, fiber.StatusInternalServerError, errors.New("failed to load settings"))
	}
	return c.JSON(fiber.Map{"settings": settingsJSON(s), "limits": settingsLimits})
}

func (h *AdminHandler) UpdateSettings(c fiber.Ctx) error {
	var req struct {
		RakeMode                     string  `json:"rake_mode"`
		RakePercentage               float64 `json:"rake_percentage"`
		ReferralPercentagePctMode    float64 `json:"referral_percentage_pct_mode"`
		ReferralPercentageSbMode     float64 `json:"referral_percentage_sb_mode"`
		CountdownSeconds             int32   `json:"countdown_seconds"`
		IntermissionSeconds          int32   `json:"intermission_seconds"`
		RealDepositsEnabled          bool    `json:"real_deposits_enabled"`
		DepositAccountName           string  `json:"deposit_account_name"`
		DepositAccountNumber         string  `json:"deposit_account_number"`
		GatewayDepositsEnabled       bool    `json:"gateway_deposits_enabled"`
		WithdrawalsEnabled           bool    `json:"withdrawals_enabled"`
		WithdrawalMinAmount          int64   `json:"withdrawal_min_amount"`
		WithdrawalCancelEnabled      bool    `json:"withdrawal_cancel_enabled"`
		WithdrawalResetCooldownHours int32   `json:"withdrawal_reset_cooldown_hours"`
		PublicRefillEnabled          bool    `json:"public_refill_enabled"`
	}
	if err := c.Bind().JSON(&req); err != nil {
		return jsonError(c, fiber.StatusBadRequest, errors.New("invalid settings payload"))
	}

	err := h.settings.Update(c.Context(), services.UpdateSettingsRequest{
		RakeMode:                     req.RakeMode,
		RakePercentage:               req.RakePercentage,
		ReferralPercentagePctMode:    req.ReferralPercentagePctMode,
		ReferralPercentageSbMode:     req.ReferralPercentageSbMode,
		CountdownSeconds:             req.CountdownSeconds,
		IntermissionSeconds:          req.IntermissionSeconds,
		RealDepositsEnabled:          req.RealDepositsEnabled,
		DepositAccountName:           req.DepositAccountName,
		DepositAccountNumber:         req.DepositAccountNumber,
		GatewayDepositsEnabled:       req.GatewayDepositsEnabled,
		WithdrawalsEnabled:           req.WithdrawalsEnabled,
		WithdrawalMinAmount:          req.WithdrawalMinAmount,
		WithdrawalCancelEnabled:      req.WithdrawalCancelEnabled,
		WithdrawalResetCooldownHours: req.WithdrawalResetCooldownHours,
		PublicRefillEnabled:          req.PublicRefillEnabled,
	})
	if err != nil {
		return jsonError(c, fiber.StatusBadRequest, err)
	}
	return h.GetSettings(c)
}

// --- Users ----------------------------------------------------------------

// adminUserJSON is the one shape a user leaves the admin API in. Building it
// field by field is what keeps password_hash from ever being serialized.
func adminUserJSON(u repository.User) fiber.Map {
	return fiber.Map{
		"id":            u.ID,
		"username":      u.Username,
		"phone_number":  u.PhoneNumber,
		"wallet":        u.Wallet,
		"referral_code": u.ReferralCode,
		"referred_by":   u.ReferredBy.String,
		"role":          u.Role,
		"created_at":    u.CreatedAt,
	}
}

func (h *AdminHandler) SearchUsers(c fiber.Ctx) error {
	page := queryPage(c)
	users, total, err := h.admin.SearchUsers(c.Context(), strings.TrimSpace(c.Query("query")), page)
	if err != nil {
		return jsonError(c, fiber.StatusInternalServerError, errors.New("failed to load users"))
	}
	items := make([]fiber.Map, 0, len(users))
	for _, u := range users {
		items = append(items, adminUserJSON(u))
	}
	return c.JSON(pageJSON(items, total, page, services.AdminListPageSize))
}

func (h *AdminHandler) GetUser(c fiber.Ctx) error {
	id, ok := pathID(c)
	if !ok {
		return jsonError(c, fiber.StatusBadRequest, errors.New("bad user id"))
	}
	detail, err := h.admin.GetUser(c.Context(), id)
	if errors.Is(err, sql.ErrNoRows) {
		return jsonError(c, fiber.StatusNotFound, errors.New("user not found"))
	}
	if err != nil {
		return jsonError(c, fiber.StatusInternalServerError, errors.New("failed to load user"))
	}

	txs := make([]fiber.Map, 0, len(detail.Transactions))
	for _, t := range detail.Transactions {
		txs = append(txs, fiber.Map{
			"id":             t.ID,
			"amount":         t.Amount,
			"type":           t.Type,
			"reason":         t.Reason,
			"transaction_id": t.TransactionID,
			"created_at":     t.CreatedAt,
		})
	}
	return c.JSON(fiber.Map{"user": adminUserJSON(detail.User), "transactions": txs})
}

// --- Transactions ---------------------------------------------------------

func (h *AdminHandler) SearchTransactions(c fiber.Ctx) error {
	page := queryPage(c)
	rows, total, err := h.admin.FilterTransactions(c.Context(), strings.TrimSpace(c.Query("query")), c.Query("type"), page)
	if err != nil {
		return jsonError(c, fiber.StatusInternalServerError, errors.New("failed to load transactions"))
	}
	items := make([]fiber.Map, 0, len(rows))
	for _, r := range rows {
		items = append(items, fiber.Map{
			"id":             r.ID,
			"user_id":        r.UserID,
			"username":       r.Username,
			"type":           r.Type,
			"amount":         r.Amount,
			"reason":         r.Reason,
			"transaction_id": r.TransactionID,
			"created_at":     r.CreatedAt,
		})
	}
	return c.JSON(pageJSON(items, total, page, services.AdminListPageSize))
}

// --- Bank deposit review queue -------------------------------------------

func (h *AdminHandler) ListPendingDeposits(c fiber.Ctx) error {
	page := queryPage(c)
	rows, total, err := h.deposits.PendingReview(c.Context(), page)
	if err != nil {
		return jsonError(c, fiber.StatusInternalServerError, errors.New("failed to load deposits"))
	}
	items := make([]fiber.Map, 0, len(rows))
	for _, row := range rows {
		d := row.BankDeposit
		items = append(items, fiber.Map{
			"id":           d.ID,
			"user_id":      d.UserID,
			"username":     row.Username,
			"receipt_url":  d.ReceiptUrl,
			"note":         d.Note,
			"attempts":     d.Attempts,
			"gave_up":      d.Attempts >= services.MaxQueueAttempts,
			"submitted_at": d.CreatedAt,
		})
	}
	out := pageJSON(items, total, page, services.DepositReviewPageSize)
	out["manual_min"] = services.MinReceiptDeposit
	out["manual_max"] = services.MaxManualDeposit
	return c.JSON(out)
}

// adminDepositStatus maps a refused review action onto a status code. The
// message is what the admin reads; the code is what tells "the bank said no"
// apart from "the bank could not be asked".
func adminDepositStatus(err error) int {
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return fiber.StatusNotFound
	case errors.Is(err, services.ErrDepositNotPending):
		return fiber.StatusConflict
	case errors.Is(err, services.ErrVerifierBlocked), errors.Is(err, services.ErrVerifierUnavailable):
		return fiber.StatusBadGateway
	case errors.Is(err, services.ErrManualAmountInvalid):
		return fiber.StatusBadRequest
	}
	if status := depositErrorStatus(err); status != fiber.StatusInternalServerError {
		return status
	}
	return fiber.StatusBadRequest
}

// ApproveDeposit re-verifies a queued receipt and credits it if the bank
// confirms it. "Approve" is not a credit instruction: the receipt still has to
// pass every check a normal deposit does, so an admin cannot accidentally
// credit a receipt that was never paid to us or that has already been spent.
func (h *AdminHandler) ApproveDeposit(c fiber.Ctx) error {
	id, ok := pathID(c)
	if !ok {
		return jsonError(c, fiber.StatusBadRequest, errors.New("bad deposit id"))
	}

	outcome, err := h.deposits.ApproveQueued(c.Context(), id)
	switch {
	case err == nil:
		return c.JSON(fiber.Map{"message": fmt.Sprintf("Credited %d ETB (reference %s).", outcome.Amount, outcome.Reference)})
	case errors.Is(err, services.ErrVerifierBlocked):
		// This one is not "try again shortly": retrying changes nothing until
		// somebody edits a WAF rule, so the message says what to go and do.
		return jsonError(c, fiber.StatusBadGateway, fmt.Errorf("not credited — the call never reached the verifier: %w", err))
	case errors.Is(err, services.ErrVerifierUnavailable):
		return jsonError(c, fiber.StatusBadGateway, fmt.Errorf("the verifier is still unreachable — the receipt stays in the queue, try again shortly (%w)", err))
	default:
		return jsonError(c, adminDepositStatus(err), fmt.Errorf("not credited: %w", err))
	}
}

// CreditDeposit credits a queued receipt on the admin's own figure, for the
// receipts the verifier cannot read at all. Unlike ApproveDeposit this does
// take the admin at their word about the amount, which is why the row records
// who did it.
func (h *AdminHandler) CreditDeposit(c fiber.Ctx) error {
	id, ok := pathID(c)
	if !ok {
		return jsonError(c, fiber.StatusBadRequest, errors.New("bad deposit id"))
	}
	var body struct {
		Amount    int64  `json:"amount"`
		Reference string `json:"reference"`
	}
	if err := c.Bind().JSON(&body); err != nil || body.Amount <= 0 {
		return jsonError(c, fiber.StatusBadRequest, services.ErrManualAmountInvalid)
	}

	outcome, err := h.deposits.CreditManually(c.Context(), id, body.Amount, body.Reference, adminName(c))
	if err != nil {
		return jsonError(c, adminDepositStatus(err), fmt.Errorf("not credited: %w", err))
	}
	return c.JSON(fiber.Map{"message": fmt.Sprintf("Credited %d ETB by hand — no bank check was performed.", outcome.Amount)})
}

func (h *AdminHandler) RejectDeposit(c fiber.Ctx) error {
	id, ok := pathID(c)
	if !ok {
		return jsonError(c, fiber.StatusBadRequest, errors.New("bad deposit id"))
	}
	var body struct {
		Note string `json:"note"`
	}
	_ = c.Bind().JSON(&body)

	if err := h.deposits.RejectQueued(c.Context(), id, body.Note); err != nil {
		return jsonError(c, adminDepositStatus(err), fmt.Errorf("could not reject: %w", err))
	}
	return c.JSON(fiber.Map{"message": "Deposit rejected."})
}

// --- Withdrawals ----------------------------------------------------------

func withdrawalJSON(w repository.Withdrawal) fiber.Map {
	return fiber.Map{
		"id":                    w.ID,
		"user_id":               w.UserID,
		"method":                w.Method,
		"account_number":        w.AccountNumber,
		"account_name":          w.AccountName,
		"amount":                w.Amount,
		"status":                w.Status,
		"note":                  w.Note,
		"payout_reference":      w.PayoutReference,
		"resolved_by":           w.ResolvedBy,
		"debit_transaction_id":  w.DebitTransactionID,
		"refund_transaction_id": w.RefundTransactionID.String,
		"created_at":            w.CreatedAt,
		"resolved_at":           unixTime(w.ResolvedAt),
	}
}

func withdrawalErrorStatus(err error) int {
	var tooSmall services.WithdrawalTooSmallError
	var cooldown services.WithdrawalCooldownError
	switch {
	case errors.Is(err, services.ErrWithdrawalNotFound):
		return fiber.StatusNotFound
	case errors.Is(err, services.ErrWithdrawalNotPending), errors.Is(err, services.ErrWithdrawalPending):
		return fiber.StatusConflict
	case errors.Is(err, services.ErrWithdrawalsDisabled), errors.Is(err, services.ErrWithdrawalCancelDisabled),
		errors.As(err, &cooldown):
		return fiber.StatusForbidden
	case errors.Is(err, services.ErrInsufficientBalance), errors.As(err, &tooSmall),
		errors.Is(err, services.ErrWithdrawalMethodInvalid), errors.Is(err, services.ErrWithdrawalNameRequired),
		errors.Is(err, services.ErrWithdrawalNameTooLong), errors.Is(err, services.ErrWithdrawalReasonRequired),
		errors.Is(err, services.ErrWithdrawalStatusInvalid), errors.Is(err, services.ErrWithdrawalAmountInvalid),
		errors.Is(err, utilities.ErrInvalidCBEAccount), errors.Is(err, utilities.ErrInvalidTelebirrAccount):
		return fiber.StatusBadRequest
	default:
		return fiber.StatusInternalServerError
	}
}

func (h *AdminHandler) ListWithdrawals(c fiber.Ctx) error {
	page := queryPage(c)
	userID, _ := strconv.ParseInt(c.Query("user_id"), 10, 64)
	result, err := h.withdrawals.ListForAdmin(c.Context(), c.Query("status"), userID, page)
	if err != nil {
		return jsonError(c, withdrawalErrorStatus(err), err)
	}
	items := make([]fiber.Map, 0, len(result.Rows))
	for _, row := range result.Rows {
		item := withdrawalJSON(row.Withdrawal)
		item["username"] = row.Username
		item["phone_number"] = row.PhoneNumber
		items = append(items, item)
	}
	return c.JSON(pageJSON(items, result.Total, result.Page, services.WithdrawalPageSize))
}

func (h *AdminHandler) CompleteWithdrawal(c fiber.Ctx) error {
	id, ok := pathID(c)
	if !ok {
		return jsonError(c, fiber.StatusBadRequest, errors.New("bad withdrawal id"))
	}
	var body struct {
		Reference string `json:"reference"`
	}
	_ = c.Bind().JSON(&body)

	row, err := h.withdrawals.Complete(c.Context(), id, body.Reference, adminName(c))
	if err != nil {
		return jsonError(c, withdrawalErrorStatus(err), err)
	}
	return c.JSON(fiber.Map{"withdrawal": withdrawalJSON(*row)})
}

func (h *AdminHandler) RejectWithdrawal(c fiber.Ctx) error {
	id, ok := pathID(c)
	if !ok {
		return jsonError(c, fiber.StatusBadRequest, errors.New("bad withdrawal id"))
	}
	var body struct {
		Reason string `json:"reason"`
	}
	_ = c.Bind().JSON(&body)

	row, err := h.withdrawals.Reject(c.Context(), id, body.Reason, adminName(c))
	if err != nil {
		return jsonError(c, withdrawalErrorStatus(err), err)
	}
	return c.JSON(fiber.Map{"withdrawal": withdrawalJSON(*row)})
}
