package handlers

import (
	"os"
	"strconv"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/zuse/poker5/go-poker/services"
)

const sessionCookieName = "poker_session"

// sessionCookieTTL matches services.SessionIdleTTL. The cookie is refreshed on
// the same schedule the session row slides on, so the browser never discards a
// cookie whose session is still alive server-side.
const sessionCookieTTL = services.SessionIdleTTL

// secureCookiesEnabled reports whether session cookies should carry the Secure
// attribute, from SESSION_COOKIE_SECURE.
//
// It defaults to OFF. That is the wrong default for production and the right
// one for this project's workflow: a Secure cookie is dropped by the browser
// over plain HTTP, so defaulting it on would silently break sign-in both on an
// HTTP deployment and when testing against a LAN IP from a phone. The server
// logs a warning on every start while it is off, so turning it on stays a
// visible to-do rather than a silent one.
func secureCookiesEnabled() bool {
	raw := os.Getenv("SESSION_COOKIE_SECURE")
	if raw == "" {
		return false
	}
	enabled, err := strconv.ParseBool(raw)
	if err != nil {
		return false
	}
	return enabled
}

// sessionCookie builds the login cookie. Every attribute is set explicitly:
// the clearing cookie in Logout has to match on name, path and domain for the
// browser to overwrite it, so both are built from the same shape.
func sessionCookie(value string, expires time.Time, secure bool) *fiber.Cookie {
	return &fiber.Cookie{
		Name:     sessionCookieName,
		Value:    value,
		Path:     "/",
		Expires:  expires,
		HTTPOnly: true,
		Secure:   secure,
		SameSite: fiber.CookieSameSiteLaxMode,
	}
}
