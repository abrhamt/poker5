package handlers

import (
	"github.com/gofiber/fiber/v3"
	"github.com/zuse/poker5/go-poker/repository"
	"github.com/zuse/poker5/go-poker/services"
)

type ViewHandler struct {
	auth   *services.AuthService
	wallet *services.WalletService
	rooms  *services.RoomService
	game   *services.GameService
}

func NewViewHandler(auth *services.AuthService, wallet *services.WalletService, rooms *services.RoomService, game *services.GameService) *ViewHandler {
	return &ViewHandler{
		auth:   auth,
		wallet: wallet,
		rooms:  rooms,
		game:   game,
	}
}

// homeRouteFor sends admin accounts straight to the admin panel — admins
// aren't players and have no business in the lobby/wallet/table views.
func homeRouteFor(user *repository.User) string {
	if user.Role == "admin" {
		return "/admin"
	}
	return "/lobby"
}

func (h *ViewHandler) RenderHome(c fiber.Ctx) error {
	token := c.Cookies("poker_session")
	if token != "" {
		user, err := h.auth.GetUserByToken(c.Context(), token)
		if err == nil {
			return c.Redirect().To(homeRouteFor(user))
		}
	}
	return c.Redirect().To("/login")
}

func (h *ViewHandler) RenderLogin(c fiber.Ctx) error {
	token := c.Cookies("poker_session")
	if token != "" {
		user, err := h.auth.GetUserByToken(c.Context(), token)
		if err == nil {
			return c.Redirect().To(homeRouteFor(user))
		}
	}
	html := renderBaseLayout("Sign In", renderLoginHTML(), "login", "", 0, false)
	c.Set("Content-Type", "text/html; charset=utf-8")
	return c.SendString(html)
}

func (h *ViewHandler) RenderRegister(c fiber.Ctx) error {
	token := c.Cookies("poker_session")
	if token != "" {
		user, err := h.auth.GetUserByToken(c.Context(), token)
		if err == nil {
			return c.Redirect().To(homeRouteFor(user))
		}
	}
	html := renderBaseLayout("Create Account", renderRegisterHTML(), "register", "", 0, false)
	c.Set("Content-Type", "text/html; charset=utf-8")
	return c.SendString(html)
}

func (h *ViewHandler) RenderLobby(c fiber.Ctx) error {
	token := c.Cookies("poker_session")
	user, err := h.auth.GetUserByToken(c.Context(), token)
	if err != nil {
		return c.Redirect().To("/login")
	}
	if user.Role == "admin" {
		return c.Redirect().To("/admin")
	}

	publicRooms, _ := h.rooms.ListPublicRooms(c.Context())
	live := h.buildLiveSummaries(publicRooms)
	activeRoomCode, _ := h.game.GetSeatedRoomCode(user.ID)
	bodyHTML := renderLobbyHTML(publicRooms, live, activeRoomCode)
	html := renderBaseLayout("Lobby", bodyHTML, "lobby", user.Username, user.Wallet, false)
	c.Set("Content-Type", "text/html; charset=utf-8")
	return c.SendString(html)
}

// RenderLobbyRooms serves the polled public-room-list fragment (see the
// hx-trigger="every 5s" on #public-rooms-section) so seat counts and
// join-countdowns on the lobby stay live without a full page refresh.
func (h *ViewHandler) RenderLobbyRooms(c fiber.Ctx) error {
	token := c.Cookies("poker_session")
	user, err := h.auth.GetUserByToken(c.Context(), token)
	if err != nil {
		return c.Redirect().To("/login")
	}
	if user.Role == "admin" {
		return c.Redirect().To("/admin")
	}

	publicRooms, _ := h.rooms.ListPublicRooms(c.Context())
	live := h.buildLiveSummaries(publicRooms)
	html := renderPublicRoomsSection(publicRooms, live)
	c.Set("Content-Type", "text/html; charset=utf-8")
	return c.SendString(html)
}

func (h *ViewHandler) buildLiveSummaries(rooms []repository.PokerRoom) map[string]services.TableLiveSummary {
	live := make(map[string]services.TableLiveSummary, len(rooms))
	for _, r := range rooms {
		if summary, ok := h.game.GetLiveSummary(r.RoomCode); ok {
			live[r.RoomCode] = summary
		}
	}
	return live
}

func (h *ViewHandler) RenderWallet(c fiber.Ctx) error {
	token := c.Cookies("poker_session")
	user, err := h.auth.GetUserByToken(c.Context(), token)
	if err != nil {
		return c.Redirect().To("/login")
	}
	if user.Role == "admin" {
		return c.Redirect().To("/admin")
	}

	txs, _ := h.wallet.GetUserTransactions(c.Context(), user.ID)
	bodyHTML := renderWalletHTML(user, txs)
	html := renderBaseLayout("Wallet", bodyHTML, "wallet", user.Username, user.Wallet, false)
	c.Set("Content-Type", "text/html; charset=utf-8")
	return c.SendString(html)
}

func (h *ViewHandler) RenderTable(c fiber.Ctx) error {
	roomCode := c.Params("id")
	token := c.Cookies("poker_session")
	user, err := h.auth.GetUserByToken(c.Context(), token)
	if err != nil {
		return c.Redirect().To("/login")
	}
	if user.Role == "admin" {
		return c.Redirect().To("/admin")
	}

	state := h.game.GetTableState(c.Context(), roomCode, user.Username)

	if c.Get("HX-Request") == "true" && c.Get("HX-Target") == "table-view-container" {
		partialHTML := renderTablePartialHTML(roomCode, user.Username, user.Wallet, state)
		c.Set("Content-Type", "text/html; charset=utf-8")
		return c.SendString(partialHTML)
	}

	html := renderTablePageHTML(roomCode, user.Username, user.Wallet, false, state)
	c.Set("Content-Type", "text/html; charset=utf-8")
	return c.SendString(html)
}
