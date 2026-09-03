package controllers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	jwt "github.com/dgrijalva/jwt-go"
	"github.com/google/uuid"
	"github.com/labstack/echo"
	"github.com/smetroid/d3d-api/app/auth/token"
	"github.com/smetroid/d3d-api/app/db/postgres"
	"github.com/smetroid/d3d-api/app/models"
	"github.com/smetroid/d3d-api/app/services"
)

// newDAGDBController opens the repository on TEST_DATABASE_URL and clears the
// dags table so each DB-backed test starts clean. It follows the same shape
// as newTestController in element_shares_test.go.
func newDAGDBController(t *testing.T) *DAGsController {
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
	return &DAGsController{DB: p, DAGService: services.DAGService{DB: p}}
}

// seedDAG inserts a minimal DAG directly via the service and returns its id.
func seedDAG(t *testing.T, dc *DAGsController) string {
	t.Helper()
	id, err := dc.DAGService.ProcessDAG(models.Dag{
		Name:    "seed",
		Diagram: `{"nodes":[],"edges":[]}`,
	})
	if err != nil {
		t.Fatalf("seed dag: %v", err)
	}
	return id
}

// shareToken mints a d3d-share token with the given role, per the claim
// shape used in app/controllers/shares.go:68-73.
func shareToken(t *testing.T, role string) string {
	t.Helper()
	raw, err := token.CreateToken(testSigningKey, jwt.MapClaims{
		"jti":    uuid.New().String(),
		"iss":    "d3d-share",
		"dag_id": "irrelevant",
		"role":   role,
		"exp":    time.Now().Add(time.Hour).Unix(),
	})
	if err != nil {
		t.Fatalf("CreateToken: %v", err)
	}
	return raw
}

// sessionToken mints a normal (non-share) session token, per
// token.CreateExpiringToken's claim shape.
func sessionToken(t *testing.T) string {
	t.Helper()
	return token.CreateExpiringToken("alice", testSigningKey, time.Hour, "localauth")
}

// dagCtx builds an echo.Context carrying raw as the parsed "user" token
// (mirroring what the JWT auth middleware installs) with :dag set to dagId.
func dagCtx(t *testing.T, method, path, dagId, raw string) (echo.Context, *httptest.ResponseRecorder) {
	t.Helper()
	e := echo.New()
	req := httptest.NewRequest(method, path, nil)
	rec := httptest.NewRecorder()
	ctx := authedContext(t, e, req, rec, raw)
	ctx.SetParamNames("dag")
	ctx.SetParamValues(dagId)
	return ctx, rec
}

func TestDeleteDAG_ViewShareRejected(t *testing.T) {
	dc := newDAGDBController(t)
	dagId := seedDAG(t, dc)

	ctx, rec := dagCtx(t, http.MethodDelete, "/dag/"+dagId, dagId, shareToken(t, "view"))
	if err := dc.deleteDAG(ctx); err != nil {
		t.Fatalf("handler: %v", err)
	}
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusForbidden, rec.Body.String())
	}

	// The DAG must still exist — the delete must not have gone through.
	if _, err := dc.DAGService.GetDAG(dagId); err != nil {
		t.Errorf("GetDAG after rejected delete: %v", err)
	}
}

func TestDeleteDAG_EditShareAllowed(t *testing.T) {
	dc := newDAGDBController(t)
	dagId := seedDAG(t, dc)

	ctx, rec := dagCtx(t, http.MethodDelete, "/dag/"+dagId, dagId, shareToken(t, "edit"))
	if err := dc.deleteDAG(ctx); err != nil {
		t.Fatalf("handler: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}
}

func TestDeleteDAG_SessionTokenAllowed(t *testing.T) {
	dc := newDAGDBController(t)
	dagId := seedDAG(t, dc)

	ctx, rec := dagCtx(t, http.MethodDelete, "/dag/"+dagId, dagId, sessionToken(t))
	if err := dc.deleteDAG(ctx); err != nil {
		t.Fatalf("handler: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}
}

func TestRestoreDAGHistory_ViewShareRejected(t *testing.T) {
	dc := newDAGDBController(t)
	dagId := seedDAG(t, dc)
	if err := dc.DAGService.AppendHistory(dagId, `{"nodes":[],"edges":[]}`, "alice"); err != nil {
		t.Fatalf("AppendHistory: %v", err)
	}
	hist, err := dc.DAGService.GetHistory(dagId)
	if err != nil || len(hist.History) == 0 {
		t.Fatalf("GetHistory: %v, %+v", err, hist)
	}
	historyId := hist.History[0].Id

	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/dag/"+dagId+"/history/"+historyId+"/restore", nil)
	rec := httptest.NewRecorder()
	ctx := authedContext(t, e, req, rec, shareToken(t, "view"))
	ctx.SetParamNames("dag", "historyId")
	ctx.SetParamValues(dagId, historyId)

	if err := dc.restoreDAGHistory(ctx); err != nil {
		t.Fatalf("handler: %v", err)
	}
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusForbidden, rec.Body.String())
	}
}

func TestRestoreDAGHistory_EditShareAllowed(t *testing.T) {
	dc := newDAGDBController(t)
	dagId := seedDAG(t, dc)
	if err := dc.DAGService.AppendHistory(dagId, `{"nodes":[],"edges":[]}`, "alice"); err != nil {
		t.Fatalf("AppendHistory: %v", err)
	}
	hist, err := dc.DAGService.GetHistory(dagId)
	if err != nil || len(hist.History) == 0 {
		t.Fatalf("GetHistory: %v, %+v", err, hist)
	}
	historyId := hist.History[0].Id

	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/dag/"+dagId+"/history/"+historyId+"/restore", nil)
	rec := httptest.NewRecorder()
	ctx := authedContext(t, e, req, rec, shareToken(t, "edit"))
	ctx.SetParamNames("dag", "historyId")
	ctx.SetParamValues(dagId, historyId)

	if err := dc.restoreDAGHistory(ctx); err != nil {
		t.Fatalf("handler: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}
}

func TestRestoreDAGHistory_SessionTokenAllowed(t *testing.T) {
	dc := newDAGDBController(t)
	dagId := seedDAG(t, dc)
	if err := dc.DAGService.AppendHistory(dagId, `{"nodes":[],"edges":[]}`, "alice"); err != nil {
		t.Fatalf("AppendHistory: %v", err)
	}
	hist, err := dc.DAGService.GetHistory(dagId)
	if err != nil || len(hist.History) == 0 {
		t.Fatalf("GetHistory: %v, %+v", err, hist)
	}
	historyId := hist.History[0].Id

	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/dag/"+dagId+"/history/"+historyId+"/restore", nil)
	rec := httptest.NewRecorder()
	ctx := authedContext(t, e, req, rec, sessionToken(t))
	ctx.SetParamNames("dag", "historyId")
	ctx.SetParamValues(dagId, historyId)

	if err := dc.restoreDAGHistory(ctx); err != nil {
		t.Fatalf("handler: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}
}
