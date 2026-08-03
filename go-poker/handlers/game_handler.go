package handlers

import (
	"github.com/gofiber/fiber/v3"
	"github.com/zuse/poker5/go-poker/services"
)

type GameHandler struct {
	sse *services.SSEHub
}

func NewGameHandler(sse *services.SSEHub) *GameHandler {
	return &GameHandler{sse: sse}
}

func (h *GameHandler) GetState(c fiber.Ctx) error {
	tableID := c.Query("table_id")
	if tableID == "" {
		tableID = "demo"
	}

	return c.JSON(fiber.Map{
		"table_id": tableID,
		"status":   "active",
		"message":  "game engine operational",
	})
}

func (h *GameHandler) Action(c fiber.Ctx) error {
	var req struct {
		TableID string `json:"table_id"`
		Action  string `json:"action"`
		Amount  int    `json:"amount"`
	}

	if err := c.Bind().JSON(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid action request"})
	}

	if req.TableID == "" {
		req.TableID = "demo"
	}

	h.sse.Broadcast(req.TableID, "action", fiber.Map{
		"action": req.Action,
		"amount": req.Amount,
	})

	return c.JSON(fiber.Map{
		"status":  "ok",
		"action":  req.Action,
		"amount":  req.Amount,
		"table_id": req.TableID,
	})
}
