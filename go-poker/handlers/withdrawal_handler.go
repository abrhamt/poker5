package handlers

import (
	"errors"

	"github.com/gofiber/fiber/v3"
	"github.com/zuse/poker5/go-poker/repository"
	"github.com/zuse/poker5/go-poker/services"
)

// WithdrawalHandler is the player's side of withdrawals: ask to be paid out,
// see how the request is going, and take it back while nobody has paid it.
type WithdrawalHandler struct {
	auth        *services.AuthService
	withdrawals *services.WithdrawalService
}

func NewWithdrawalHandler(auth *services.AuthService, withdrawals *services.WithdrawalService) *WithdrawalHandler {
	return &WithdrawalHandler{auth: auth, withdrawals: withdrawals}
}

func (h *WithdrawalHandler) authenticate(c fiber.Ctx) (*repository.User, error) {
	token := c.Cookies("poker_session")
	if token == "" {
		token = c.Get("Authorization")
	}
	return h.auth.GetUserByToken(c.Context(), token)
}

// playerWithdrawalJSON leaves out what only the admin needs — who resolved it
// and the internal ledger ids.
func playerWithdrawalJSON(w repository.Withdrawal) fiber.Map {
	return fiber.Map{
		"id":               w.ID,
		"method":           w.Method,
		"account_number":   w.AccountNumber,
		"account_name":     w.AccountName,
		"amount":           w.Amount,
		"status":           w.Status,
		"note":             w.Note,
		"payout_reference": w.PayoutReference,
		"created_at":       w.CreatedAt,
		"resolved_at":      unixTime(w.ResolvedAt),
	}
}

// Info is everything the withdrawal sheet needs to render: whether it is open,
// the rules in force, and the player's recent requests.
func (h *WithdrawalHandler) Info(c fiber.Ctx) error {
	user, err := h.authenticate(c)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "unauthorized"})
	}

	cfg, err := h.withdrawals.Config(c.Context(), user.ID)
	if err != nil {
		return jsonError(c, fiber.StatusInternalServerError, errors.New("could not read withdrawal settings"))
	}
	rows, err := h.withdrawals.ListForPlayer(c.Context(), user.ID)
	if err != nil {
		return jsonError(c, fiber.StatusInternalServerError, errors.New("could not load withdrawals"))
	}
	history := make([]fiber.Map, 0, len(rows))
	for _, w := range rows {
		history = append(history, playerWithdrawalJSON(w))
	}

	return c.JSON(fiber.Map{
		"enabled":        cfg.Enabled,
		"min_amount":     cfg.MinAmount,
		"cancel_enabled": cfg.CancelEnabled,
		"methods":        cfg.Methods,
		"cooldown_until": cfg.CooldownUntil,
		"withdrawals":    history,
	})
}

func (h *WithdrawalHandler) Request(c fiber.Ctx) error {
	user, err := h.authenticate(c)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "unauthorized"})
	}

	var body struct {
		Method        string `json:"method"`
		AccountNumber string `json:"account_number"`
		AccountName   string `json:"account_name"`
		Amount        int64  `json:"amount"`
	}
	if err := c.Bind().JSON(&body); err != nil {
		return jsonError(c, fiber.StatusBadRequest, errors.New("invalid withdrawal request"))
	}

	row, err := h.withdrawals.Request(c.Context(), user.ID, services.WithdrawalRequest{
		Method:        body.Method,
		AccountNumber: body.AccountNumber,
		AccountName:   body.AccountName,
		Amount:        body.Amount,
	})
	if err != nil {
		return jsonError(c, withdrawalErrorStatus(err), err)
	}
	return c.JSON(fiber.Map{"withdrawal": playerWithdrawalJSON(*row)})
}

func (h *WithdrawalHandler) Cancel(c fiber.Ctx) error {
	user, err := h.authenticate(c)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "unauthorized"})
	}
	id, ok := pathID(c)
	if !ok {
		return jsonError(c, fiber.StatusBadRequest, errors.New("bad withdrawal id"))
	}

	row, err := h.withdrawals.Cancel(c.Context(), user.ID, id)
	if err != nil {
		return jsonError(c, withdrawalErrorStatus(err), err)
	}
	return c.JSON(fiber.Map{"withdrawal": playerWithdrawalJSON(*row)})
}
