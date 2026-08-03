package handlers

import (
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/zuse/poker5/go-poker/services"
)

type AuthHandler struct {
	auth *services.AuthService
}

func NewAuthHandler(auth *services.AuthService) *AuthHandler {
	return &AuthHandler{auth: auth}
}

func (h *AuthHandler) Register(c fiber.Ctx) error {
	var req services.RegisterRequest
	if err := c.Bind().JSON(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request body"})
	}

	user, err := h.auth.Register(c.Context(), req)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	return c.Status(fiber.StatusCreated).JSON(fiber.Map{
		"message": "user registered successfully",
		"user": fiber.Map{
			"id":            user.ID,
			"username":      user.Username,
			"phone_number":  user.PhoneNumber,
			"wallet":        user.Wallet,
			"referral_code": user.ReferralCode,
		},
	})
}

func (h *AuthHandler) Login(c fiber.Ctx) error {
	var req services.LoginRequest
	if err := c.Bind().JSON(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request body"})
	}

	user, token, err := h.auth.Login(c.Context(), req)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": err.Error()})
	}

	c.Cookie(&fiber.Cookie{
		Name:     "poker_session",
		Value:    token,
		Expires:  time.Now().Add(24 * 7 * time.Hour),
		HTTPOnly: true,
		SameSite: "Lax",
	})

	return c.JSON(fiber.Map{
		"message": "login successful",
		"token":   token,
		"user": fiber.Map{
			"id":            user.ID,
			"username":      user.Username,
			"phone_number":  user.PhoneNumber,
			"wallet":        user.Wallet,
			"referral_code": user.ReferralCode,
		},
	})
}

func (h *AuthHandler) Me(c fiber.Ctx) error {
	token := c.Cookies("poker_session")
	if token == "" {
		token = c.Get("Authorization")
	}

	user, err := h.auth.GetUserByToken(c.Context(), token)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "unauthorized"})
	}

	return c.JSON(fiber.Map{
		"user": fiber.Map{
			"id":            user.ID,
			"username":      user.Username,
			"phone_number":  user.PhoneNumber,
			"wallet":        user.Wallet,
			"referral_code": user.ReferralCode,
			"created_at":    user.CreatedAt,
		},
	})
}

func (h *AuthHandler) Logout(c fiber.Ctx) error {
	token := c.Cookies("poker_session")
	if token != "" {
		_ = h.auth.Logout(c.Context(), token)
	}

	c.Cookie(&fiber.Cookie{
		Name:     "poker_session",
		Value:    "",
		Expires:  time.Now().Add(-1 * time.Hour),
		HTTPOnly: true,
	})

	return c.JSON(fiber.Map{"message": "logged out successfully"})
}
