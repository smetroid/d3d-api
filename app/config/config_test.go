package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/BurntSushi/toml"
)

func writeTempConfig(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "samus.toml")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestBuildConfigPostgresSection(t *testing.T) {
	path := writeTempConfig(t, `
[postgres]
    dsn = "postgres://u:p@db:5432/samus?sslmode=require"
`)

	cfg := BuildConfig(path)
	if got := cfg.Postgres.EffectiveDSN(); got != "postgres://u:p@db:5432/samus?sslmode=require" {
		t.Errorf("[postgres] dsn: EffectiveDSN() = %q", got)
	}
}

func TestBuildConfigPostgresqlAliasSection(t *testing.T) {
	path := writeTempConfig(t, `
[postgresql]
    address = "release-postgresql:5432"
    user = "samus"
    password = "dev-password-change-in-prod"
    database = "samus"
`)

	cfg := BuildConfig(path)
	want := "postgres://samus:dev-password-change-in-prod@release-postgresql:5432/samus"
	if got := cfg.Postgres.EffectiveDSN(); got != want {
		t.Errorf("[postgresql] alias: EffectiveDSN() = %q, want %q", got, want)
	}
}

func TestBuildConfigPostgresWinsOverAlias(t *testing.T) {
	path := writeTempConfig(t, `
[postgres]
    dsn = "postgres://explicit@db:5432/samus"

[postgresql]
    address = "ignored:5432"
    user = "ignored"
    password = "ignored"
    database = "ignored"
`)

	cfg := BuildConfig(path)
	want := "postgres://explicit@db:5432/samus"
	if got := cfg.Postgres.EffectiveDSN(); got != want {
		t.Errorf("EffectiveDSN() = %q, want %q", got, want)
	}
}

func TestDecodesSocialProviderBlocks(t *testing.T) {
	body := `
[samus]
frontend_origin = "http://localhost:5173"
cookie_secure = false

[google]
client_id = "g-id"
client_secret = "g-secret"
redirect_url = "http://localhost:5173/auth/callback"

[github]
client_id = "gh-id"
client_secret = "gh-secret"
redirect_url = "http://localhost:5173/auth/callback"
`
	path := filepath.Join(t.TempDir(), "c.toml")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	var cfg SamusConfig
	if _, err := toml.DecodeFile(path, &cfg); err != nil {
		t.Fatalf("decode: %v", err)
	}

	if cfg.Samus.FrontendOrigin != "http://localhost:5173" {
		t.Errorf("FrontendOrigin = %q", cfg.Samus.FrontendOrigin)
	}
	if cfg.Samus.CookieSecure {
		t.Error("CookieSecure = true, want false")
	}
	if cfg.Google.ClientID != "g-id" || cfg.Google.ClientSecret != "g-secret" {
		t.Errorf("Google = %+v", cfg.Google)
	}
	if cfg.GitHub.ClientID != "gh-id" || cfg.GitHub.RedirectURL == "" {
		t.Errorf("GitHub = %+v", cfg.GitHub)
	}
}
