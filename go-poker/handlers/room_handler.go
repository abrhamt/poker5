package handlers

import (
	"strconv"

	"github.com/gofiber/fiber/v3"
	"github.com/zuse/poker5/go-poker/services"
)

type RoomHandler struct {
	rooms *services.RoomService
	auth  *services.AuthService
}

func NewRoomHandler(rooms *services.RoomService, auth *services.AuthService) *RoomHandler {
	return &RoomHandler{rooms: rooms, auth: auth}
}

func (h *RoomHandler) CreatePrivate(c fiber.Ctx) error {
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
			return c.SendString(`<div class="error-badge">Admin accounts can't play.</div>`)
		}
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "admin accounts cannot play"})
	}

	var req struct {
		RoomName   string `json:"room_name" form:"room_name"`
		SmallBlind int32  `json:"small_blind" form:"small_blind"`
		MaxPlayers int32  `json:"max_players" form:"max_players"`
	}
	_ = c.Bind().Body(&req)
	if req.RoomName == "" {
		req.RoomName = c.FormValue("room_name")
	}
	if req.SmallBlind <= 0 {
		if val := c.FormValue("small_blind"); val != "" {
			n, _ := strconv.Atoi(val)
			req.SmallBlind = int32(n)
		}
	}
	if req.MaxPlayers <= 0 {
		if val := c.FormValue("max_players"); val != "" {
			n, _ := strconv.Atoi(val)
			req.MaxPlayers = int32(n)
		}
	}

	room, err := h.rooms.CreatePrivateRoom(c.Context(), user.ID, req.RoomName, req.SmallBlind, req.MaxPlayers)
	if err != nil {
		if c.Get("HX-Request") == "true" {
			return c.SendString(`<div class="error-badge">` + err.Error() + `</div>`)
		}
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	if c.Get("HX-Request") == "true" {
		c.Set("HX-Redirect", "/table/"+room.RoomCode)
		return c.SendStatus(fiber.StatusOK)
	}

	return c.JSON(fiber.Map{
		"message":   "private room created",
		"room_code": room.RoomCode,
		"room_name": room.RoomName,
		"buy_in":    room.BuyIn,
	})
}

func (h *RoomHandler) QuickJoin(c fiber.Ctx) error {
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
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "admin accounts cannot play"})
	}

	var smallBlind int32
	if val := c.FormValue("small_blind"); val != "" {
		n, _ := strconv.Atoi(val)
		smallBlind = int32(n)
	}
	if smallBlind == 0 {
		var req struct {
			SmallBlind int32 `json:"small_blind"`
		}
		_ = c.Bind().Body(&req)
		smallBlind = req.SmallBlind
	}

	room, err := h.rooms.QuickJoinPublicRoom(c.Context(), smallBlind)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	if c.Get("HX-Request") == "true" {
		c.Set("HX-Redirect", "/table/"+room.RoomCode)
		return c.SendStatus(fiber.StatusOK)
	}

	return c.JSON(fiber.Map{
		"room_code": room.RoomCode,
		"room_name": room.RoomName,
		"buy_in":    room.BuyIn,
	})
}

func (h *RoomHandler) JoinByCode(c fiber.Ctx) error {
	var req struct {
		Code string `json:"code" form:"code"`
	}
	_ = c.Bind().Body(&req)
	if req.Code == "" {
		req.Code = c.FormValue("code")
	}
	if req.Code == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "room code is required"})
	}

	room, err := h.rooms.GetRoomByCode(c.Context(), req.Code)
	if err != nil {
		if c.Get("HX-Request") == "true" {
			return c.SendString(`<div class="error-badge">Room not found. Check the 5-digit code and try again.</div>`)
		}
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "room not found"})
	}

	if c.Get("HX-Request") == "true" {
		c.Set("HX-Redirect", "/table/"+room.RoomCode)
		return c.SendStatus(fiber.StatusOK)
	}

	return c.JSON(fiber.Map{
		"room_code": room.RoomCode,
		"room_name": room.RoomName,
		"buy_in":    room.BuyIn,
	})
}

func (h *RoomHandler) ListPublic(c fiber.Ctx) error {
	rooms, err := h.rooms.ListPublicRooms(c.Context())
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	out := make([]fiber.Map, 0, len(rooms))
	for _, r := range rooms {
		out = append(out, fiber.Map{
			"room_code":   r.RoomCode,
			"room_name":   r.RoomName,
			"buy_in":      r.BuyIn,
			"small_blind": r.SmallBlind,
			"big_blind":   r.BigBlind,
			"max_players": r.MaxPlayers,
		})
	}

	return c.JSON(fiber.Map{"rooms": out, "tiers": services.PublicBlindTiers})
}
