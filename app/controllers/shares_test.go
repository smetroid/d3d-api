package controllers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/labstack/echo"
	"github.com/smetroid/d3d-api/app/db/postgres"
	"github.com/smetroid/d3d-api/app/models"
	"github.com/smetroid/d3d-api/app/services"
)

// newSharesDBController opens the repository on TEST_DATABASE_URL and clears
// the dags table (which cascades to shares, per shares.dag_id's ON DELETE
// CASCADE) so each DB-backed test starts clean. It follows the same shape as
// newDAGDBController in dag_test.go.
func newSharesDBController(t *testing.T) *SharesController {
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
	return &SharesController{DB: p, SigningKey: testSigningKey}
}

// seedShareDAG seeds a DAG for share tests, reusing seedDAG (dag_test.go)
// against sc's own DB connection so both controllers see the same row.
func seedShareDAG(t *testing.T, sc *SharesController) string {
	t.Helper()
	dagDC := &DAGsController{DAGService: services.DAGService{DB: sc.DB}}
	return seedDAG(t, dagDC)
}

// countShares returns the number of share rows for dagId, to prove a
// rejected createShare did not write.
func countShares(t *testing.T, sc *SharesController, dagId string) int {
	t.Helper()
	var n int
	if err := sc.DB.Pool().QueryRow(context.Background(),
		`SELECT COUNT(*) FROM shares WHERE dag_id = $1`, dagId).Scan(&n); err != nil {
		t.Fatalf("count shares: %v", err)
	}
	return n
}

// createShareCtx builds an echo.Context for a POST /dag/:dag/shares request
// with the given JSON body, carrying raw as the parsed "user" token.
func createShareCtx(t *testing.T, dagId, raw, body string) (echo.Context, *httptest.ResponseRecorder) {
	t.Helper()
	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/dag/"+dagId+"/shares", strings.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	ctx := authedContext(t, e, req, rec, raw)
	ctx.SetParamNames("dag")
	ctx.SetParamValues(dagId)
	return ctx, rec
}

// revokeShareCtx builds an echo.Context for a POST
// /dag/:dag/shares/:jti/revoke request, carrying raw as the parsed "user"
// token.
func revokeShareCtx(t *testing.T, dagId, jti, raw string) (echo.Context, *httptest.ResponseRecorder) {
	t.Helper()
	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/dag/"+dagId+"/shares/"+jti+"/revoke", nil)
	rec := httptest.NewRecorder()
	ctx := authedContext(t, e, req, rec, raw)
	ctx.SetParamNames("dag", "jti")
	ctx.SetParamValues(dagId, jti)
	return ctx, rec
}

// seedShare inserts a share row directly via the DB, for revoke tests.
func seedShare(t *testing.T, sc *SharesController, dagId, role string) string {
	t.Helper()
	jti := uuid.New().String()
	if err := sc.DB.CreateShare(models.Share{
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
	return jti
}

func TestCreateShare_ViewShareRejected(t *testing.T) {
	sc := newSharesDBController(t)
	dagId := seedShareDAG(t, sc)

	before := countShares(t, sc, dagId)

	ctx, rec := createShareCtx(t, dagId, shareToken(t, "view"), `{"role":"view"}`)
	if err := sc.createShare(ctx); err != nil {
		t.Fatalf("handler: %v", err)
	}
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusForbidden, rec.Body.String())
	}

	if after := countShares(t, sc, dagId); after != before {
		t.Errorf("share row count changed: before=%d after=%d, want unchanged (rejected create must not write)", before, after)
	}
}

// TestCreateShare_ViewShareEscalationRejected proves the escalation path is
// closed end to end: a view-only share recipient must not be able to mint
// themselves an edit-role token via {"role":"edit"}.
func TestCreateShare_ViewShareEscalationRejected(t *testing.T) {
	sc := newSharesDBController(t)
	dagId := seedShareDAG(t, sc)

	before := countShares(t, sc, dagId)

	ctx, rec := createShareCtx(t, dagId, shareToken(t, "view"), `{"role":"edit"}`)
	if err := sc.createShare(ctx); err != nil {
		t.Fatalf("handler: %v", err)
	}
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d, body = %s — view-only recipient minted an edit-role share token", rec.Code, http.StatusForbidden, rec.Body.String())
	}

	if after := countShares(t, sc, dagId); after != before {
		t.Errorf("share row count changed: before=%d after=%d, want unchanged (rejected create must not write)", before, after)
	}
}

func TestCreateShare_EditShareAllowed(t *testing.T) {
	sc := newSharesDBController(t)
	dagId := seedShareDAG(t, sc)

	ctx, rec := createShareCtx(t, dagId, shareToken(t, "edit"), `{"role":"view"}`)
	if err := sc.createShare(ctx); err != nil {
		t.Fatalf("handler: %v", err)
	}
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusCreated, rec.Body.String())
	}
}

func TestCreateShare_SessionTokenAllowed(t *testing.T) {
	sc := newSharesDBController(t)
	dagId := seedShareDAG(t, sc)

	ctx, rec := createShareCtx(t, dagId, sessionToken(t), `{"role":"view"}`)
	if err := sc.createShare(ctx); err != nil {
		t.Fatalf("handler: %v", err)
	}
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusCreated, rec.Body.String())
	}
}

func TestRevokeShare_ViewShareRejected(t *testing.T) {
	sc := newSharesDBController(t)
	dagId := seedShareDAG(t, sc)
	jti := seedShare(t, sc, dagId, "edit")

	ctx, rec := revokeShareCtx(t, dagId, jti, shareToken(t, "view"))
	if err := sc.revokeShare(ctx); err != nil {
		t.Fatalf("handler: %v", err)
	}
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusForbidden, rec.Body.String())
	}

	revoked, err := sc.DB.IsRevoked(jti)
	if err != nil {
		t.Fatalf("IsRevoked: %v", err)
	}
	if revoked {
		t.Error("share was revoked despite rejected request")
	}
}

func TestRevokeShare_EditShareAllowed(t *testing.T) {
	sc := newSharesDBController(t)
	dagId := seedShareDAG(t, sc)
	jti := seedShare(t, sc, dagId, "view")

	ctx, rec := revokeShareCtx(t, dagId, jti, shareToken(t, "edit"))
	if err := sc.revokeShare(ctx); err != nil {
		t.Fatalf("handler: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}
}

func TestRevokeShare_SessionTokenAllowed(t *testing.T) {
	sc := newSharesDBController(t)
	dagId := seedShareDAG(t, sc)
	jti := seedShare(t, sc, dagId, "view")

	ctx, rec := revokeShareCtx(t, dagId, jti, sessionToken(t))
	if err := sc.revokeShare(ctx); err != nil {
		t.Fatalf("handler: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}
}
