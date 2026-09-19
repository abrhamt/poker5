package handlers

import (
	"strconv"

	"github.com/gofiber/fiber/v3"
	"github.com/zuse/poker5/go-poker/services"
)

type RoomHandler struct {
	rooms *services.RoomService
	auth  *services.AuthService
	game  *services.GameService
}

func NewRoomHandler(rooms *services.RoomService, auth *services.AuthService, game *services.GameService) *RoomHandler {
	return &RoomHandler{rooms: rooms, auth: auth, game: game}
}

func (h *RoomHandler) CreatePrivate(c fiber.Ctx) error {
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
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
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
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "room not found"})
	}

	return c.JSON(fiber.Map{
		"room_code": room.RoomCode,
		"room_name": room.RoomName,
		"buy_in":    room.BuyIn,
	})
}

// ListPublic backs the lobby: every public room plus the live seat counts and
// start countdowns the client needs to render it, and the code of the table
// this player is still seated at (if any) so they can be offered a way back.
func (h *RoomHandler) ListPublic(c fiber.Ctx) error {
	rooms, err := h.rooms.ListPublicRooms(c.Context())
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	out := make([]fiber.Map, 0, len(rooms))
	for _, r := range rooms {
		room := fiber.Map{
			"room_code":            r.RoomCode,
			"room_name":            r.RoomName,
			"buy_in":               r.BuyIn,
			"small_blind":          r.SmallBlind,
			"big_blind":            r.BigBlind,
			"max_players":          r.MaxPlayers,
			"player_count":         0,
			"game_started":         false,
			"countdown_active":     false,
			"countdown_ends_at_ms": int64(0),
		}
		if summary, ok := h.game.GetLiveSummary(r.RoomCode); ok {
			room["player_count"] = summary.PlayerCount
			room["game_started"] = summary.GameStarted
			room["countdown_active"] = summary.CountdownActive
			room["countdown_ends_at_ms"] = summary.CountdownEndsAtMs
		}
		out = append(out, room)
	}

	activeRoomCode := ""
	token := c.Cookies("poker_session")
	if token == "" {
		token = c.Get("Authorization")
	}
	if user, err := h.auth.GetUserByToken(c.Context(), token); err == nil {
		activeRoomCode, _ = h.game.GetSeatedRoomCode(user.ID)
	}

	return c.JSON(fiber.Map{
		"rooms":            out,
		"tiers":            services.PublicBlindTiers,
		"active_room_code": activeRoomCode,
	})
}
