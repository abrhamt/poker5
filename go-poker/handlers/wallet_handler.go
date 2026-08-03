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
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "unauthorized"})
	}

	var body struct {
		Amount float64 `json:"amount"`
	}
	if err := c.Bind().JSON(&body); err != nil || body.Amount <= 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid deposit amount"})
	}

	tx, err := h.wallet.Deposit(c.Context(), user.ID, body.Amount)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
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
