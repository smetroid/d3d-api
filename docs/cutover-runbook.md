# RethinkDB → Postgres cutover runbook

Step-by-step procedure for migrating production data from RethinkDB into PostgreSQL
and switching `d3d-api` over to the new database.

The data-copy tool is `cmd/migrate-rb2pg`. It is idempotent: already-present rows
are skipped, so re-running is always safe.

## 0. Prerequisites

- A build of the Postgres-backed API (any commit on the `feat-postgresql-migration` branch).
- Access to the running RethinkDB instance (driver port `28015`).
- A Postgres instance reachable from the API hosts (Neon, via the Vercel integration, or the `pg-migrate-test`
  container for a rehearsal).

## 1. Freeze writes

Migration copies a point-in-time snapshot. Schedule the cutover for a low-traffic window and:

1. Pause the API (scale down / stop machines) so no new DAG edits, shares, or users are written.
2. Note that RethinkDB has **no foreign keys**: `dag_history` and `shares` rows whose parent DAG
   was deleted are orphans. The tool detects these up front and skips them (see Step 3 for what
   the counts should look like). This is expected, not an error.

## 2. Provision Postgres

1. Provision a Postgres 14+ instance (e.g. `fly postgres create` and attach to the app).
2. Ensure the `samus` database exists and the app role has `CONNECT`, `CREATE`, `USAGE`
   privileges on it. Schema creation is handled by the tool (embedded goose migrations
   run before any insert).

## 3. Run the migration

```bash
go run ./cmd/migrate-rb2pg \
  -rb-address <rethinkdb-host>:28015 \
  -rb-database samus \
  -pg-dsn "<postgres-dsn>"
```

Flags:

| Flag | Description |
|---|---|
| `-rb-address` | RethinkDB host:port (default `localhost:28015`) |
| `-rb-database` | RethinkDB database (default `samus`) |
| `-pg-dsn` | Postgres DSN (required) |

The tool logs a per-table breakdown, e.g.:

```
dags: 1 rows, 1 inserted, 0 already present, 0 orphaned, 0 failed
dag_history: 78 rows, 50 inserted, 0 already present, 28 orphaned, 0 failed
shares: 8 rows, 8 inserted, 0 already present, 0 orphaned, 0 failed
share_denylist: 0 rows, 0 inserted, 0 already present, 0 orphaned, 0 failed
users: 1 rows, 1 inserted, 0 already present, 0 orphaned, 0 failed
nodes: table does not exist in RethinkDB, skipping
```

- **orphaned** = rows skipped because their parent DAG no longer exists in RethinkDB.
- **failed** = genuine errors (invalid data, etc.). Only a non-zero `failed` count means the
  run did not succeed; the process exits non-zero in that case.
- Tables missing in RethinkDB are skipped (`nodes`/`edges`/`menus` are legacy/dormant).

## 4. Verify

1. **Idempotency** — re-run the same command; expect `0 inserted` and everything
   `already present`. This is the fastest check that the copy is complete.
2. **Row counts** — for each table, `inserted` should equal the RethinkDB row count
   minus `orphaned`.
3. **Spot-check data** (psql):

   ```sql
   -- jsonb columns are valid JSON objects
   SELECT id, name, jsonb_typeof(diagram) FROM dags;
   -- zero-time RethinkDB values became NULL, real timestamps preserved
   SELECT id, created, updated FROM dags;
   -- history snapshots preserved
   SELECT count(*), min(saved_at), max(saved_at) FROM dag_history;
   -- denylist jti mapping
   SELECT jti, revoked_at FROM share_denylist;
   ```

4. **End-to-end** — boot the API against Postgres and confirm:
   - login works (`POST /auth/login`),
   - `GET /dags` lists the migrated DAGs,
   - `GET /dag/:id` returns the DAG with its `diagram`,
   - `GET /dag/:id/history` returns the migrated snapshots.

## 5. Cut over the app

1. Point the app config at Postgres. Example `samus.toml`:

   ```toml
   [samus]
       bind_addr     = ":3001"
       signing_key   = "<prod-signing-key>"
       auth_provider = "localauth"

   [postgres]
       dsn = "<postgres-dsn>"
   ```

    For container deployments, render this from `samus.tmpl` and `POSTGRES_DSN`
    (see README → Deployment).
2. Deploy. `Postgres.Init()` applies any pending goose migrations on startup.
3. Smoke-test the same endpoints from Step 4 against the new deployment.

## 6. Rollback

If anything is wrong after cutover:

1. Deploy the previous build back onto the RethinkDB-backed config (`[rethinkdb]` block) —
   the RethinkDB instance is untouched by the migration (read-only access).
2. The Postgres copy can be re-run at any time to refresh data once writes resume.

## 7. Post-cutover cleanup

1. Keep RethinkDB running for a grace period (e.g. a week) in case rollback is needed.
2. After the grace period: stop the RethinkDB container/service.
3. Optional: drop `gorethink` from `go.mod` — note it is still a required dependency of
   `cmd/migrate-rb2pg` (the only tool that reads RethinkDB).
