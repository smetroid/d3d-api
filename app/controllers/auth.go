package controllers

import (
	"net/http"

	"github.com/labstack/echo"
	"github.com/smetroid/d3d-api/app/auth"
	"github.com/smetroid/d3d-api/app/models"
)

type AuthController struct {
	Echo         *echo.Echo
	AuthProvider auth.AuthProvider
	SigningKey   string
	CookieSecure bool
}

func (ac *AuthController) Init() {
	ac.Echo.POST("/auth/login", ac.LoginHandler)
}

// Handles login request
func (ac *AuthController) LoginHandler(ctx echo.Context) error {
	var loginRequest models.LoginRequest
	err := ctx.Bind(&loginRequest)

	if err != nil || loginRequest.Username == "" || loginRequest.Password == "" {
		return ctx.JSON(http.StatusUnauthorized, models.ErrorResponse("Invalid login request"))
	}

	loginSuccess, token, err := ac.AuthProvider.Authenticate(loginRequest.Username, loginRequest.Password)

	if err != nil || !loginSuccess {
		return ctx.JSON(http.StatusUnauthorized, models.ErrorResponse("Login failed"))
	}

	authToken := models.AuthToken{Token: token}

	// Local logins get the same httpOnly cookie as social logins, so the
	// frontend has one session mechanism rather than two.
	SetSessionCookie(ctx, token, ac.CookieSecure)

	// The token also stays in the body during the migration; the frontend
	// stops reading it in the d3dweb work but other callers may not have.
	return ctx.JSON(http.StatusOK, authToken)
}
