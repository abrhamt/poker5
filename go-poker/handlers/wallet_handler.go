package handlers

import (
	"strconv"

	"github.com/gofiber/fiber/v3"
	"github.com/zuse/poker5/go-poker/services"
)

type WalletHandler struct {
	wallet *services.WalletService
	auth   *services.AuthService
}

func NewWalletHandler(wallet *services.WalletService, auth *services.AuthService) *WalletHandler {
	return &WalletHandler{wallet: wallet, auth: auth}
}

func (h *WalletHandler) Deposit(c fiber.Ctx) error {
	token := c.Cookies("poker_session")
	if token == "" {
		token = c.Get("Authorization")
	}

	user, err := h.auth.GetUserByToken(c.Context(), token)
	if err != nil {
		if c.Get("HX-Request") == "true" {
			c.Set("HX-Redirect", "/login")
			return c.SendStatus(fiber.StatusUnauthorized)
		}
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "unauthorized"})
	}

	var body struct {
		Amount int64 `json:"amount" form:"amount"`
	}
	_ = c.Bind().Body(&body)
	if body.Amount <= 0 {
		if amtStr := c.FormValue("amount"); amtStr != "" {
			body.Amount, _ = strconv.ParseInt(amtStr, 10, 64)
		}
	}
	if body.Amount <= 0 {
		_ = c.Bind().JSON(&body)
	}

	if body.Amount <= 0 {
		if c.Get("HX-Request") == "true" {
			return c.SendString(`<div class="toast error">Please enter a valid deposit amount greater than $0.</div>`)
		}
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid deposit amount"})
	}

	tx, err := h.wallet.Deposit(c.Context(), user.ID, body.Amount)
	if err != nil {
		if c.Get("HX-Request") == "true" {
			return c.SendString(`<div class="toast error">` + err.Error() + `</div>`)
		}
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	if c.Get("HX-Request") == "true" {
		c.Set("HX-Redirect", "/wallet")
		return c.SendStatus(fiber.StatusOK)
	}

	return c.JSON(fiber.Map{
		"message":        "deposit successful",
		"transaction_id": tx.TransactionID,
		"amount":         tx.Amount,
		"reason":         tx.Reason,
	})
}

func (h *WalletHandler) GetTransactions(c fiber.Ctx) error {
	token := c.Cookies("poker_session")
	if token == "" {
		token = c.Get("Authorization")
	}

	user, err := h.auth.GetUserByToken(c.Context(), token)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "unauthorized"})
	}

	txs, err := h.wallet.GetUserTransactions(c.Context(), user.ID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	out := make([]fiber.Map, 0, len(txs))
	for _, t := range txs {
		out = append(out, fiber.Map{
			"id":             t.ID,
			"amount":         t.Amount,
			"type":           t.Type,
			"reason":         t.Reason,
			"transaction_id": t.TransactionID,
			"created_at":     t.CreatedAt,
		})
	}

	return c.JSON(fiber.Map{
		"user_id":      strconv.FormatInt(user.ID, 10),
		"transactions": out,
	})
}
