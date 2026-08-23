package localauth_test

import (
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/smetroid/d3d-api/app/auth/localauth"
	"github.com/smetroid/d3d-api/app/db/postgres"
	"github.com/smetroid/d3d-api/app/models"
	"golang.org/x/crypto/bcrypt"
)

const testDSNEnv = "TEST_DATABASE_URL"

func TestLocalAuth_Connect_NilDB(t *testing.T) {
	p := &localauth.LocalAuthProvider{}
	if err := p.Connect(); err == nil {
		t.Error("expected error when DB is nil")
	}
}

func TestLocalAuth_Authenticate(t *testing.T) {
	dsn := os.Getenv(testDSNEnv)
	if dsn == "" {
		t.Skipf("%s not set; skipping integration test", testDSNEnv)
	}

	db := &postgres.Postgres{DSN: dsn}
	if err := db.Init(); err != nil {
		t.Fatalf("db init: %v", err)
	}

	// Unique username so parallel or repeated runs don't collide.
	username := fmt.Sprintf("localauth-test-%d", time.Now().UnixNano())
	hash, err := bcrypt.GenerateFromPassword([]byte("correct-password"), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.CreateUser(models.User{
		Username:     username,
		PasswordHash: string(hash),
		CreatedAt:    time.Now().UTC(),
	}); err != nil {
		t.Fatalf("create user: %v", err)
	}

	provider := &localauth.LocalAuthProvider{DB: db}
	provider.SetSigningKey("test-signing-key")

	t.Run("correct password returns token", func(t *testing.T) {
		ok, token, err := provider.Authenticate(username, "correct-password")
		if err != nil || !ok || token == "" {
			t.Errorf("expected success: ok=%v token=%q err=%v", ok, token, err)
		}
	})

	t.Run("wrong password returns false", func(t *testing.T) {
		ok, _, err := provider.Authenticate(username, "wrong-password")
		if err != nil || ok {
			t.Errorf("expected failure on wrong password: ok=%v err=%v", ok, err)
		}
	})

	t.Run("unknown user returns false", func(t *testing.T) {
		ok, _, err := provider.Authenticate("nobody-"+username, "correct-password")
		if err != nil || ok {
			t.Errorf("expected failure for unknown user: ok=%v err=%v", ok, err)
		}
	})
}
