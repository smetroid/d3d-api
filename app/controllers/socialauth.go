package controllers

import (
	"errors"
	"net/http"

	jwt "github.com/dgrijalva/jwt-go"
	"github.com/labstack/echo"
	"golang.org/x/oauth2"

	"github.com/smetroid/d3d-api/app/auth/socialauth"
	"github.com/smetroid/d3d-api/app/auth/token"
	"github.com/smetroid/d3d-api/app/config"
	"github.com/smetroid/d3d-api/app/db/postgres"
	"github.com/smetroid/d3d-api/app/models"
)

type SocialAuthController struct {
	Echo           *echo.Echo
	DB             *postgres.Postgres
	SigningKey     string
	CookieSecure   bool
	Google         config.SocialProvider
	GitHub         config.SocialProvider
	AuthMiddleware echo.MiddlewareFunc
}

type socialCallbackRequest struct {
	Code     string `json:"code"`
	State    string `json:"state"`
	Provider string `json:"provider"`
}

func (sac *SocialAuthController) Init() {
	// Public: the caller has no session yet.
	sac.Echo.GET("/auth/:provider/url", sac.providerURL)
	sac.Echo.POST("/auth/social/callback", sac.callback)
	sac.Echo.POST("/auth/logout", sac.logout)
	// Authenticated: reports who the cookie belongs to.
	sac.Echo.GET("/auth/me", sac.me, sac.AuthMiddleware)
}

// oauthConfig returns the OAuth client for a provider name, or nil if the
// provider is unknown or unconfigured.
func (sac *SocialAuthController) oauthConfig(provider string) *oauth2.Config {
	switch provider {
	case "google":
		if sac.Google.ClientID == "" {
			return nil
		}
		return socialauth.NewGoogleConfig(sac.Google.ClientID, sac.Google.ClientSecret, sac.Google.RedirectURL)
	case "github":
		if sac.GitHub.ClientID == "" {
			return nil
		}
		return socialauth.NewGitHubConfig(sac.GitHub.ClientID, sac.GitHub.ClientSecret, sac.GitHub.RedirectURL)
	default:
		return nil
	}
}

// providerURL hands the frontend a consent URL carrying a fresh signed state.
func (sac *SocialAuthController) providerURL(ctx echo.Context) error {
	cfg := sac.oauthConfig(ctx.Param("provider"))
	if cfg == nil {
		return ctx.JSON(http.StatusNotFound, models.ErrorResponse("Unknown or unconfigured provider"))
	}

	state, err := socialauth.GenerateState(sac.SigningKey)
	if err != nil {
		return ctx.JSON(http.StatusInternalServerError, models.ErrorResponse("Could not start login"))
	}

	return ctx.JSON(http.StatusOK, map[string]string{
		"url": cfg.AuthCodeURL(state, oauth2.AccessTypeOnline),
	})
}

// callback completes the handshake: verify state, exchange the code, upsert
// the account, and hand back a session cookie.
func (sac *SocialAuthController) callback(ctx echo.Context) error {
	var req socialCallbackRequest
	if err := ctx.Bind(&req); err != nil || req.Code == "" || req.State == "" {
		return ctx.JSON(http.StatusBadRequest, models.ErrorResponse("Invalid callback request"))
	}

	if err := socialauth.ValidateState(req.State, sac.SigningKey); err != nil {
		return ctx.JSON(http.StatusUnauthorized, models.ErrorResponse("Invalid or expired login attempt"))
	}

	cfg := sac.oauthConfig(req.Provider)
	if cfg == nil {
		return ctx.JSON(http.StatusNotFound, models.ErrorResponse("Unknown or unconfigured provider"))
	}

	var (
		profile socialauth.SocialUserProfile
		err     error
	)
	switch req.Provider {
	case "google":
		profile, err = socialauth.FetchGoogleProfile(ctx.Request().Context(), cfg, req.Code)
	case "github":
		profile, err = socialauth.FetchGitHubProfile(ctx.Request().Context(), cfg, req.Code)
	}
	if err != nil {
		// The provider, not the caller, is at fault.
		return ctx.JSON(http.StatusBadGateway, models.ErrorResponse("Could not reach the identity provider"))
	}

	user, err := sac.DB.UpsertSocialUser(profile)
	if err != nil {
		if errors.Is(err, postgres.ErrUsernameTaken) {
			return ctx.JSON(http.StatusConflict, models.ErrorResponse("That username is already taken by another account"))
		}
		return ctx.JSON(http.StatusInternalServerError, models.ErrorResponse("Could not create the account"))
	}

	SetSessionCookie(ctx, token.CreateExpiringToken(user.Username, sac.SigningKey, SessionTTL, profile.Provider), sac.CookieSecure)
	return ctx.JSON(http.StatusOK, map[string]interface{}{"user": user})
}

// logout drops the session cookie. It is public so an expired session can
// still be cleaned up.
func (sac *SocialAuthController) logout(ctx echo.Context) error {
	ClearSessionCookie(ctx, sac.CookieSecure)
	return ctx.JSON(http.StatusOK, map[string]string{"status": "ok"})
}

// me reports the signed-in user. The frontend cannot read the httpOnly cookie,
// so this is its only way to learn who it is.
func (sac *SocialAuthController) me(ctx echo.Context) error {
	tok, ok := ctx.Get("user").(*jwt.Token)
	if !ok {
		return ctx.JSON(http.StatusUnauthorized, models.ErrorResponse("Not signed in"))
	}
	claims, ok := tok.Claims.(jwt.MapClaims)
	if !ok {
		return ctx.JSON(http.StatusUnauthorized, models.ErrorResponse("Not signed in"))
	}
	username, _ := claims["jti"].(string)
	if username == "" {
		return ctx.JSON(http.StatusUnauthorized, models.ErrorResponse("Not signed in"))
	}

	user, err := sac.DB.GetUser(username)
	if err != nil {
		return ctx.JSON(http.StatusInternalServerError, models.ErrorResponse("Could not load the account"))
	}
	if user.Id == "" {
		return ctx.JSON(http.StatusUnauthorized, models.ErrorResponse("Not signed in"))
	}

	return ctx.JSON(http.StatusOK, map[string]interface{}{"user": user})
}
