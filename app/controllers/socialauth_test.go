package controllers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	jwt "github.com/dgrijalva/jwt-go"
	"github.com/google/uuid"
	"github.com/labstack/echo"
	"github.com/smetroid/d3d-api/app/auth/socialauth"
	"github.com/smetroid/d3d-api/app/auth/token"
	"github.com/smetroid/d3d-api/app/config"
	"github.com/smetroid/d3d-api/app/db/postgres"
	"github.com/smetroid/d3d-api/app/models"
)

// newSocialDBController opens the repository on TEST_DATABASE_URL and clears
// the users table so each DB-backed test starts clean. It follows the same
// shape as newTestController in element_shares_test.go.
func newSocialDBController(t *testing.T) *SocialAuthController {
	t.Helper()
	dsn := os.Getenv(testDSNEnv)
	if dsn == "" {
		t.Skipf("%s not set; skipping controller integration tests", testDSNEnv)
	}
	p := &postgres.Postgres{DSN: dsn}
	if err := p.Init(); err != nil {
		t.Fatalf("init: %v", err)
	}
	t.Cleanup(func() { p.Pool().Close() })

	if _, err := p.Pool().Exec(context.Background(), `TRUNCATE users CASCADE`); err != nil {
		t.Fatalf("truncate: %v", err)
	}
	return &SocialAuthController{DB: p, SigningKey: testSigningKey}
}

// authedContext parses raw into a *jwt.Token and installs it under the
// "user" context key the way the JWT middleware would, per the pattern in
// app/controllers/dag.go:215-228.
func authedContext(t *testing.T, e *echo.Echo, req *http.Request, rec *httptest.ResponseRecorder, raw string) echo.Context {
	t.Helper()
	tok, err := jwt.Parse(raw, func(tt *jwt.Token) (interface{}, error) {
		if _, ok := tt.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method %v", tt.Header["alg"])
		}
		return []byte(testSigningKey), nil
	})
	if err != nil || !tok.Valid {
		t.Fatalf("parse token: %v", err)
	}
	ctx := e.NewContext(req, rec)
	ctx.Set("user", tok)
	return ctx
}

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

func TestMeReturnsAuthenticatedUser(t *testing.T) {
	sac := newSocialDBController(t)

	username := "me-happy-" + uuid.New().String()
	if err := sac.DB.CreateUser(models.User{Username: username, PasswordHash: "hash", CreatedAt: time.Now()}); err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	raw := token.CreateExpiringToken(username, testSigningKey, SessionTTL, "localauth")

	e := echo.New()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/auth/me", nil)
	ctx := authedContext(t, e, req, rec, raw)

	if err := sac.me(ctx); err != nil {
		t.Fatalf("handler: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}

	if strings.Contains(rec.Body.String(), "hash") {
		t.Errorf("response body leaks password_hash: %s", rec.Body.String())
	}

	var out struct {
		User models.User `json:"user"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if out.User.Username != username {
		t.Errorf("username = %q, want %q", out.User.Username, username)
	}
}

func TestMeRejectsUnknownUser(t *testing.T) {
	sac := newSocialDBController(t)

	username := "me-ghost-" + uuid.New().String()
	raw := token.CreateExpiringToken(username, testSigningKey, SessionTTL, "localauth")

	e := echo.New()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/auth/me", nil)
	ctx := authedContext(t, e, req, rec, raw)

	if err := sac.me(ctx); err != nil {
		t.Fatalf("handler: %v", err)
	}
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401 (user deleted after token was issued), body = %s", rec.Code, rec.Body.String())
	}
}

func TestMeRejectsMissingToken(t *testing.T) {
	sac := newSocialController()
	e := echo.New()
	rec := httptest.NewRecorder()
	ctx := e.NewContext(httptest.NewRequest(http.MethodGet, "/auth/me", nil), rec)

	if err := sac.me(ctx); err != nil {
		t.Fatalf("handler: %v", err)
	}
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rec.Code)
	}
}

func TestProviderURLRejectsUnconfiguredCredentials(t *testing.T) {
	sac := &SocialAuthController{
		SigningKey:   testSigningKey,
		CookieSecure: true,
		Google:       config.SocialProvider{RedirectURL: "http://localhost:5173/auth/callback"},
		GitHub:       config.SocialProvider{RedirectURL: "http://localhost:5173/auth/callback"},
	}
	e := echo.New()
	rec := httptest.NewRecorder()
	ctx := e.NewContext(httptest.NewRequest(http.MethodGet, "/auth/github/url", nil), rec)
	ctx.SetParamNames("provider")
	ctx.SetParamValues("github")

	if err := sac.providerURL(ctx); err != nil {
		t.Fatalf("handler: %v", err)
	}
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404 (github is a known name but has no ClientID configured)", rec.Code)
	}
	if len(rec.Result().Cookies()) != 0 {
		t.Error("no cookie may be set for an unconfigured provider")
	}
}
