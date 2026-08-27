-- +goose Up

-- 003 creates its tables with CREATE TABLE IF NOT EXISTS. On databases where
-- element_shares and the membership tables already existed, those statements
-- were skipped while goose still recorded 003 as applied, so the tables kept
-- their older shape: element_shares without type/role/revoked/catalog/tags/
-- anon_name (every CreateElementShare then failed with `column "type" of
-- relation "element_shares" does not exist`), and several indexes missing.
-- Every statement below is a no-op on a schema actually built by 003.

ALTER TABLE element_shares ADD COLUMN IF NOT EXISTS type      text    NOT NULL DEFAULT 'node';
ALTER TABLE element_shares ADD COLUMN IF NOT EXISTS role      text    NOT NULL DEFAULT 'view';
ALTER TABLE element_shares ADD COLUMN IF NOT EXISTS revoked   boolean NOT NULL DEFAULT FALSE;
ALTER TABLE element_shares ADD COLUMN IF NOT EXISTS catalog   boolean NOT NULL DEFAULT FALSE;
ALTER TABLE element_shares ADD COLUMN IF NOT EXISTS tags      text[]  NOT NULL DEFAULT '{}';
ALTER TABLE element_shares ADD COLUMN IF NOT EXISTS anon_name text    NOT NULL DEFAULT '';

CREATE INDEX IF NOT EXISTS element_shares_created_by_idx    ON element_shares (created_by);
CREATE INDEX IF NOT EXISTS element_shares_source_dag_id_idx ON element_shares (source_dag_id);
CREATE INDEX IF NOT EXISTS element_shares_catalog_idx
    ON element_shares (catalog)
    WHERE catalog = TRUE AND revoked = FALSE;
CREATE INDEX IF NOT EXISTS element_shares_audience_kind_idx ON element_shares (audience_kind);

-- CreateGroup does not write created_by, so the legacy NOT NULL column without
-- a default rejects every insert. The column does not exist on a 003 schema.
-- +goose StatementBegin
DO $$
BEGIN
    IF EXISTS (
        SELECT 1 FROM information_schema.columns
        WHERE table_schema = current_schema()
          AND table_name   = 'user_groups'
          AND column_name  = 'created_by'
    ) THEN
        ALTER TABLE user_groups ALTER COLUMN created_by SET DEFAULT '';
    END IF;
END $$;
-- +goose StatementEnd

-- UpsertGroup uses ON CONFLICT (company_id, external_ref) WHERE external_ref
-- IS NOT NULL, which needs this partial unique index as its arbiter.
CREATE UNIQUE INDEX IF NOT EXISTS user_groups_company_external_ref_idx
    ON user_groups (company_id, external_ref)
    WHERE external_ref IS NOT NULL;

CREATE INDEX IF NOT EXISTS user_groups_company_id_idx    ON user_groups (company_id);
CREATE INDEX IF NOT EXISTS memberships_user_id_idx       ON memberships (user_id);
CREATE INDEX IF NOT EXISTS memberships_company_id_idx    ON memberships (company_id);
CREATE INDEX IF NOT EXISTS group_members_user_id_idx     ON group_members (user_id);

-- +goose Down

-- The columns are dropped only in Down; on a 003-built schema this rolls back
-- more than Up added, which is why Down is not used outside development.
DROP INDEX IF EXISTS user_groups_company_external_ref_idx;
ALTER TABLE element_shares DROP COLUMN IF EXISTS anon_name;
ALTER TABLE element_shares DROP COLUMN IF EXISTS tags;
ALTER TABLE element_shares DROP COLUMN IF EXISTS catalog;
ALTER TABLE element_shares DROP COLUMN IF EXISTS revoked;
ALTER TABLE element_shares DROP COLUMN IF EXISTS role;
ALTER TABLE element_shares DROP COLUMN IF EXISTS type;
