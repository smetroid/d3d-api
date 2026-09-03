package controllers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	jwt "github.com/dgrijalva/jwt-go"
	"github.com/google/uuid"
	"github.com/labstack/echo"
	authmw "github.com/smetroid/d3d-api/app/auth/middleware"
	"github.com/smetroid/d3d-api/app/auth/token"
	"github.com/smetroid/d3d-api/app/db/postgres"
	"github.com/smetroid/d3d-api/app/models"
	"github.com/smetroid/d3d-api/app/services"
)

// shareScopeTestApp wires a real *echo.Echo with the actual route
// registrations and the actual production middleware chains (not
// handler-direct calls), because this suite exists to prove the routing
// and middleware wiring itself — layer 1 (which routes a d3d-share token
// may reach) and layer 2 (whether it's bound to the requested diagram) —
// rather than any single handler's internal logic.
type shareScopeTestApp struct {
	echo *echo.Echo
	db   *postgres.Postgres
}

func newShareScopeTestApp(t *testing.T) *shareScopeTestApp {
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

	if _, err := p.Pool().Exec(context.Background(), `TRUNCATE dags CASCADE`); err != nil {
		t.Fatalf("truncate: %v", err)
	}

	e := echo.New()

	authMiddleware := authmw.JWTWithConfig(authmw.JWTConfig{
		SigningKey: []byte(testSigningKey),
	})
	shareMiddleware := []echo.MiddlewareFunc{
		authmw.ShareJWTWithConfig(authmw.JWTConfig{
			SigningKey: []byte(testSigningKey),
		}),
		ShareResourceBinding(p),
	}

	dagController := &DAGsController{
		Echo:            e,
		DAGService:      services.DAGService{DB: p},
		DB:              p,
		AuthMiddleware:  authMiddleware,
		ShareMiddleware: shareMiddleware,
	}
	dagController.Init()

	menuController := &MenuController{
		Echo:            e,
		MenuService:     services.MenuService{DB: p},
		AuthMiddleware:  authMiddleware,
		ShareMiddleware: shareMiddleware,
	}
	menuController.Init()

	companiesController := &CompaniesController{
		Echo:           e,
		DB:             p,
		AuthMiddleware: authMiddleware,
	}
	companiesController.Init()

	elementSharesController := &ElementSharesController{
		Echo:           e,
		DB:             p,
		SigningKey:     testSigningKey,
		AuthMiddleware: authMiddleware,
	}
	elementSharesController.Init()

	return &shareScopeTestApp{echo: e, db: p}
}

// seedShareScopeDAG inserts a minimal DAG directly via the service and
// returns its id. The name is randomized per call: DAGService.ProcessDAG
// dedups on (name, description, diagram) via FindRelatedDAG, so two calls
// with an identical name/diagram would silently collapse onto the same
// row — fatal for these tests, which rely on two genuinely distinct
// diagram ids to prove cross-diagram binding.
func seedShareScopeDAG(t *testing.T, app *shareScopeTestApp) string {
	t.Helper()
	svc := services.DAGService{DB: app.db}
	id, err := svc.ProcessDAG(models.Dag{
		Name:    "seed-" + uuid.New().String(),
		Diagram: `{"nodes":[],"edges":[]}`,
	})
	if err != nil {
		t.Fatalf("seed dag: %v", err)
	}
	return id
}

// seedShareScopeShare inserts a share row bound to dagId and returns a
// d3d-share JWT whose jti matches that row, so ShareResourceBinding's
// Postgres.GetShareByJti lookup resolves to a real, dag-bound share —
// exercising layer 2 for real instead of trusting the token's own
// (attacker-controlled) dag_id claim.
func seedShareScopeShare(t *testing.T, app *shareScopeTestApp, dagId, role string) string {
	t.Helper()
	jti := uuid.New().String()
	if err := app.db.CreateShare(models.Share{
		Id:        uuid.New().String(),
		DagId:     dagId,
		Jti:       jti,
		Role:      role,
		CreatedBy: "alice",
		ExpiresAt: time.Now().Add(time.Hour),
		CreatedAt: time.Now(),
	}); err != nil {
		t.Fatalf("seed share: %v", err)
	}
	raw, err := token.CreateToken(testSigningKey, jwt.MapClaims{
		"jti":    jti,
		"iss":    "d3d-share",
		"dag_id": dagId,
		"role":   role,
		"exp":    time.Now().Add(time.Hour).Unix(),
	})
	if err != nil {
		t.Fatalf("CreateToken: %v", err)
	}
	return raw
}

// shareScopeSessionToken mints a normal (non-share) session token.
func shareScopeSessionToken(t *testing.T) string {
	t.Helper()
	return token.CreateExpiringToken("alice", testSigningKey, time.Hour, "localauth")
}

// doRequest issues method/path through the real router with the given
// bearer token (or no Authorization header if raw == "") and returns the
// response status code.
func doRequest(app *shareScopeTestApp, method, path, raw string) int {
	req := httptest.NewRequest(method, path, nil)
	if raw != "" {
		req.Header.Set(echo.HeaderAuthorization, "Bearer "+raw)
	}
	rec := httptest.NewRecorder()
	app.echo.ServeHTTP(rec, req)
	return rec.Code
}

// ─── The five share-accessible routes ──────────────────────────────────────

func TestShareScope_GetDAG(t *testing.T) {
	app := newShareScopeTestApp(t)
	dagId := seedShareScopeDAG(t, app)
	otherDagId := seedShareScopeDAG(t, app)

	t.Run("share token for this diagram allowed", func(t *testing.T) {
		tok := seedShareScopeShare(t, app, dagId, "view")
		if code := doRequest(app, http.MethodGet, "/dag/"+dagId, tok); code != http.StatusOK {
			t.Fatalf("status = %d, want 200", code)
		}
	})

	t.Run("share token for a different diagram rejected", func(t *testing.T) {
		tok := seedShareScopeShare(t, app, otherDagId, "view")
		if code := doRequest(app, http.MethodGet, "/dag/"+dagId, tok); code != http.StatusForbidden {
			t.Fatalf("status = %d, want 403 (cross-diagram share must be rejected)", code)
		}
	})

	t.Run("session token allowed", func(t *testing.T) {
		if code := doRequest(app, http.MethodGet, "/dag/"+dagId, shareScopeSessionToken(t)); code != http.StatusOK {
			t.Fatalf("status = %d, want 200", code)
		}
	})
}

func TestShareScope_UpdateDAG(t *testing.T) {
	app := newShareScopeTestApp(t)
	dagId := seedShareScopeDAG(t, app)
	otherDagId := seedShareScopeDAG(t, app)

	body := `{"diagram":"{\"nodes\":[],\"edges\":[]}"}`

	postJSON := func(path, raw string) int {
		req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
		req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
		if raw != "" {
			req.Header.Set(echo.HeaderAuthorization, "Bearer "+raw)
		}
		rec := httptest.NewRecorder()
		app.echo.ServeHTTP(rec, req)
		return rec.Code
	}

	t.Run("edit share token for this diagram allowed", func(t *testing.T) {
		tok := seedShareScopeShare(t, app, dagId, "edit")
		if code := postJSON("/dag/"+dagId+"/update", tok); code != http.StatusOK {
			t.Fatalf("status = %d, want 200", code)
		}
	})

	t.Run("edit share token for a different diagram rejected", func(t *testing.T) {
		tok := seedShareScopeShare(t, app, otherDagId, "edit")
		if code := postJSON("/dag/"+dagId+"/update", tok); code != http.StatusForbidden {
			t.Fatalf("status = %d, want 403 (cross-diagram share must be rejected)", code)
		}
	})

	t.Run("session token allowed", func(t *testing.T) {
		if code := postJSON("/dag/"+dagId+"/update", shareScopeSessionToken(t)); code != http.StatusOK {
			t.Fatalf("status = %d, want 200", code)
		}
	})
}

func TestShareScope_GetDAGHistory(t *testing.T) {
	app := newShareScopeTestApp(t)
	dagId := seedShareScopeDAG(t, app)
	otherDagId := seedShareScopeDAG(t, app)

	t.Run("share token for this diagram allowed", func(t *testing.T) {
		tok := seedShareScopeShare(t, app, dagId, "view")
		if code := doRequest(app, http.MethodGet, "/dag/"+dagId+"/history", tok); code != http.StatusOK {
			t.Fatalf("status = %d, want 200", code)
		}
	})

	t.Run("share token for a different diagram rejected", func(t *testing.T) {
		tok := seedShareScopeShare(t, app, otherDagId, "view")
		if code := doRequest(app, http.MethodGet, "/dag/"+dagId+"/history", tok); code != http.StatusForbidden {
			t.Fatalf("status = %d, want 403 (cross-diagram share must be rejected)", code)
		}
	})

	t.Run("session token allowed", func(t *testing.T) {
		if code := doRequest(app, http.MethodGet, "/dag/"+dagId+"/history", shareScopeSessionToken(t)); code != http.StatusOK {
			t.Fatalf("status = %d, want 200", code)
		}
	})
}

// TestShareScope_DagWS covers GET /dag/:dag/ws. The handler upgrades to a
// WebSocket, which httptest.NewRecorder cannot do, so a request that clears
// the auth/binding gate will fail the upgrade with 400 (missing WS
// handshake headers) rather than reaching 101 — the middleware chain runs
// before wsUpgrader.Upgrade is invoked, so 400 here proves the request
// passed layer 1 + layer 2, and 403 proves it was rejected by them.
func TestShareScope_DagWS(t *testing.T) {
	app := newShareScopeTestApp(t)
	dagId := seedShareScopeDAG(t, app)
	otherDagId := seedShareScopeDAG(t, app)

	t.Run("share token for this diagram passes auth (reaches WS upgrade)", func(t *testing.T) {
		tok := seedShareScopeShare(t, app, dagId, "view")
		if code := doRequest(app, http.MethodGet, "/dag/"+dagId+"/ws", tok); code == http.StatusForbidden {
			t.Fatalf("status = %d, want non-403 (share token for this diagram must pass the auth/binding gate)", code)
		}
	})

	t.Run("share token for a different diagram rejected", func(t *testing.T) {
		tok := seedShareScopeShare(t, app, otherDagId, "view")
		if code := doRequest(app, http.MethodGet, "/dag/"+dagId+"/ws", tok); code != http.StatusForbidden {
			t.Fatalf("status = %d, want 403 (cross-diagram share must be rejected)", code)
		}
	})

	t.Run("session token passes auth (reaches WS upgrade)", func(t *testing.T) {
		if code := doRequest(app, http.MethodGet, "/dag/"+dagId+"/ws", shareScopeSessionToken(t)); code == http.StatusForbidden {
			t.Fatalf("status = %d, want non-403 (session token must pass the auth/binding gate)", code)
		}
	})
}

func TestShareScope_GetMenus(t *testing.T) {
	app := newShareScopeTestApp(t)
	dagId := seedShareScopeDAG(t, app)

	t.Run("share token allowed (no :dag param, layer 2 is a no-op)", func(t *testing.T) {
		tok := seedShareScopeShare(t, app, dagId, "view")
		if code := doRequest(app, http.MethodGet, "/menus", tok); code != http.StatusOK {
			t.Fatalf("status = %d, want 200", code)
		}
	})

	t.Run("session token allowed", func(t *testing.T) {
		if code := doRequest(app, http.MethodGet, "/menus", shareScopeSessionToken(t)); code != http.StatusOK {
			t.Fatalf("status = %d, want 200", code)
		}
	})
}

// ─── Sample of the rejected surface ────────────────────────────────────────

func TestShareScope_RejectedSurface(t *testing.T) {
	app := newShareScopeTestApp(t)
	dagId := seedShareScopeDAG(t, app)
	shareTok := seedShareScopeShare(t, app, dagId, "edit")
	sessionTok := shareScopeSessionToken(t)

	cases := []struct {
		name   string
		method string
		path   string
	}{
		{"GET /dags", http.MethodGet, "/dags"},
		{"GET /companies", http.MethodGet, "/companies"},
		{"GET /shares/inbox", http.MethodGet, "/shares/inbox"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if code := doRequest(app, tc.method, tc.path, shareTok); code != http.StatusUnauthorized {
				t.Errorf("share token: status = %d, want 401 (%s must reject share tokens outright)", code, tc.name)
			}
			if code := doRequest(app, tc.method, tc.path, sessionTok); code != http.StatusOK {
				t.Errorf("session token: status = %d, want 200 (%s must still allow ordinary session tokens)", code, tc.name)
			}
		})
	}
}

// ─── Revocation ────────────────────────────────────────────────────────────
//
// Revoking a share writes to share_denylist but does not delete the shares
// row, so GetShareByJti still resolves and the binding check still passes.
// Before the denylist check moved into ShareResourceBinding, only dagWS
// consulted it — leaving GET /dag/:dag, GET /dag/:dag/history and
// POST /dag/:dag/update honoring a revoked link until the JWT's own exp,
// up to ExpDays (7) later. These tests pin the chokepoint.

func TestShareScope_RevokedShareRejected(t *testing.T) {
	app := newShareScopeTestApp(t)
	dagId := seedShareScopeDAG(t, app)

	body := `{"diagram":"{\"nodes\":[],\"edges\":[]}"}`

	cases := []struct {
		name   string
		method string
		path   string
		body   string
		role   string
	}{
		{"getDAG", http.MethodGet, "/dag/" + dagId, "", "view"},
		{"getDAGHistory", http.MethodGet, "/dag/" + dagId + "/history", "", "view"},
		{"updateDAG", http.MethodPost, "/dag/" + dagId + "/update", body, "edit"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tok := seedShareScopeShare(t, app, dagId, tc.role)

			// Sanity: the link works before revocation, so a 403 after it
			// can only be the revocation and not some unrelated rejection.
			if code := doRequestBody(app, tc.method, tc.path, tok, tc.body); code == http.StatusForbidden {
				t.Fatalf("share was already rejected before revocation; test proves nothing")
			}

			if err := app.db.RevokeShare(shareJti(t, tok)); err != nil {
				t.Fatalf("revoke: %v", err)
			}

			if code := doRequestBody(app, tc.method, tc.path, tok, tc.body); code != http.StatusForbidden {
				t.Fatalf("status = %d, want 403 (revoked share must not keep working)", code)
			}
		})
	}
}

func TestShareScope_RevokedShareDoesNotAffectSessionToken(t *testing.T) {
	app := newShareScopeTestApp(t)
	dagId := seedShareScopeDAG(t, app)

	tok := seedShareScopeShare(t, app, dagId, "view")
	if err := app.db.RevokeShare(shareJti(t, tok)); err != nil {
		t.Fatalf("revoke: %v", err)
	}

	// The denylist is keyed by jti, and a session token's jti is the
	// username. Revoking a share must never be able to lock out a user
	// whose username happens to collide with a share uuid, and more
	// importantly the check must be skipped entirely for non-share tokens.
	if code := doRequest(app, http.MethodGet, "/dag/"+dagId, shareScopeSessionToken(t)); code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (session tokens are not subject to the share denylist)", code)
	}
}

// shareJti pulls the jti claim back out of a minted share token so a test
// can revoke exactly the share it was handed.
func shareJti(t *testing.T, raw string) string {
	t.Helper()
	parsed, err := jwt.Parse(raw, func(*jwt.Token) (interface{}, error) {
		return []byte(testSigningKey), nil
	})
	if err != nil {
		t.Fatalf("parse share token: %v", err)
	}
	claims, ok := parsed.Claims.(jwt.MapClaims)
	if !ok {
		t.Fatalf("claims are not MapClaims")
	}
	jti, _ := claims["jti"].(string)
	if jti == "" {
		t.Fatalf("share token carries no jti")
	}
	return jti
}

// doRequestBody is doRequest with a JSON body, for the POST routes.
func doRequestBody(app *shareScopeTestApp, method, path, raw, body string) int {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if body != "" {
		req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	}
	if raw != "" {
		req.Header.Set(echo.HeaderAuthorization, "Bearer "+raw)
	}
	rec := httptest.NewRecorder()
	app.echo.ServeHTTP(rec, req)
	return rec.Code
}

// GET /menus carries no :dag parameter, so the binding check is skipped for
// it. Revocation is not: a revoked link must reach nothing, which is why the
// denylist check sits above the parameter-less early return rather than
// below it with the binding logic.
func TestShareScope_RevokedShareRejectedOnMenus(t *testing.T) {
	app := newShareScopeTestApp(t)
	dagId := seedShareScopeDAG(t, app)
	tok := seedShareScopeShare(t, app, dagId, "view")

	if code := doRequest(app, http.MethodGet, "/menus", tok); code != http.StatusOK {
		t.Fatalf("status = %d, want 200 before revocation", code)
	}

	if err := app.db.RevokeShare(shareJti(t, tok)); err != nil {
		t.Fatalf("revoke: %v", err)
	}

	if code := doRequest(app, http.MethodGet, "/menus", tok); code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403 (a revoked share must reach nothing, not even /menus)", code)
	}
}

// A well-formed share JWT whose jti has no shares row is an authorization
// failure (403), and must stay distinguishable from the database being
// unreachable (500) — collapsing the two made a Postgres blip present as a
// permanent denial.
func TestShareScope_UnknownJtiIsForbiddenNotServerError(t *testing.T) {
	app := newShareScopeTestApp(t)
	dagId := seedShareScopeDAG(t, app)

	raw, err := token.CreateToken(testSigningKey, jwt.MapClaims{
		"jti":    uuid.New().String(), // valid signature, no shares row
		"iss":    "d3d-share",
		"dag_id": dagId,
		"role":   "view",
		"exp":    time.Now().Add(time.Hour).Unix(),
	})
	if err != nil {
		t.Fatalf("CreateToken: %v", err)
	}

	if code := doRequest(app, http.MethodGet, "/dag/"+dagId, raw); code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403 for a share token with no backing row", code)
	}
}

// A controller constructed without ShareMiddleware used to register the five
// share-accessible routes with zero middleware — variadic expansion of a nil
// slice — publishing them unauthenticated. That is the one failure mode a
// deny-list cannot catch, because no token is checked at all. Init must
// refuse instead.
func TestInitPanicsWhenShareMiddlewareUnset(t *testing.T) {
	for _, tc := range []struct {
		name string
		init func()
	}{
		{"DAGsController", func() { (&DAGsController{Echo: echo.New()}).Init() }},
		{"MenuController", func() { (&MenuController{Echo: echo.New()}).Init() }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			defer func() {
				if recover() == nil {
					t.Fatalf("Init() did not panic with ShareMiddleware unset; share routes would be served unauthenticated")
				}
			}()
			tc.init()
		})
	}
}
