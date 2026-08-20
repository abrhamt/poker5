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

	page, _ := strconv.Atoi(c.Query("page"))
	pageSize, _ := strconv.Atoi(c.Query("page_size"))

	result, err := h.wallet.GetUserTransactions(c.Context(), user.ID, page, pageSize)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	out := make([]fiber.Map, 0, len(result.Transactions))
	for _, t := range result.Transactions {
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
		"transactions": out,
		"page":         result.Page,
		"page_size":    result.PageSize,
		"total":        result.Total,
		"total_pages":  result.TotalPages,
	})
}
