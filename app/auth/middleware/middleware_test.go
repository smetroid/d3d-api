package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	jwt "github.com/dgrijalva/jwt-go"
	"github.com/labstack/echo"
	"github.com/smetroid/d3d-api/app/auth/socialauth"
	"github.com/smetroid/d3d-api/app/auth/token"
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

// runThroughMiddleware presents rawToken as a bearer credential to a
// JWTWithConfig-wrapped handler signing against testSigningKey, and reports
// whether the handler was reached and what the middleware returned.
func runThroughMiddleware(t *testing.T, rawToken string) (handlerCalled bool, handlerErr error) {
	t.Helper()

	handler := JWTWithConfig(JWTConfig{
		SigningKey: []byte(testSigningKey),
	})(func(c echo.Context) error {
		handlerCalled = true
		return c.NoContent(http.StatusOK)
	})

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.Header.Set(echo.HeaderAuthorization, "Bearer "+rawToken)
	rec := httptest.NewRecorder()
	ctx := e.NewContext(req, rec)

	handlerErr = handler(ctx)
	return handlerCalled, handlerErr
}

// assertRejected fails the test unless the middleware returned a 401 and
// never reached the handler.
func assertRejected(t *testing.T, handlerCalled bool, handlerErr error) {
	t.Helper()

	if handlerCalled {
		t.Fatal("token reached the protected handler; it must not be accepted as a session credential")
	}
	he, ok := handlerErr.(*echo.HTTPError)
	if !ok {
		t.Fatalf("expected *echo.HTTPError, got %T (%v)", handlerErr, handlerErr)
	}
	if he.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", he.Code, http.StatusUnauthorized)
	}
}

// assertAccepted fails the test unless the middleware let the request reach
// the handler with a nil error.
func assertAccepted(t *testing.T, handlerCalled bool, handlerErr error) {
	t.Helper()

	if handlerErr != nil {
		t.Fatalf("unexpected middleware error: %v", handlerErr)
	}
	if !handlerCalled {
		t.Fatal("token was rejected but should have been accepted as a valid session credential")
	}
}

// TestJWTMiddlewareRejectsElementShareToken proves the live, currently
// exploitable privilege escalation described in element_shares.go: GET
// /catalog is public and hands every visitor a JWT signed with the raw
// session signing key, issuer "d3d-element-share", with `exp` set only when
// the underlying share has an expiry. The session middleware previously
// checked only signature and expiry — no issuer check — so this
// publicly-handed-out, potentially-never-expiring token was a valid bearer
// credential on every protected route, including unscoped DAG read/write/
// delete.
//
// This test mints the token exactly as listCatalog does for a
// non-expiring share: no `exp` claim at all. Before the fix, jwt-go v3
// treats a missing `exp` as valid and the token reaches the handler. After
// the fix, the middleware rejects it purely on `iss`, independent of `exp`.
func TestJWTMiddlewareRejectsElementShareToken(t *testing.T) {
	catalogToken, err := token.CreateToken(testSigningKey, jwt.MapClaims{
		"jti":      "share-jti",
		"iss":      "d3d-element-share",
		"share_id": "share-id",
		"role":     "view",
		// Deliberately no "exp": this mirrors a catalog entry whose share
		// has no expiry, which is the worst case — such a token would
		// otherwise never expire.
	})
	if err != nil {
		t.Fatalf("CreateToken: %v", err)
	}

	handlerCalled, handlerErr := runThroughMiddleware(t, catalogToken)
	assertRejected(t, handlerCalled, handlerErr)
}

// TestJWTMiddlewareRejectsSocialStateToken is belt-and-braces alongside the
// key-derivation fix for the OAuth state token (see
// TestJWTMiddlewareRejectsOAuthStateToken and socialauth.stateSigningKey).
// That fix already stops a real state token from validating as a session,
// because it's signed with a derived key. This test isolates the new
// issuer check itself: it signs a "d3d-social-state" token directly with
// the raw session signing key — something the derived-key defense alone
// would not catch if it were ever weakened or removed — and confirms the
// issuer check rejects it independently.
func TestJWTMiddlewareRejectsSocialStateToken(t *testing.T) {
	stateToken, err := token.CreateToken(testSigningKey, jwt.MapClaims{
		"iss": "d3d-social-state",
		"exp": time.Now().Add(10 * time.Minute).Unix(),
		"jti": "state-jti",
	})
	if err != nil {
		t.Fatalf("CreateToken: %v", err)
	}

	handlerCalled, handlerErr := runThroughMiddleware(t, stateToken)
	assertRejected(t, handlerCalled, handlerErr)
}

// TestJWTMiddlewareRejectsShareToken is the regression test for the route-
// scoping fix: a "d3d-share" token (minted by shares.go for anonymous DAG
// share links) must NOT pass the general session middleware (JWTWithConfig,
// backing AuthMiddleware). Share tokens are handed to untrusted external
// recipients and must reach only the small set of share-appropriate
// routes, which opt in via ShareJWTWithConfig (see
// TestShareJWTMiddlewareAcceptsShareToken below) paired with resource-
// binding middleware. Before this fix, a d3d-share token passed every
// auth-gated route with zero role checks.
func TestJWTMiddlewareRejectsShareToken(t *testing.T) {
	shareToken, err := token.CreateToken(testSigningKey, jwt.MapClaims{
		"jti":    "share-jti",
		"iss":    "d3d-share",
		"dag_id": "dag-id",
		"role":   "view",
		"exp":    time.Now().Add(7 * 24 * time.Hour).Unix(),
	})
	if err != nil {
		t.Fatalf("CreateToken: %v", err)
	}

	handlerCalled, handlerErr := runThroughMiddleware(t, shareToken)
	assertRejected(t, handlerCalled, handlerErr)
}

// runThroughShareMiddleware is runThroughMiddleware's counterpart for
// ShareJWTWithConfig.
func runThroughShareMiddleware(t *testing.T, rawToken string) (handlerCalled bool, handlerErr error) {
	t.Helper()

	handler := ShareJWTWithConfig(JWTConfig{
		SigningKey: []byte(testSigningKey),
	})(func(c echo.Context) error {
		handlerCalled = true
		return c.NoContent(http.StatusOK)
	})

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.Header.Set(echo.HeaderAuthorization, "Bearer "+rawToken)
	rec := httptest.NewRecorder()
	ctx := e.NewContext(req, rec)

	handlerErr = handler(ctx)
	return handlerCalled, handlerErr
}

// TestShareJWTMiddlewareAcceptsShareToken proves ShareJWTWithConfig (used
// only by share-accessible routes) accepts what the general middleware now
// rejects.
func TestShareJWTMiddlewareAcceptsShareToken(t *testing.T) {
	shareToken, err := token.CreateToken(testSigningKey, jwt.MapClaims{
		"jti":    "share-jti",
		"iss":    "d3d-share",
		"dag_id": "dag-id",
		"role":   "view",
		"exp":    time.Now().Add(7 * 24 * time.Hour).Unix(),
	})
	if err != nil {
		t.Fatalf("CreateToken: %v", err)
	}

	handlerCalled, handlerErr := runThroughShareMiddleware(t, shareToken)
	assertAccepted(t, handlerCalled, handlerErr)
}

// TestShareJWTMiddlewareAcceptsSessionToken proves ShareJWTWithConfig still
// accepts ordinary session tokens — share-accessible routes must remain
// reachable by logged-in owners, not just share recipients.
func TestShareJWTMiddlewareAcceptsSessionToken(t *testing.T) {
	sessionToken := token.CreateExpiringToken("alice", testSigningKey, time.Hour, "localauth")

	handlerCalled, handlerErr := runThroughShareMiddleware(t, sessionToken)
	assertAccepted(t, handlerCalled, handlerErr)
}

// TestShareJWTMiddlewareRejectsElementShareAndStateTokens proves
// ShareJWTWithConfig's narrower allowance is only for "d3d-share": the
// other special-purpose issuers stay denied.
func TestShareJWTMiddlewareRejectsElementShareAndStateTokens(t *testing.T) {
	catalogToken, err := token.CreateToken(testSigningKey, jwt.MapClaims{
		"jti":      "share-jti",
		"iss":      "d3d-element-share",
		"share_id": "share-id",
		"role":     "view",
	})
	if err != nil {
		t.Fatalf("CreateToken: %v", err)
	}
	handlerCalled, handlerErr := runThroughShareMiddleware(t, catalogToken)
	assertRejected(t, handlerCalled, handlerErr)

	stateToken, err := token.CreateToken(testSigningKey, jwt.MapClaims{
		"iss": "d3d-social-state",
		"exp": time.Now().Add(10 * time.Minute).Unix(),
		"jti": "state-jti",
	})
	if err != nil {
		t.Fatalf("CreateToken: %v", err)
	}
	handlerCalled, handlerErr = runThroughShareMiddleware(t, stateToken)
	assertRejected(t, handlerCalled, handlerErr)
}

// TestJWTMiddlewareAcceptsSessionToken is a regression guard: a normal
// login-issued session token must continue to pass, or every login breaks.
func TestJWTMiddlewareAcceptsSessionToken(t *testing.T) {
	sessionToken := token.CreateExpiringToken("alice", testSigningKey, time.Hour, "localauth")

	handlerCalled, handlerErr := runThroughMiddleware(t, sessionToken)
	assertAccepted(t, handlerCalled, handlerErr)
}

// TestJWTMiddlewareIssuerVerdicts is a table-driven guard against the whole
// class of bug recurring: every issuer minted with the session signing key
// (found by grepping `"iss":` literals in non-test Go — see the fix's
// commit for the inventory) must have an explicit, intentional verdict
// here. We chose a table over a source-scanning assertion because several
// issuers are computed at runtime (token.CreateExpiringToken's `backend`
// parameter is caller-supplied, e.g. "localauth", "ldap", "oauth",
// "google", "github") rather than being static string literals somewhere
// in the source for a scanner to collect reliably — a scan would either
// miss those or require hard-coding them anyway, which the table already
// does more legibly.
//
// When you add a new token kind signed with the session key, add a row
// here stating whether it may act as a session, and if not, add its issuer
// to rejectedSessionIssuers in middleware.go.
func TestJWTMiddlewareIssuerVerdicts(t *testing.T) {
	cases := []struct {
		name         string
		iss          string
		mayBeSession bool
	}{
		{"backend: localauth", "localauth", true},
		{"backend: ldap", "ldap", true},
		{"backend: oauth", "oauth", true},
		{"backend: google", "google", true},
		{"backend: github", "github", true},
		{"samus-token-tool", "samus-token-tool", true},
		{"d3d-share", "d3d-share", false},
		{"d3d-element-share", "d3d-element-share", false},
		{"d3d-social-state", "d3d-social-state", false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			claims := jwt.MapClaims{
				"jti": "jti",
				"iss": tc.iss,
				"exp": time.Now().Add(time.Hour).Unix(),
			}
			tok, err := token.CreateToken(testSigningKey, claims)
			if err != nil {
				t.Fatalf("CreateToken: %v", err)
			}

			handlerCalled, handlerErr := runThroughMiddleware(t, tok)
			if tc.mayBeSession {
				assertAccepted(t, handlerCalled, handlerErr)
			} else {
				assertRejected(t, handlerCalled, handlerErr)
			}
		})
	}
}
