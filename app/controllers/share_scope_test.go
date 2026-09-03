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
