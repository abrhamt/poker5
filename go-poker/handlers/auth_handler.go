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
	_ = c.Bind().Body(&req)
	if req.Username == "" {
		req.Username = c.FormValue("username")
	}
	if req.PhoneNumber == "" {
		req.PhoneNumber = c.FormValue("phone_number")
	}
	if req.Password == "" {
		req.Password = c.FormValue("password")
	}
	if req.ReferralCode == "" {
		req.ReferralCode = c.FormValue("referral_code")
	}
	if req.Username == "" && req.PhoneNumber == "" {
		_ = c.Bind().JSON(&req)
	}

	user, err := h.auth.Register(c.Context(), req)
	if err != nil {
		if c.Get("HX-Request") == "true" {
			return c.SendString(`<div class="error-badge">` + err.Error() + `</div>`)
		}
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	_, token, err := h.auth.Login(c.Context(), services.LoginRequest{
		Login:    req.Username,
		Password: req.Password,
	})
	if err == nil {
		c.Cookie(&fiber.Cookie{
			Name:     "poker_session",
			Value:    token,
			Expires:  time.Now().Add(24 * 7 * time.Hour),
			HTTPOnly: true,
			SameSite: "Lax",
		})
	}

	if c.Get("HX-Request") == "true" {
		c.Set("HX-Redirect", "/lobby")
		return c.SendStatus(fiber.StatusCreated)
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
	_ = c.Bind().Body(&req)
	if req.Login == "" {
		req.Login = c.FormValue("login")
	}
	if req.Password == "" {
		req.Password = c.FormValue("password")
	}
	if req.Login == "" {
		_ = c.Bind().JSON(&req)
	}

	user, token, err := h.auth.Login(c.Context(), req)
	if err != nil {
		if c.Get("HX-Request") == "true" {
			return c.SendString(`<div class="error-badge">` + err.Error() + `</div>`)
		}
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": err.Error()})
	}

	c.Cookie(&fiber.Cookie{
		Name:     "poker_session",
		Value:    token,
		Expires:  time.Now().Add(24 * 7 * time.Hour),
		HTTPOnly: true,
		SameSite: "Lax",
	})

	if c.Get("HX-Request") == "true" {
		c.Set("HX-Redirect", homeRouteFor(user))
		return c.SendStatus(fiber.StatusOK)
	}

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

	if c.Get("HX-Request") == "true" {
		c.Set("HX-Redirect", "/login")
		return c.SendStatus(fiber.StatusOK)
	}

	return c.JSON(fiber.Map{"message": "logged out successfully"})
}
