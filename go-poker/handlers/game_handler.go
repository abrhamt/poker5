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
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "unauthorized"})
	}

	if user.Role == "admin" {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "admin accounts cannot play"})
	}

	room, err := h.rooms.GetRoomByCode(c.Context(), roomCode)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "room not found"})
	}

	err = h.game.JoinTable(c.Context(), room, user)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
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
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	return c.SendStatus(fiber.StatusOK)
}

// SitIn returns a player who was sat out by the turn clock to the next hand.
// Nothing acts on their behalf again until they ask for it here.
func (h *GameHandler) SitIn(c fiber.Ctx) error {
	roomCode := c.Params("id")
	token := c.Cookies("poker_session")
	if token == "" {
		token = c.Get("Authorization")
	}

	user, err := h.auth.GetUserByToken(c.Context(), token)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "unauthorized"})
	}

	if err := h.game.SitIn(c.Context(), roomCode, user.ID); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{"status": "ok"})
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
