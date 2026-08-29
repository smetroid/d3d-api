package controllers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/labstack/echo"
	"github.com/smetroid/d3d-api/app/auth/socialauth"
	"github.com/smetroid/d3d-api/app/config"
)

func newSocialController() *SocialAuthController {
	return &SocialAuthController{
		SigningKey:   testSigningKey,
		CookieSecure: true,
		Google: config.SocialProvider{
			ClientID:    "g-id",
			RedirectURL: "http://localhost:5173/auth/callback",
		},
		GitHub: config.SocialProvider{
			ClientID:    "gh-id",
			RedirectURL: "http://localhost:5173/auth/callback",
		},
	}
}

func TestProviderURLIncludesValidState(t *testing.T) {
	sac := newSocialController()
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/auth/github/url", nil)
	rec := httptest.NewRecorder()
	ctx := e.NewContext(req, rec)
	ctx.SetParamNames("provider")
	ctx.SetParamValues("github")

	if err := sac.providerURL(ctx); err != nil {
		t.Fatalf("handler: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}

	var body struct {
		URL string `json:"url"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !strings.Contains(body.URL, "github.com") {
		t.Errorf("url = %q, want a GitHub authorize URL", body.URL)
	}

	parsed, err := url.Parse(body.URL)
	if err != nil {
		t.Fatalf("parse url: %v", err)
	}
	state := parsed.Query().Get("state")
	if state == "" {
		t.Fatal("authorize URL carries no state parameter")
	}
	if err := socialauth.ValidateState(state, testSigningKey); err != nil {
		t.Errorf("state does not validate: %v", err)
	}
}

func TestProviderURLRejectsUnknownProvider(t *testing.T) {
	sac := newSocialController()
	e := echo.New()
	rec := httptest.NewRecorder()
	ctx := e.NewContext(httptest.NewRequest(http.MethodGet, "/auth/myspace/url", nil), rec)
	ctx.SetParamNames("provider")
	ctx.SetParamValues("myspace")

	if err := sac.providerURL(ctx); err != nil {
		t.Fatalf("handler: %v", err)
	}
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rec.Code)
	}
}

func TestCallbackRejectsInvalidState(t *testing.T) {
	sac := newSocialController()
	e := echo.New()
	body := strings.NewReader(`{"code":"c","state":"not-a-jwt","provider":"github"}`)
	req := httptest.NewRequest(http.MethodPost, "/auth/social/callback", body)
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()

	if err := sac.callback(e.NewContext(req, rec)); err != nil {
		t.Fatalf("handler: %v", err)
	}
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rec.Code)
	}
	if len(rec.Result().Cookies()) != 0 {
		t.Error("no cookie may be set for a rejected state")
	}
}

func TestLogoutClearsCookie(t *testing.T) {
	sac := newSocialController()
	e := echo.New()
	rec := httptest.NewRecorder()

	if err := sac.logout(e.NewContext(httptest.NewRequest(http.MethodPost, "/auth/logout", nil), rec)); err != nil {
		t.Fatalf("handler: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}

	cookies := rec.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("got %d cookies, want 1", len(cookies))
	}
	c := cookies[0]
	if c.Name != SessionCookieName {
		t.Errorf("name = %q, want %q", c.Name, SessionCookieName)
	}
	if c.MaxAge >= 0 {
		t.Errorf("MaxAge = %d, want negative so the browser drops it", c.MaxAge)
	}
	if !c.HttpOnly {
		t.Error("cleared cookie must still be HttpOnly")
	}
}

func TestSetSessionCookieAttributes(t *testing.T) {
	e := echo.New()
	rec := httptest.NewRecorder()
	SetSessionCookie(e.NewContext(httptest.NewRequest(http.MethodGet, "/", nil), rec), "jwt-value", true)

	cookies := rec.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("got %d cookies, want 1", len(cookies))
	}
	c := cookies[0]
	if c.Value != "jwt-value" || c.Path != "/" {
		t.Errorf("cookie = %+v", c)
	}
	if !c.HttpOnly || !c.Secure {
		t.Errorf("HttpOnly = %v, Secure = %v; want both true", c.HttpOnly, c.Secure)
	}
	// SameSite=None would be a third-party cookie between the two vercel.app
	// sites and Safari would drop it. Lax is required.
	if c.SameSite != http.SameSiteLaxMode {
		t.Errorf("SameSite = %v, want Lax", c.SameSite)
	}
	if c.MaxAge != int(SessionTTL.Seconds()) {
		t.Errorf("MaxAge = %d, want %d", c.MaxAge, int(SessionTTL.Seconds()))
	}
}
