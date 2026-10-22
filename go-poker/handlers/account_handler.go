package handlers

import (
	"errors"
	"strconv"

	"github.com/gofiber/fiber/v3"
	"github.com/zuse/poker5/go-poker/repository"
	"github.com/zuse/poker5/go-poker/services"
)

// AccountHandler serves the signed-in player's own pages: their hand history,
// their statistics, and changes to their profile.
type AccountHandler struct {
	auth    *services.AuthService
	history *services.HistoryService
	profile *services.ProfileService
}

func NewAccountHandler(auth *services.AuthService, history *services.HistoryService, profile *services.ProfileService) *AccountHandler {
	return &AccountHandler{auth: auth, history: history, profile: profile}
}

func (h *AccountHandler) user(c fiber.Ctx) (*repository.User, string, error) {
	token := c.Cookies("poker_session")
	if token == "" {
		token = c.Get("Authorization")
	}
	user, err := h.auth.GetUserByToken(c.Context(), token)
	return user, token, err
}

func unauthorized(c fiber.Ctx) error {
	return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "unauthorized"})
}

// HistoryRooms lists every room the player has been dealt into.
func (h *AccountHandler) HistoryRooms(c fiber.Ctx) error {
	user, _, err := h.user(c)
	if err != nil {
		return unauthorized(c)
	}
	rooms, err := h.history.Rooms(c.Context(), user.ID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "could not load history"})
	}
	return c.JSON(fiber.Map{"rooms": rooms})
}

// HistoryRoomHands lists the player's own hands in one room, newest first.
func (h *AccountHandler) HistoryRoomHands(c fiber.Ctx) error {
	user, _, err := h.user(c)
	if err != nil {
		return unauthorized(c)
	}
	page, _ := strconv.Atoi(c.Query("page", "1"))
	pageSize, _ := strconv.Atoi(c.Query("page_size", "20"))
	result, err := h.history.RoomHands(c.Context(), user.ID, c.Params("code"), page, pageSize)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "could not load hands"})
	}
	return c.JSON(result)
}

func (h *AccountHandler) Stats(c fiber.Ctx) error {
	user, _, err := h.user(c)
	if err != nil {
		return unauthorized(c)
	}
	stats, err := h.history.Stats(c.Context(), user.ID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "could not load statistics"})
	}
	return c.JSON(stats)
}

func (h *AccountHandler) SetAvatar(c fiber.Ctx) error {
	user, _, err := h.user(c)
	if err != nil {
		return unauthorized(c)
	}
	var body struct {
		Avatar *int `json:"avatar"`
	}
	if err := c.Bind().JSON(&body); err != nil || body.Avatar == nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "avatar is required"})
	}
	if err := h.profile.SetAvatar(c.Context(), user, *body.Avatar); err != nil {
		return profileError(c, err)
	}
	return c.JSON(fiber.Map{"message": "avatar updated", "avatar": *body.Avatar})
}

func (h *AccountHandler) SetUsername(c fiber.Ctx) error {
	user, _, err := h.user(c)
	if err != nil {
		return unauthorized(c)
	}
	var body struct {
		Username string `json:"username"`
	}
	_ = c.Bind().JSON(&body)
	if err := h.profile.SetUsername(c.Context(), user, body.Username); err != nil {
		return profileError(c, err)
	}
	return c.JSON(fiber.Map{"message": "username updated"})
}

func (h *AccountHandler) SetPassword(c fiber.Ctx) error {
	user, token, err := h.user(c)
	if err != nil {
		return unauthorized(c)
	}
	var body struct {
		CurrentPassword string `json:"current_password"`
		NewPassword     string `json:"new_password"`
	}
	_ = c.Bind().JSON(&body)
	if err := h.profile.SetPassword(c.Context(), user, token, body.CurrentPassword, body.NewPassword); err != nil {
		return profileError(c, err)
	}
	return c.JSON(fiber.Map{"message": "password updated"})
}

func profileError(c fiber.Ctx, err error) error {
	var input *services.InputError
	switch {
	case errors.Is(err, services.ErrUsernameTaken), errors.Is(err, services.ErrSeatedRename):
		return c.Status(fiber.StatusConflict).JSON(fiber.Map{"error": err.Error()})
	case errors.Is(err, services.ErrWrongPassword):
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": err.Error()})
	case errors.Is(err, services.ErrInvalidAvatar),
		errors.Is(err, services.ErrInvalidUsername),
		errors.Is(err, services.ErrSamePassword),
		errors.As(err, &input):
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	default:
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "could not update your profile"})
	}
}
