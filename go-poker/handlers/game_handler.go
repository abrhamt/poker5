package handlers

import (
	"strconv"

	"github.com/gofiber/fiber/v3"
	"github.com/zuse/poker5/go-poker/services"
)

type GameHandler struct {
	game  *services.GameService
	auth  *services.AuthService
	rooms *services.RoomService
}

func NewGameHandler(game *services.GameService, auth *services.AuthService, rooms *services.RoomService) *GameHandler {
	return &GameHandler{game: game, auth: auth, rooms: rooms}
}

func (h *GameHandler) Join(c fiber.Ctx) error {
	roomCode := c.Params("id")
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

	if user.Role == "admin" {
		if c.Get("HX-Request") == "true" {
			return c.SendString(`<div class="toast error">Admin accounts can't play — head to the dashboard instead.</div>`)
		}
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "admin accounts cannot play"})
	}

	room, err := h.rooms.GetRoomByCode(c.Context(), roomCode)
	if err != nil {
		if c.Get("HX-Request") == "true" {
			return c.SendString(`<div class="toast error">Room not found.</div>`)
		}
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "room not found"})
	}

	err = h.game.JoinTable(c.Context(), room, user)
	if err != nil {
		if c.Get("HX-Request") == "true" {
			return c.SendString(`<div class="toast error">` + err.Error() + `</div>`)
		}
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	if c.Get("HX-Request") == "true" {
		return c.SendString(`<div class="toast success">Joined table successfully!</div>`)
	}
	return c.JSON(fiber.Map{"status": "ok", "message": "joined table"})
}

func (h *GameHandler) Leave(c fiber.Ctx) error {
	roomCode := c.Params("id")
	token := c.Cookies("poker_session")
	if token == "" {
		token = c.Get("Authorization")
	}

	user, err := h.auth.GetUserByToken(c.Context(), token)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "unauthorized"})
	}

	err = h.game.LeaveTable(c.Context(), roomCode, user.ID)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	if c.Get("HX-Request") == "true" {
		c.Set("HX-Redirect", "/lobby")
		return c.SendStatus(fiber.StatusOK)
	}
	return c.JSON(fiber.Map{"status": "ok", "message": "left table"})
}

func (h *GameHandler) Act(c fiber.Ctx) error {
	roomCode := c.Params("id")
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

	var req struct {
		Action string `json:"action" form:"action"`
		Amount int    `json:"amount" form:"amount"`
	}
	_ = c.Bind().Body(&req)
	if req.Action == "" {
		req.Action = c.FormValue("action")
	}
	if req.Amount == 0 {
		if amtStr := c.FormValue("amount"); amtStr != "" {
			req.Amount, _ = strconv.Atoi(amtStr)
		}
	}
	if req.Action == "" {
		_ = c.Bind().JSON(&req)
	}

	err = h.game.SubmitAction(c.Context(), roomCode, user, req.Action, req.Amount)
	if err != nil {
		if c.Get("HX-Request") == "true" {
			return c.SendString(`<div class="action-feedback error">` + err.Error() + `</div>`)
		}
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	return c.SendStatus(fiber.StatusOK)
}

func (h *GameHandler) Start(c fiber.Ctx) error {
	roomCode := c.Params("id")
	token := c.Cookies("poker_session")
	if token == "" {
		token = c.Get("Authorization")
	}

	user, err := h.auth.GetUserByToken(c.Context(), token)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "unauthorized"})
	}

	err = h.game.StartHandManually(c.Context(), roomCode, user.ID)
	if err != nil {
		if c.Get("HX-Request") == "true" {
			return c.SendString(`<div class="toast error">` + err.Error() + `</div>`)
		}
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	return c.SendStatus(fiber.StatusOK)
}

func (h *GameHandler) GetState(c fiber.Ctx) error {
	roomCode := c.Params("id")
	token := c.Cookies("poker_session")
	if token == "" {
		token = c.Get("Authorization")
	}

	username := ""
	user, err := h.auth.GetUserByToken(c.Context(), token)
	if err == nil {
		username = user.Username
	}

	state := h.game.GetTableState(c.Context(), roomCode, username)
	return c.JSON(state)
}
