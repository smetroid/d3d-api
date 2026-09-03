package controllers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo"
)

// stubAuthProvider authenticates exactly one credential pair.
type stubAuthProvider struct{ token string }

func (s *stubAuthProvider) Authenticate(username, password string) (bool, string, error) {
	if username == "alice" && password == "correct" {
		return true, s.token, nil
	}
	return false, "", nil
}

func (s *stubAuthProvider) SetSigningKey(key string) {}
func (s *stubAuthProvider) Connect() error           { return nil }
func (s *stubAuthProvider) Close()                   {}

func postLogin(t *testing.T, ac *AuthController, body string) *httptest.ResponseRecorder {
	t.Helper()
	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/auth/login", strings.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	if err := ac.LoginHandler(e.NewContext(req, rec)); err != nil {
		t.Fatalf("handler: %v", err)
	}
	return rec
}

func TestLoginSetsSessionCookie(t *testing.T) {
	ac := &AuthController{
		AuthProvider: &stubAuthProvider{token: "jwt-abc"},
		CookieSecure: true,
	}
	rec := postLogin(t, ac, `{"username":"alice","password":"correct"}`)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	cookies := rec.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("got %d cookies, want 1", len(cookies))
	}
	c := cookies[0]
	if c.Name != SessionCookieName || c.Value != "jwt-abc" {
		t.Errorf("cookie = %+v", c)
	}
	if !c.HttpOnly || c.SameSite != http.SameSiteLaxMode {
		t.Errorf("HttpOnly = %v, SameSite = %v", c.HttpOnly, c.SameSite)
	}
	// The body still carries the token so any un-migrated caller keeps working.
	if !strings.Contains(rec.Body.String(), "jwt-abc") {
		t.Errorf("body = %s, want it to still contain the token", rec.Body.String())
	}
}

func TestFailedLoginSetsNoCookie(t *testing.T) {
	ac := &AuthController{
		AuthProvider: &stubAuthProvider{token: "jwt-abc"},
		CookieSecure: true,
	}
	rec := postLogin(t, ac, `{"username":"alice","password":"wrong"}`)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rec.Code)
	}
	if len(rec.Result().Cookies()) != 0 {
		t.Error("a failed login must not set a session cookie")
	}
}
