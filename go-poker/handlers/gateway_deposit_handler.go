package handlers

import (
	"encoding/json"
	"errors"
	"log"
	"strconv"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/zuse/poker5/go-poker/services"
	"github.com/zuse/poker5/go-poker/utilities"
)

type GatewayDepositHandler struct {
	auth    *services.AuthService
	gateway *services.GatewayDepositService
}

func NewGatewayDepositHandler(auth *services.AuthService, gateway *services.GatewayDepositService) *GatewayDepositHandler {
	return &GatewayDepositHandler{auth: auth, gateway: gateway}
}

func (h *GatewayDepositHandler) Start(c fiber.Ctx) error {
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
		if raw := c.FormValue("amount"); raw != "" {
			body.Amount, _ = strconv.ParseInt(raw, 10, 64)
		}
	}

	checkout, err := h.gateway.Start(c.Context(), *user, body.Amount)
	if err != nil {
		return c.Status(gatewayErrorStatus(err)).JSON(fiber.Map{"error": gatewayErrorMessage(err)})
	}
	return c.JSON(checkout)
}

func (h *GatewayDepositHandler) Status(c fiber.Ctx) error {
	token := c.Cookies("poker_session")
	if token == "" {
		token = c.Get("Authorization")
	}
	user, err := h.auth.GetUserByToken(c.Context(), token)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "unauthorized"})
	}

	status, err := h.gateway.Status(c.Context(), user.ID, c.Params("reference"))
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(status)
}

func (h *GatewayDepositHandler) RouterCallback(c fiber.Ctx) error {
	body := c.Body()
	if !utilities.VerifyRouterSignature(
		h.gateway.WebhookSecret(),
		c.Get(utilities.RouterSignatureHeader),
		c.Get(utilities.RouterTimestampHeader),
		body,
		time.Now(),
	) {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "invalid signature"})
	}

	var payload struct {
		Event string                    `json:"event"`
		Data  *services.RouterEventData `json:"data"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid json"})
	}
	log.Printf("router callback delivery=%s event=%s", c.Get(utilities.RouterDeliveryIDHeader), payload.Event)

	if payload.Event == services.RouterEventTest {
		return c.JSON(fiber.Map{"status": "ok"})
	}
	if payload.Event == "" || payload.Data == nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid payload"})
	}

	if err := h.gateway.ApplyEvent(c.Context(), payload.Event, *payload.Data); err != nil {
		log.Printf("router callback: %s for %s failed: %v", payload.Event, payload.Data.Reference, err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "could not apply event"})
	}
	return c.JSON(fiber.Map{"status": "ok"})
}

func gatewayErrorStatus(err error) int {
	var routerErr *services.PaymentRouterError
	switch {
	case errors.Is(err, services.ErrGatewayAmountInvalid):
		return fiber.StatusBadRequest
	case errors.Is(err, services.ErrGatewayDepositsDisabled):
		return fiber.StatusForbidden
	case errors.Is(err, services.ErrPaymentRouterUnavailable), errors.As(err, &routerErr):
		return fiber.StatusBadGateway
	default:
		return fiber.StatusInternalServerError
	}
}

func gatewayErrorMessage(err error) string {
	var routerErr *services.PaymentRouterError
	switch {
	case errors.Is(err, services.ErrGatewayAmountInvalid), errors.Is(err, services.ErrGatewayDepositsDisabled):
		return err.Error()
	case errors.Is(err, services.ErrPaymentRouterUnavailable), errors.As(err, &routerErr):
		return "The payment service could not start your deposit. Please try again in a moment."
	default:
		return "could not start deposit"
	}
}
