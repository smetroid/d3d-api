package config

import (
	"os"
	"path/filepath"
	"testing"
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
