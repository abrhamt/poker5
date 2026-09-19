package handlers

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/gofiber/fiber/v3"
	"github.com/zuse/poker5/go-poker/repository"
	"github.com/zuse/poker5/go-poker/services"
)

func filterTxRows(rows []repository.FilterTransactionsRow) []adminTxRow {
	out := make([]adminTxRow, 0, len(rows))
	for _, r := range rows {
		out = append(out, adminTxRow{
			Username:      r.Username,
			Type:          r.Type,
			Amount:        r.Amount,
			Reason:        r.Reason,
			TransactionID: r.TransactionID,
			CreatedAt:     r.CreatedAt.Format("Jan 02, 15:04"),
		})
	}
	return out
}

type AdminHandler struct {
	admin    *services.AdminService
	settings *services.SettingsService
	auth     *services.AuthService
	deposits *services.DepositService
}

func NewAdminHandler(admin *services.AdminService, settings *services.SettingsService, auth *services.AuthService, deposits *services.DepositService) *AdminHandler {
	return &AdminHandler{admin: admin, settings: settings, auth: auth, deposits: deposits}
}

// RequireAdmin is Fiber middleware that only lets users with role=admin
// through; everyone else is redirected to the normal login/lobby.
func (h *AdminHandler) RequireAdmin(c fiber.Ctx) error {
	token := c.Cookies("poker_session")
	if token == "" {
		token = c.Get("Authorization")
	}

	user, err := h.auth.GetUserByToken(c.Context(), token)
	if err != nil || user.Role != "admin" {
		if c.Get("HX-Request") == "true" {
			c.Set("HX-Redirect", "/login")
			return c.SendStatus(fiber.StatusUnauthorized)
		}
		return c.Redirect().To("/login")
	}

	c.Locals("admin_user", user)
	return c.Next()
}

func (h *AdminHandler) RenderDashboard(c fiber.Ctx) error {
	ctx := c.Context()

	earnings, err := h.admin.GetEarnings(ctx)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).SendString("failed to load earnings")
	}
	siteSettings, err := h.settings.Get(ctx)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).SendString("failed to load settings")
	}
	users, _ := h.admin.ListUsers(ctx, 1)
	txs, _ := h.admin.FilterTransactions(ctx, "", "", 1)
	pending, _, _ := h.deposits.PendingReview(ctx, 1)

	html := renderAdminDashboardHTML(earnings, siteSettings, users, filterTxRows(txs), pending)
	c.Set("Content-Type", "text/html; charset=utf-8")
	return c.SendString(html)
}

func (h *AdminHandler) UpdateSettings(c fiber.Ctx) error {
	var req struct {
		RakeMode                  string  `form:"rake_mode"`
		RakePercentage            float64 `form:"rake_percentage"`
		ReferralPercentagePctMode float64 `form:"referral_percentage_pct_mode"`
		ReferralPercentageSbMode  float64 `form:"referral_percentage_sb_mode"`
		CountdownSeconds          int     `form:"countdown_seconds"`
		RealDepositsEnabled       string  `form:"real_deposits_enabled"`
		DepositAccountName        string  `form:"deposit_account_name"`
		DepositAccountNumber      string  `form:"deposit_account_number"`
		GatewayDepositsEnabled    string  `form:"gateway_deposits_enabled"`
	}
	_ = c.Bind().Body(&req)

	err := h.settings.Update(c.Context(), services.UpdateSettingsRequest{
		RakeMode:                  req.RakeMode,
		RakePercentage:            req.RakePercentage,
		ReferralPercentagePctMode: req.ReferralPercentagePctMode,
		ReferralPercentageSbMode:  req.ReferralPercentageSbMode,
		CountdownSeconds:          int32(req.CountdownSeconds),
		RealDepositsEnabled:       req.RealDepositsEnabled == "true",
		DepositAccountName:        req.DepositAccountName,
		DepositAccountNumber:      req.DepositAccountNumber,
		GatewayDepositsEnabled:    req.GatewayDepositsEnabled == "true",
	})
	if err != nil {
		return c.Status(fiber.StatusBadRequest).SendString(`<div class="error-badge">` + err.Error() + `</div>`)
	}

	if c.Get("HX-Request") == "true" {
		c.Set("HX-Redirect", "/admin")
		return c.SendStatus(fiber.StatusOK)
	}
	return c.Redirect().To("/admin")
}

func (h *AdminHandler) SearchUsers(c fiber.Ctx) error {
	query := c.Query("query")
	page, _ := strconv.Atoi(c.Query("page", "1"))

	if query != "" {
		results, err := h.admin.SearchUsers(c.Context(), query, page)
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).SendString("search failed")
		}
		c.Set("Content-Type", "text/html; charset=utf-8")
		return c.SendString(renderAdminUsersTableHTML(results))
	}

	results, err := h.admin.ListUsers(c.Context(), page)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).SendString("list failed")
	}
	c.Set("Content-Type", "text/html; charset=utf-8")
	return c.SendString(renderAdminUsersTableHTML(results))
}

func (h *AdminHandler) SearchTransactions(c fiber.Ctx) error {
	query := c.Query("query")
	txType := c.Query("type")
	page, _ := strconv.Atoi(c.Query("page", "1"))

	results, err := h.admin.FilterTransactions(c.Context(), query, txType, page)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).SendString("search failed")
	}
	c.Set("Content-Type", "text/html; charset=utf-8")
	return c.SendString(renderAdminTransactionsTableHTML(filterTxRows(results)))
}

// ListPendingDeposits renders the review queue. It answers with the table
// fragment rather than JSON because the admin dashboard is server-rendered
// HTML driven by htmx, unlike the player-facing API.
func (h *AdminHandler) ListPendingDeposits(c fiber.Ctx) error {
	page, _ := strconv.Atoi(c.Query("page", "1"))
	rows, _, err := h.deposits.PendingReview(c.Context(), page)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).SendString("failed to load deposits")
	}
	c.Set("Content-Type", "text/html; charset=utf-8")
	return c.SendString(renderAdminDepositsTableHTML(rows, ""))
}

// ApproveDeposit re-verifies a queued receipt and credits it if the bank
// confirms it. "Approve" is not a credit instruction: the receipt still has to
// pass every check a normal deposit does, so an admin cannot accidentally
// credit a receipt that was never paid to us or that has already been spent.
func (h *AdminHandler) ApproveDeposit(c fiber.Ctx) error {
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).SendString("bad deposit id")
	}

	notice := ""
	outcome, err := h.deposits.ApproveQueued(c.Context(), id)
	switch {
	case err == nil:
		notice = fmt.Sprintf("Credited %d ETB (reference %s).", outcome.Amount, outcome.Reference)
	case errors.Is(err, services.ErrVerifierBlocked):
		// This one is not "try again shortly": retrying changes nothing until
		// somebody edits a WAF rule, so the notice says what to go and do.
		notice = "Not credited — the call never reached the verifier: " + err.Error()
	case errors.Is(err, services.ErrVerifierUnavailable):
		notice = "The verifier is still unreachable — the receipt stays in the queue. Try again shortly. (" + err.Error() + ")"
	default:
		notice = "Not credited: " + err.Error()
	}

	rows, _, listErr := h.deposits.PendingReview(c.Context(), 1)
	if listErr != nil {
		return c.Status(fiber.StatusInternalServerError).SendString("failed to reload deposits")
	}
	c.Set("Content-Type", "text/html; charset=utf-8")
	return c.SendString(renderAdminDepositsTableHTML(rows, notice))
}

// CreditDeposit credits a queued receipt on the admin's own figure, for the
// receipts the verifier cannot read at all. Unlike ApproveDeposit this does
// take the admin at their word about the amount, which is why the row records
// who did it.
func (h *AdminHandler) CreditDeposit(c fiber.Ctx) error {
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).SendString("bad deposit id")
	}

	notice := ""
	amount, convErr := strconv.ParseInt(strings.TrimSpace(c.FormValue("amount")), 10, 64)
	switch {
	case convErr != nil:
		notice = "Not credited: " + services.ErrManualAmountInvalid.Error()
	default:
		admin := ""
		if user, ok := c.Locals("admin_user").(*repository.User); ok {
			admin = user.Username
		}
		outcome, err := h.deposits.CreditManually(c.Context(), id, amount, c.FormValue("reference"), admin)
		if err != nil {
			notice = "Not credited: " + err.Error()
		} else {
			notice = fmt.Sprintf("Credited %d ETB by hand — no bank check was performed.", outcome.Amount)
		}
	}

	rows, _, listErr := h.deposits.PendingReview(c.Context(), 1)
	if listErr != nil {
		return c.Status(fiber.StatusInternalServerError).SendString("failed to reload deposits")
	}
	c.Set("Content-Type", "text/html; charset=utf-8")
	return c.SendString(renderAdminDepositsTableHTML(rows, notice))
}

func (h *AdminHandler) RejectDeposit(c fiber.Ctx) error {
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).SendString("bad deposit id")
	}

	notice := "Deposit rejected."
	if err := h.deposits.RejectQueued(c.Context(), id, c.FormValue("note")); err != nil {
		notice = "Could not reject: " + err.Error()
	}

	rows, _, listErr := h.deposits.PendingReview(c.Context(), 1)
	if listErr != nil {
		return c.Status(fiber.StatusInternalServerError).SendString("failed to reload deposits")
	}
	c.Set("Content-Type", "text/html; charset=utf-8")
	return c.SendString(renderAdminDepositsTableHTML(rows, notice))
}
