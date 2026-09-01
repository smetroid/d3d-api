package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo"
	"github.com/smetroid/d3d-api/app/auth/socialauth"
)

// testSigningKey mirrors the value used across the auth test suites.
const testSigningKey = "test-signing-key"

// TestJWTMiddlewareRejectsOAuthStateToken is the regression test for the
// whole-branch finding C1: the OAuth `state` JWT must never be usable as a
// session token. providerURL is a public endpoint that hands out a state
// token signed with the session signing key; the JWT middleware only checks
// signature and expiry (no issuer check), so if the state token were signed
// with that same key it would be accepted as a session — a bearer credential
// for every protected route, mintable by anyone.
//
// Before the fix (state signed with the raw session key) this test fails:
// the middleware lets the request through. After the fix (state signed with
// a key derived from, but distinct from, the session key) it passes: the
// state token's signature no longer validates against the session key the
// middleware checks, so the request is rejected with 401.
func TestJWTMiddlewareRejectsOAuthStateToken(t *testing.T) {
	state, err := socialauth.GenerateState(testSigningKey)
	if err != nil {
		t.Fatalf("GenerateState: %v", err)
	}

	handlerCalled := false
	handler := JWTWithConfig(JWTConfig{
		SigningKey: []byte(testSigningKey),
	})(func(c echo.Context) error {
		handlerCalled = true
		return c.NoContent(http.StatusOK)
	})

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.Header.Set(echo.HeaderAuthorization, "Bearer "+state)
	rec := httptest.NewRecorder()
	ctx := e.NewContext(req, rec)

	handlerErr := handler(ctx)

	if handlerCalled {
		t.Fatal("OAuth state token reached the protected handler; it must never be accepted as a session credential")
	}
	he, ok := handlerErr.(*echo.HTTPError)
	if !ok {
		t.Fatalf("expected *echo.HTTPError, got %T (%v)", handlerErr, handlerErr)
	}
	if he.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", he.Code, http.StatusUnauthorized)
	}
}
