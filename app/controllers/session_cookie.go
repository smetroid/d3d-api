package controllers

import (
	"net/http"
	"time"

	"github.com/labstack/echo"
)

// SessionCookieName is the cookie the JWT middleware reads via
// TokenLookup "cookie:jwt_token".
const SessionCookieName = "jwt_token"

// SessionTTL matches the expiry baked into the JWT itself.
const SessionTTL = 48 * time.Hour

// SetSessionCookie writes the session JWT as an httpOnly cookie.
//
// SameSite is Lax, never None: the frontend reaches the API through a
// same-origin path rewrite, so the cookie is first-party. A None cookie would
// be third-party between two vercel.app sites (vercel.app is on the Public
// Suffix List) and Safari would refuse it outright.
//
// secure is configuration-driven because local dev runs over plain HTTP.
func SetSessionCookie(ctx echo.Context, jwt string, secure bool) {
	ctx.SetCookie(&http.Cookie{
		Name:     SessionCookieName,
		Value:    jwt,
		Path:     "/",
		MaxAge:   int(SessionTTL.Seconds()),
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
	})
}

// ClearSessionCookie expires the session cookie. Every attribute except MaxAge
// must match SetSessionCookie or the browser will keep the original.
func ClearSessionCookie(ctx echo.Context, secure bool) {
	ctx.SetCookie(&http.Cookie{
		Name:     SessionCookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
	})
}
