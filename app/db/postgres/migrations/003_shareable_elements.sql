-- +goose Up

CREATE TABLE IF NOT EXISTS companies (
    id         uuid        PRIMARY KEY,
    name       text        NOT NULL,
    created_by text        NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL
);

-- user_id is text (not a FK to users) so LDAP users without a DB record
-- can also be members.
CREATE TABLE IF NOT EXISTS memberships (
    user_id    text NOT NULL,
    company_id uuid NOT NULL REFERENCES companies (id) ON DELETE CASCADE,
    PRIMARY KEY (user_id, company_id)
);

CREATE INDEX IF NOT EXISTS memberships_user_id_idx    ON memberships (user_id);
CREATE INDEX IF NOT EXISTS memberships_company_id_idx ON memberships (company_id);

-- Avoid the PG11+ reserved word GROUPS by naming the table user_groups.
-- external_ref stores the LDAP/AD group DN when the group was derived from
-- an external auth provider, enabling upsert-on-login for hybrid membership.
CREATE TABLE IF NOT EXISTS user_groups (
    id           uuid        PRIMARY KEY,
    name         text        NOT NULL,
    company_id   uuid        NOT NULL REFERENCES companies (id) ON DELETE CASCADE,
    external_ref text,
    created_at   timestamptz NOT NULL
);

CREATE INDEX IF NOT EXISTS user_groups_company_id_idx ON user_groups (company_id);
-- Prevent duplicate external groups per company (NULLs are excluded from
-- unique indexes so native groups with external_ref IS NULL don't conflict).
CREATE UNIQUE INDEX IF NOT EXISTS user_groups_company_external_ref_idx
    ON user_groups (company_id, external_ref)
    WHERE external_ref IS NOT NULL;

CREATE TABLE IF NOT EXISTS group_members (
    group_id uuid NOT NULL REFERENCES user_groups (id) ON DELETE CASCADE,
    user_id  text NOT NULL,
    PRIMARY KEY (group_id, user_id)
);

CREATE INDEX IF NOT EXISTS group_members_user_id_idx ON group_members (user_id);

-- cluster stores a graphlib-format subgraph snapshot: {nodes:[…], edges:[…]}.
-- jti is set only for public link shares and is indexed via UNIQUE to support
-- denylist lookups; multiple NULLs are permitted by the UNIQUE constraint.
CREATE TABLE IF NOT EXISTS element_shares (
    id            uuid        PRIMARY KEY,
    type          text        NOT NULL DEFAULT 'node',
    root_ids      text[]      NOT NULL DEFAULT '{}',
    cluster       jsonb       NOT NULL,
    audience_kind text        NOT NULL DEFAULT 'public',
    audience_ids  text[]      NOT NULL DEFAULT '{}',
    role          text        NOT NULL DEFAULT 'view',
    created_by    text        NOT NULL DEFAULT '',
    source_dag_id uuid        REFERENCES dags (id) ON DELETE SET NULL,
    expires_at    timestamptz,
    revoked       boolean     NOT NULL DEFAULT FALSE,
    catalog       boolean     NOT NULL DEFAULT FALSE,
    tags          text[]      NOT NULL DEFAULT '{}',
    imported_by   text[]      NOT NULL DEFAULT '{}',
    jti           uuid        UNIQUE,
    anon_name     text        NOT NULL DEFAULT '',
    created_at    timestamptz NOT NULL
);

CREATE INDEX IF NOT EXISTS element_shares_created_by_idx    ON element_shares (created_by);
CREATE INDEX IF NOT EXISTS element_shares_source_dag_id_idx ON element_shares (source_dag_id);
CREATE INDEX IF NOT EXISTS element_shares_catalog_idx
    ON element_shares (catalog)
    WHERE catalog = TRUE AND revoked = FALSE;
CREATE INDEX IF NOT EXISTS element_shares_audience_kind_idx ON element_shares (audience_kind);

-- +goose Down
DROP TABLE IF EXISTS element_shares;
DROP TABLE IF EXISTS group_members;
DROP TABLE IF EXISTS user_groups;
DROP TABLE IF EXISTS memberships;
DROP TABLE IF EXISTS companies;
