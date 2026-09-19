package handlers

import (
	"errors"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/zuse/poker5/go-poker/repository"
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

// Register starts a signup. No users row is created here: the account exists
// only once VerifyOTP confirms the phone, so an abandoned signup expires
// instead of squatting a username.
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

	phone, err := h.auth.StartRegistration(c.Context(), req)
	if err != nil {
		return authError(c, err)
	}

	return c.Status(fiber.StatusAccepted).JSON(fiber.Map{
		"message":      "verification code sent",
		"phone_number": phone,
	})
}

type verifyOTPRequest struct {
	PhoneNumber string `json:"phone_number"`
	Code        string `json:"code"`
}

// VerifyOTP completes a signup and signs the new player in. It reports success
// even when the session write fails — the account exists at that point, and the
// client can fall back to the login screen.
func (h *AuthHandler) VerifyOTP(c fiber.Ctx) error {
	var req verifyOTPRequest
	_ = c.Bind().Body(&req)

	user, token, err := h.auth.VerifyRegistration(c.Context(), req.PhoneNumber, req.Code)
	if err != nil {
		return authError(c, err)
	}
	if token != "" {
		c.Cookie(sessionCookie(token, time.Now().Add(sessionCookieTTL), h.secureCookies))
	}

	return c.Status(fiber.StatusCreated).JSON(fiber.Map{
		"message": "user registered successfully",
		"user":    userPayload(user),
	})
}

type resendOTPRequest struct {
	PhoneNumber string `json:"phone_number"`
	Purpose     string `json:"purpose"`
}

func (h *AuthHandler) ResendOTP(c fiber.Ctx) error {
	var req resendOTPRequest
	_ = c.Bind().Body(&req)

	purpose := services.PurposeRegister
	if req.Purpose == string(services.PurposeReset) {
		purpose = services.PurposeReset
	}

	if err := h.auth.ResendOTP(c.Context(), req.PhoneNumber, purpose); err != nil {
		return authError(c, err)
	}
	return c.JSON(fiber.Map{"message": "verification code sent"})
}

type phoneRequest struct {
	PhoneNumber string `json:"phone_number"`
}

func (h *AuthHandler) ForgotPassword(c fiber.Ctx) error {
	var req phoneRequest
	_ = c.Bind().Body(&req)

	phone, err := h.auth.StartPasswordReset(c.Context(), req.PhoneNumber)
	if err != nil {
		return authError(c, err)
	}
	return c.JSON(fiber.Map{
		"message":      "reset code sent",
		"phone_number": phone,
	})
}

// VerifyResetOTP spends the code and hands back the token that authorizes the
// password change. The token carries its own, longer clock so that choosing a
// password can't expire the code out from under someone.
func (h *AuthHandler) VerifyResetOTP(c fiber.Ctx) error {
	var req verifyOTPRequest
	_ = c.Bind().Body(&req)

	token, err := h.auth.VerifyResetOTP(c.Context(), req.PhoneNumber, req.Code)
	if err != nil {
		return authError(c, err)
	}
	return c.JSON(fiber.Map{"reset_token": token})
}

type resetPasswordRequest struct {
	ResetToken string `json:"reset_token"`
	Password   string `json:"password"`
}

// ResetPassword changes the password and signs the caller in on this device.
// Every other session is destroyed first, so an attacker holding a stolen
// session does not survive the victim's remediation.
func (h *AuthHandler) ResetPassword(c fiber.Ctx) error {
	var req resetPasswordRequest
	_ = c.Bind().Body(&req)

	user, token, err := h.auth.ResetPassword(c.Context(), req.ResetToken, req.Password)
	if err != nil {
		return authError(c, err)
	}
	if token != "" {
		c.Cookie(sessionCookie(token, time.Now().Add(sessionCookieTTL), h.secureCookies))
	}

	return c.JSON(fiber.Map{
		"message": "password updated",
		"user":    userPayload(user),
	})
}

// authError maps a service error to a status. The distinctions that matter to
// the client: 429 means "wait, then retry the same thing", 502 means "nothing
// was sent, retry now", 404 means "there is nothing in flight to verify".
func authError(c fiber.Ctx, err error) error {
	var throttled *services.ThrottleError

	switch {
	case errors.As(err, &throttled):
		return c.Status(fiber.StatusTooManyRequests).JSON(fiber.Map{"error": err.Error()})
	case errors.Is(err, services.ErrSendFailed):
		return c.Status(fiber.StatusBadGateway).JSON(fiber.Map{"error": err.Error()})
	case errors.Is(err, services.ErrUserExists):
		return c.Status(fiber.StatusConflict).JSON(fiber.Map{"error": err.Error()})
	case errors.Is(err, services.ErrUserNotFound),
		errors.Is(err, services.ErrNoPendingRegistration),
		errors.Is(err, services.ErrNoPendingReset):
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": err.Error()})
	default:
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
}

func userPayload(user *repository.User) fiber.Map {
	return fiber.Map{
		"id":            user.ID,
		"username":      user.Username,
		"phone_number":  user.PhoneNumber,
		"wallet":        user.Wallet,
		"referral_code": user.ReferralCode,
	}
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
