package handlers

import (
	"strconv"

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
}

func NewAdminHandler(admin *services.AdminService, settings *services.SettingsService, auth *services.AuthService) *AdminHandler {
	return &AdminHandler{admin: admin, settings: settings, auth: auth}
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

	html := renderAdminDashboardHTML(earnings, siteSettings, users, filterTxRows(txs))
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
	}
	_ = c.Bind().Body(&req)

	err := h.settings.Update(c.Context(), services.UpdateSettingsRequest{
		RakeMode:                  req.RakeMode,
		RakePercentage:            req.RakePercentage,
		ReferralPercentagePctMode: req.ReferralPercentagePctMode,
		ReferralPercentageSbMode:  req.ReferralPercentageSbMode,
		CountdownSeconds:          int32(req.CountdownSeconds),
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
