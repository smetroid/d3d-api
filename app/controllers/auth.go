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

	authToken := models.AuthToken{token}

	//	cookie := new(http.Cookie)
	//	cookie.Name = "jwt_token"
	//	cookie.Value = authToken.String()
	//	log.Println(authToken.String())
	//	cookie.Expires = time.Now().Add(24 * time.Hour)
	//	ctx.SetCookie(cookie)
	//	log.Println(cookie)

	return ctx.JSON(http.StatusOK, authToken)
}
