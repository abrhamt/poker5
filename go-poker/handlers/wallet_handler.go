package handlers

import (
	"errors"
	"strconv"
	"strings"

	"github.com/gofiber/fiber/v3"
	"github.com/zuse/poker5/go-poker/repository"
	"github.com/zuse/poker5/go-poker/services"
	"github.com/zuse/poker5/go-poker/utilities"
)

type WalletHandler struct {
	wallet  *services.WalletService
	auth    *services.AuthService
	deposit *services.DepositService
}

func NewWalletHandler(wallet *services.WalletService, auth *services.AuthService, deposit *services.DepositService) *WalletHandler {
	return &WalletHandler{wallet: wallet, auth: auth, deposit: deposit}
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

	// With real deposits on, this route is the play-money path and has to be
	// closed: a client that keeps calling it — an old bundle, or a crafted
	// request — would otherwise mint balance for free.
	cfg, err := h.deposit.Config(c.Context())
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "could not read deposit settings"})
	}
	if cfg.RealDepositsEnabled {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": services.ErrRealDepositsRequired.Error()})
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

// DepositInfo tells the client which deposit form to render and, when real
// deposits are on, the account to send money to. It is the client's only
// source for that account number — hardcoding it in the bundle would mean a
// stale build sends players' money to an account we no longer hold.
func (h *WalletHandler) DepositInfo(c fiber.Ctx) error {
	if _, err := h.authenticate(c); err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "unauthorized"})
	}

	cfg, err := h.deposit.Config(c.Context())
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "could not read deposit settings"})
	}
	if !cfg.RealDepositsEnabled {
		// The account is withheld while the toggle is off so a half-configured
		// one is never shown as somewhere to send money.
		return c.JSON(fiber.Map{"real_deposits_enabled": false})
	}
	return c.JSON(fiber.Map{
		"real_deposits_enabled": true,
		"account_name":          cfg.AccountName,
		"account_number":        cfg.AccountNumber,
		"min_amount":            services.MinReceiptDeposit,
	})
}

// SubmitReceipt takes the pasted CBE SMS. The client extracts the link before
// posting, but the whole text is accepted and re-parsed here: the client's
// extraction is there to catch a bad paste early, not to be trusted.
func (h *WalletHandler) SubmitReceipt(c fiber.Ctx) error {
	user, err := h.authenticate(c)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "unauthorized"})
	}

	var body struct {
		Message string `json:"message" form:"message"`
		URL     string `json:"url" form:"url"`
	}
	_ = c.Bind().Body(&body)

	pasted := body.Message
	if strings.TrimSpace(pasted) == "" {
		pasted = body.URL
	}
	if strings.TrimSpace(pasted) == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": utilities.ErrNoReceiptURL.Error()})
	}

	outcome, err := h.deposit.SubmitReceipt(c.Context(), user.ID, pasted)
	if err != nil {
		return c.Status(depositErrorStatus(err)).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(outcome)
}

// depositErrorStatus maps a deposit refusal onto the status code that says why
// it happened: the client shows the message either way, but a 409 on an
// already-claimed receipt and a 502 on a dead verifier are what make the logs
// and any future retry logic legible.
func depositErrorStatus(err error) int {
	switch {
	case errors.Is(err, services.ErrReceiptAlreadyClaimed), errors.Is(err, services.ErrReceiptQueued):
		return fiber.StatusConflict
	case errors.Is(err, services.ErrRealDepositsDisabled), errors.Is(err, services.ErrDepositAccountUnset):
		return fiber.StatusForbidden
	case errors.Is(err, services.ErrVerifierUnavailable):
		return fiber.StatusBadGateway
	case errors.Is(err, services.ErrReceiptRejected),
		errors.Is(err, services.ErrReceiptWrongAccount),
		errors.Is(err, services.ErrReceiptNotCompleted),
		errors.Is(err, services.ErrReceiptTooSmall),
		errors.Is(err, services.ErrReceiptWrongCurrency),
		errors.Is(err, utilities.ErrNoReceiptURL),
		errors.Is(err, utilities.ErrReceiptURLTooLong):
		return fiber.StatusBadRequest
	default:
		return fiber.StatusInternalServerError
	}
}

func (h *WalletHandler) authenticate(c fiber.Ctx) (*repository.User, error) {
	token := c.Cookies("poker_session")
	if token == "" {
		token = c.Get("Authorization")
	}
	return h.auth.GetUserByToken(c.Context(), token)
}
