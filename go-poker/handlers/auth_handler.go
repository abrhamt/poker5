package handlers

import (
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/zuse/poker5/go-poker/services"
)

type AuthHandler struct {
	auth *services.AuthService
	// secureCookies mirrors SESSION_COOKIE_SECURE; see session_cookie.go.
	secureCookies bool
}

func NewAuthHandler(auth *services.AuthService, secureCookies bool) *AuthHandler {
	return &AuthHandler{auth: auth, secureCookies: secureCookies}
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
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	_, token, err := h.auth.Login(c.Context(), services.LoginRequest{
		Login:    req.Username,
		Password: req.Password,
	})
	if err == nil {
		c.Cookie(sessionCookie(token, time.Now().Add(sessionCookieTTL), h.secureCookies))
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
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": err.Error()})
	}

	c.Cookie(sessionCookie(token, time.Now().Add(sessionCookieTTL), h.secureCookies))

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
			"role":          user.Role,
			"created_at":    user.CreatedAt,
		},
	})
}

func (h *AuthHandler) Logout(c fiber.Ctx) error {
	token := c.Cookies("poker_session")
	if token != "" {
		_ = h.auth.Logout(c.Context(), token)
	}

	c.Cookie(sessionCookie("", time.Now().Add(-1*time.Hour), h.secureCookies))

	return c.JSON(fiber.Map{"message": "logged out successfully"})
}
