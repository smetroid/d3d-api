package postgres

import (
	"context"
	"fmt"
	"strings"
	"testing"
)

// gooseUpSQL returns the "+goose Up" section of an embedded migration so tests
// can apply it to a schema goose itself has already marked as migrated.
func gooseUpSQL(name string) (string, error) {
	raw, err := embedMigrations.ReadFile("migrations/" + name)
	if err != nil {
		return "", err
	}
	body := string(raw)
	_, after, ok := strings.Cut(body, "-- +goose Up")
	if !ok {
		return "", fmt.Errorf("%s: no +goose Up section", name)
	}
	up, _, _ := strings.Cut(after, "-- +goose Down")
	return up, nil
}

// legacyElementSharesDDL recreates the pre-003 shape of element_shares found on
// databases where the table already existed when 003 ran: because 003 uses
// CREATE TABLE IF NOT EXISTS, goose marked it applied without adding the
// columns the repository writes (type, role, revoked, catalog, tags,
// anon_name), so CreateElementShare failed with
// `column "type" of relation "element_shares" does not exist`.
const legacyElementSharesDDL = `
CREATE TABLE element_shares (
    id            uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    source_dag_id uuid,
    root_ids      text[]      NOT NULL,
    cluster       jsonb       NOT NULL,
    audience_kind text        NOT NULL,
    audience_ids  text[]      NOT NULL DEFAULT '{}',
    jti           uuid        UNIQUE,
    created_by    text        NOT NULL,
    imported_by   text[]      NOT NULL DEFAULT '{}',
    created_at    timestamptz NOT NULL DEFAULT now(),
    expires_at    timestamptz,
    title         text        NOT NULL DEFAULT ''
);

CREATE TABLE user_groups (
    id           uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    company_id   uuid        NOT NULL,
    name         text        NOT NULL,
    external_ref text,
    created_by   text        NOT NULL,
    created_at   timestamptz NOT NULL DEFAULT now(),
    UNIQUE (company_id, name)
);

CREATE TABLE memberships (
    company_id uuid        NOT NULL,
    user_id    text        NOT NULL,
    joined_at  timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (company_id, user_id)
);

CREATE TABLE group_members (
    group_id  uuid        NOT NULL,
    user_id   text        NOT NULL,
    joined_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (group_id, user_id)
);
`

// TestMigration_ReconcilesLegacyElementShares applies the reconciliation
// migration to a legacy-shaped schema and asserts the repository's writes
// then succeed.
func TestMigration_ReconcilesLegacyElementShares(t *testing.T) {
	p := newTestPostgres(t)
	ctx := context.Background()

	// Work in a throwaway schema so the real tables are untouched.
	if _, err := p.Pool().Exec(ctx, `DROP SCHEMA IF EXISTS drift_test CASCADE; CREATE SCHEMA drift_test`); err != nil {
		t.Fatalf("create schema: %v", err)
	}
	t.Cleanup(func() {
		_, _ = p.Pool().Exec(ctx, `DROP SCHEMA IF EXISTS drift_test CASCADE`)
	})

	conn, err := p.Pool().Acquire(ctx)
	if err != nil {
		t.Fatalf("acquire: %v", err)
	}
	defer conn.Release()
	if _, err := conn.Exec(ctx, `SET search_path TO drift_test`); err != nil {
		t.Fatalf("set search_path: %v", err)
	}
	if _, err := conn.Exec(ctx, legacyElementSharesDDL); err != nil {
		t.Fatalf("legacy ddl: %v", err)
	}

	up, err := gooseUpSQL("005_reconcile_element_shares.sql")
	if err != nil {
		t.Fatalf("read migration: %v", err)
	}
	if _, err := conn.Exec(ctx, up); err != nil {
		t.Fatalf("apply migration: %v", err)
	}

	// The exact column list CreateElementShare writes.
	if _, err := conn.Exec(ctx, `
		INSERT INTO element_shares (
			id, type, root_ids, cluster, audience_kind, audience_ids,
			role, created_by, source_dag_id, expires_at, revoked,
			catalog, tags, imported_by, jti, anon_name, title, created_at
		) VALUES (
			gen_random_uuid(), 'node', '{n1}', '{}'::jsonb, 'public', '{}',
			'view', 'alice', NULL, now(), false,
			false, '{}', '{}', NULL, 'anon', 'demo', now()
		)`); err != nil {
		t.Fatalf("insert after migration: %v", err)
	}

	// CreateGroup omits created_by, and UpsertGroup needs the partial unique
	// index on (company_id, external_ref) for its ON CONFLICT target.
	if _, err := conn.Exec(ctx, `
		INSERT INTO user_groups (id, name, company_id, external_ref, created_at)
		VALUES (gen_random_uuid(), 'eng', gen_random_uuid(), 'cn=eng', now())
		ON CONFLICT (company_id, external_ref) WHERE external_ref IS NOT NULL
		DO UPDATE SET name = EXCLUDED.name`); err != nil {
		t.Fatalf("upsert group after migration: %v", err)
	}
}
