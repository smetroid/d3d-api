-- +goose Up
CREATE TABLE dags (
    id          uuid        PRIMARY KEY,
    name        text        NOT NULL DEFAULT '',
    description text        NOT NULL DEFAULT '',
    diagram     jsonb,
    created     timestamptz,
    updated     timestamptz
);

CREATE TABLE dag_history (
    id            uuid        PRIMARY KEY,
    dag_id        uuid        NOT NULL REFERENCES dags (id) ON DELETE CASCADE,
    snapshot_json jsonb,
    saved_by      text        NOT NULL DEFAULT '',
    saved_at      timestamptz NOT NULL
);

CREATE INDEX dag_history_dag_id_saved_at_idx ON dag_history (dag_id, saved_at DESC);

CREATE TABLE shares (
    id         uuid        PRIMARY KEY,
    dag_id     uuid        NOT NULL REFERENCES dags (id) ON DELETE CASCADE,
    jti        uuid        NOT NULL UNIQUE,
    role       text        NOT NULL DEFAULT 'view',
    anon_name  text        NOT NULL DEFAULT '',
    created_by text        NOT NULL DEFAULT '',
    expires_at timestamptz NOT NULL,
    created_at timestamptz NOT NULL
);

CREATE INDEX shares_dag_id_idx ON shares (dag_id);

CREATE TABLE share_denylist (
    jti        uuid        PRIMARY KEY,
    revoked_at timestamptz NOT NULL
);

CREATE TABLE users (
    id            uuid        PRIMARY KEY,
    username      text        NOT NULL UNIQUE,
    password_hash text        NOT NULL,
    created_at    timestamptz NOT NULL
);

CREATE TABLE nodes (
    id                    uuid        PRIMARY KEY,
    v                     text        NOT NULL DEFAULT '',
    parent                text        NOT NULL DEFAULT '',
    value_label           jsonb,
    value_type            text        NOT NULL DEFAULT '',
    value_cluster_label_pos text      NOT NULL DEFAULT '',
    value_style           text        NOT NULL DEFAULT '',
    created               timestamptz 
);

CREATE TABLE edges (
    id      uuid        PRIMARY KEY,
    v       text        NOT NULL DEFAULT '',
    w       text        NOT NULL DEFAULT '',
    label   jsonb,
    created timestamptz 
);

CREATE TABLE menus (
    id      uuid        PRIMARY KEY,
    name    text        NOT NULL DEFAULT '',
    parent  text        NOT NULL DEFAULT '',
    options text        NOT NULL DEFAULT '',
    created timestamptz 
);

-- +goose Down
DROP TABLE IF EXISTS dag_history;
DROP TABLE IF EXISTS shares;
DROP TABLE IF EXISTS share_denylist;
DROP TABLE IF EXISTS menus;
DROP TABLE IF EXISTS edges;
DROP TABLE IF EXISTS nodes;
DROP TABLE IF EXISTS users;
DROP TABLE IF EXISTS dags;
