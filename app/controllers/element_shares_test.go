package controllers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/labstack/echo"
	"github.com/smetroid/d3d-api/app/auth/token"
	"github.com/smetroid/d3d-api/app/db/postgres"
	"github.com/smetroid/d3d-api/app/models"

	jwt "github.com/dgrijalva/jwt-go"
)

const testDSNEnv = "TEST_DATABASE_URL"

const testSigningKey = "test-signing-key"

// newTestController opens the repository on TEST_DATABASE_URL and clears the
// element_shares table so each test starts clean.
func newTestController(t *testing.T) *ElementSharesController {
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

	if _, err := p.Pool().Exec(context.Background(), `TRUNCATE element_shares CASCADE`); err != nil {
		t.Fatalf("truncate: %v", err)
	}
	return &ElementSharesController{DB: p, SigningKey: testSigningKey}
}

// exchange calls the handler directly, bypassing route registration so the
// test does not need the auth middleware.
func exchange(t *testing.T, ec *ElementSharesController, rawToken string) (int, map[string]interface{}) {
	t.Helper()
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/element-shares/exchange?token="+rawToken, nil)
	rec := httptest.NewRecorder()
	if err := ec.exchangeElementShare(e.NewContext(req, rec)); err != nil {
		t.Fatalf("handler: %v", err)
	}
	var body map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode %q: %v", rec.Body.String(), err)
	}
	return rec.Code, body
}

func seedPublicShare(t *testing.T, ec *ElementSharesController, title string) string {
	t.Helper()
	jti := uuid.New().String()
	share := models.ElementShare{
		Id:           uuid.New().String(),
		Title:        title,
		Type:         "node",
		RootIds:      []string{"n1"},
		Cluster:      `{"nodes":[{"v":"n1","value":{}}],"edges":[]}`,
		AudienceKind: "public",
		Role:         "view",
		CreatedBy:    "alice",
		AnonName:     "anon-fox",
		Jti:          jti,
		ExpiresAt:    time.Now().Add(24 * time.Hour),
		CreatedAt:    time.Now(),
	}
	id, err := ec.DB.CreateElementShare(share)
	if err != nil {
		t.Fatalf("CreateElementShare: %v", err)
	}
	raw, err := token.CreateToken(testSigningKey, jwt.MapClaims{
		"jti":      jti,
		"iss":      "d3d-element-share",
		"share_id": id,
		"role":     "view",
		"exp":      share.ExpiresAt.Unix(),
	})
	if err != nil {
		t.Fatalf("CreateToken: %v", err)
	}
	return raw
}

// The preview page renders whatever this endpoint returns, so its field names
// are a contract. Both `title` and `anonName` are read by the client; `id` is
// deliberately absent — the share identifier is `shareId`.
func TestExchangeElementShare_ResponseContract(t *testing.T) {
	ec := newTestController(t)
	raw := seedPublicShare(t, ec, "Auth service cluster")

	code, body := exchange(t, ec, raw)
	if code != http.StatusOK {
		t.Fatalf("status: got %d, want %d (%v)", code, http.StatusOK, body)
	}

	if got := body["title"]; got != "Auth service cluster" {
		t.Errorf("title: got %v, want %q", got, "Auth service cluster")
	}
	if got := body["anonName"]; got != "anon-fox" {
		t.Errorf("anonName: got %v, want %q", got, "anon-fox")
	}
	if body["shareId"] == nil || body["shareId"] == "" {
		t.Errorf("shareId: got %v, want a non-empty id", body["shareId"])
	}
	if got := body["type"]; got != "node" {
		t.Errorf("type: got %v, want %q", got, "node")
	}
	if body["cluster"] == nil {
		t.Error("cluster: got nil, want the subgraph")
	}
	if _, ok := body["createdBy"]; ok {
		t.Error("createdBy leaked; anonName exists to keep the creator anonymous")
	}
}

func TestExchangeElementShare_EmptyTitle(t *testing.T) {
	ec := newTestController(t)
	raw := seedPublicShare(t, ec, "")

	code, body := exchange(t, ec, raw)
	if code != http.StatusOK {
		t.Fatalf("status: got %d, want %d (%v)", code, http.StatusOK, body)
	}
	// Present but empty, so the client applies its own fallback.
	title, ok := body["title"]
	if !ok {
		t.Fatal("title key missing")
	}
	if title != "" {
		t.Errorf("title: got %v, want empty string", title)
	}
}
