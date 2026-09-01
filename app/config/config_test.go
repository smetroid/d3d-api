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

func TestEnvOverridesBeatTomlValues(t *testing.T) {
	body := `
[samus]
frontend_origin = "http://localhost:5173"
cookie_secure = false

[google]
client_id = "toml-google-id"
client_secret = "toml-google-secret"

[github]
client_id = "toml-github-id"
client_secret = "toml-github-secret"
`
	path := filepath.Join(t.TempDir(), "c.toml")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	t.Setenv("D3D_GOOGLE_CLIENT_SECRET", "env-google-secret")
	t.Setenv("D3D_GITHUB_CLIENT_ID", "env-github-id")
	t.Setenv("D3D_FRONTEND_ORIGIN", "https://d3dweb.vercel.app")
	t.Setenv("D3D_COOKIE_SECURE", "true")

	cfg := BuildConfig(path)

	if cfg.Google.ClientSecret != "env-google-secret" {
		t.Errorf("Google.ClientSecret = %q, want the env value", cfg.Google.ClientSecret)
	}
	// Unset vars must leave the TOML value alone.
	if cfg.Google.ClientID != "toml-google-id" {
		t.Errorf("Google.ClientID = %q, want the toml value", cfg.Google.ClientID)
	}
	if cfg.GitHub.ClientID != "env-github-id" {
		t.Errorf("GitHub.ClientID = %q, want the env value", cfg.GitHub.ClientID)
	}
	if cfg.Samus.FrontendOrigin != "https://d3dweb.vercel.app" {
		t.Errorf("FrontendOrigin = %q", cfg.Samus.FrontendOrigin)
	}
	if !cfg.Samus.CookieSecure {
		t.Error("CookieSecure = false, want true from D3D_COOKIE_SECURE")
	}
}

func TestEmptyEnvVarDoesNotClobberToml(t *testing.T) {
	body := "[google]\nclient_id = \"toml-google-id\"\n"
	path := filepath.Join(t.TempDir(), "c.toml")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	// An empty variable is not a value. Treating "" as an override would wipe
	// working config on any host that exports the name blank.
	t.Setenv("D3D_GOOGLE_CLIENT_ID", "")

	if got := BuildConfig(path).Google.ClientID; got != "toml-google-id" {
		t.Errorf("Google.ClientID = %q, want the toml value preserved", got)
	}
}

func TestSigningKeyEnvOverride(t *testing.T) {
	body := "[samus]\nsigning_key = \"toml-key\"\n"
	path := filepath.Join(t.TempDir(), "c.toml")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	t.Setenv("D3D_SIGNING_KEY", "env-key")
	if got := BuildConfig(path).Samus.SigningKey; got != "env-key" {
		t.Errorf("SigningKey = %q, want the env value", got)
	}
}

// A blank variable must not empty the signing key — app.go:211 log.Fatals on
// an empty key, so clobbering it here would stop the service from booting.
func TestBlankSigningKeyEnvDoesNotClobberToml(t *testing.T) {
	body := "[samus]\nsigning_key = \"toml-key\"\n"
	path := filepath.Join(t.TempDir(), "c.toml")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	t.Setenv("D3D_SIGNING_KEY", "")
	if got := BuildConfig(path).Samus.SigningKey; got != "toml-key" {
		t.Errorf("SigningKey = %q, want the toml value preserved", got)
	}
}

// I2 regression: cookie_secure must fail closed. Omitting the key from the
// TOML file entirely (the common way to "forget" it in production) must
// still produce a Secure session cookie.
func TestCookieSecureDefaultsToTrueWhenAbsent(t *testing.T) {
	body := "[samus]\nfrontend_origin = \"http://localhost:5173\"\n"
	path := filepath.Join(t.TempDir(), "c.toml")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	if got := BuildConfig(path).Samus.CookieSecure; !got {
		t.Error("CookieSecure = false, want true (secure-by-default) when the key is absent from the TOML file")
	}
}

// An explicit `cookie_secure = false` (as used for plain-HTTP local dev) must
// still be honored through BuildConfig, not just direct toml.Decode.
func TestCookieSecureExplicitFalseIsHonoured(t *testing.T) {
	body := "[samus]\nfrontend_origin = \"http://localhost:5173\"\ncookie_secure = false\n"
	path := filepath.Join(t.TempDir(), "c.toml")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	if got := BuildConfig(path).Samus.CookieSecure; got {
		t.Error("CookieSecure = true, want false (explicit dev override) to be honored")
	}
}
