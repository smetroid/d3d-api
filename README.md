# d3d-api

Go REST + WebSocket backend for [d3dweb](https://github.com/smetroid/d3dweb). Stores directed acyclic graphs (DAGs) in PostgreSQL and provides real-time collaborative editing via a WebSocket relay.

## Tech stack

| Layer | Library |
|---|---|
| HTTP / WS | [Echo v3](https://github.com/labstack/echo) + [gorilla/websocket](https://github.com/gorilla/websocket) |
| Database | [PostgreSQL](https://www.postgresql.org/) via [pgx v5](https://github.com/jackc/pgx) |
| Migrations | [goose](https://github.com/pressly/goose) — embedded, applied on startup |
| Auth | JWT (HS256) — LDAP, OAuth, or local bcrypt |
| Config | TOML via BurntSushi/toml |

## Prerequisites

- Go 1.23+
- Docker (for local PostgreSQL)
- [`gin`](https://github.com/codegangsta/gin) for live-reload during development (optional: `go install github.com/codegangsta/gin@latest`)

## Local development setup

### 1. Start PostgreSQL

```bash
docker run -d --name pg -p 5432:5432 -e POSTGRES_PASSWORD=postgres postgres:16-alpine
# or if the container already exists:
docker start pg
```

The schema is managed by embedded goose migrations and is applied automatically on startup (`app/db/postgres/migrations/`). No manual setup is required.

### 2. Configure the API

Copy `samus_dev.toml` as your local config (it is gitignored):

```toml
[samus]
    bind_addr     = ":3001"
    signing_key   = "dev-signing-key-change-in-prod"
    auth_provider = "localauth"

[postgres]
    dsn = "postgres://postgres:postgres@localhost:5432/samus?sslmode=disable"
```

### 3. Create the first user

`localauth` stores bcrypt hashed users in the `users` table:

```bash
go run . createUser admin changeme --config samus_dev.toml
```

### 4. Run the API

```bash
go run . --config samus_dev.toml

# with live-reload
gin --all run samus.go -- --config samus_dev.toml
```

Without `--config`, the app reads `./samus.toml`. The API listens on the `bind_addr` from your config (e.g. `http://localhost:3001`).

## API reference

All endpoints except `/auth/login` and `/shares/exchange` require a `Bearer <jwt>` token in the `Authorization` header (or `?token=` / `?api-key=` query params).

### Auth

| Method | Path | Description |
|---|---|---|
| POST | `/auth/login` | Exchange credentials for a JWT |

### DAGs

| Method | Path | Description |
|---|---|---|
| POST | `/dag` | Create a new DAG |
| GET | `/dags` | List all DAGs |
| GET | `/dag/:dag` | Get a DAG by ID |
| POST | `/dag/:dag/update` | Save diagram JSON; triggers collab broadcast + history snapshot |
| DELETE | `/dag/:dag` | Delete a DAG |

### Real-time collaboration

| Method | Path | Description |
|---|---|---|
| GET | `/dag/:dag/ws` | Upgrade to WebSocket; join the collab room for this DAG |

The WebSocket relay forwards two message types:

- `diagram:updated` — broadcast to all peers when a save occurs (echo-prevented via `clientId`)
- `presence` — relayed to other peers in the room (cursor position, node selection, display name)

### Diagram history

| Method | Path | Description |
|---|---|---|
| GET | `/dag/:dag/history` | List snapshots (latest 50 per DAG) |
| POST | `/dag/:dag/history/:historyId/restore` | Restore a snapshot and broadcast `diagram:updated` |

### Share links

| Method | Path | Auth | Description |
|---|---|---|---|
| GET | `/shares/exchange?token=<jwt>` | None | Validate a share token; returns `dagId`, `role`, `anonName` |
| POST | `/dag/:dag/shares` | Required | Create a signed share JWT (`role`: `view`\|`edit`, `expDays`: int) |
| POST | `/dag/:dag/shares/:jti/revoke` | Required | Revoke a share by JTI |

Share tokens are signed with the same `signing_key` as regular auth tokens but carry `iss: "d3d-share"`. The `role` claim is enforced server-side:

- `view` — `POST /dag/:dag/update` returns 403; WS is allowed (presence only)
- `edit` — full read/write access

Revoked JTIs are stored in the `share_denylist` table and checked on WS upgrade and token exchange.

Anonymous display names (e.g. `"Teal Fox"`) are generated on share creation and returned by `/shares/exchange` so view-only users appear in the presence HUD with a readable identity.

### Nodes / Edges / Menus

| Method | Path | Description |
|---|---|---|
| POST | `/node` | Create a node |
| GET | `/nodes` | List nodes |
| POST | `/edge` | Create an edge |
| GET | `/edges` | List edges |
| POST | `/menu` | Create a menu |
| GET | `/menus` | List menus |
| GET | `/menus_options` | List menu options |
| GET | `/menu/:menu` | Get a menu |
| POST | `/menu/:menu/update` | Update a menu |
| DELETE | `/menu/:menu` | Delete a menu |

## Auth providers

Set `auth_provider` in your config to one of (defaults to `ldap` if unset):

| Value | Description |
|---|---|
| `ldap` | LDAP/AD bind auth. Configure the `[ldap]` block. |
| `oauth` | OAuth2. Configure the `[oauth]` block. |
| `localauth` | Bcrypt passwords stored in the Postgres `users` table. Seed users with `samus createUser` (see above). |

## Running tests

```bash
go test ./...
```

The `app/collab/` package has unit tests for the hub room registry and write pump.

## Database tables

| Table | Purpose |
|---|---|
| `dags` | Diagram documents |
| `dag_history` | Latest 50 snapshots per diagram (pruned on write) |
| `shares` | Active share tokens (jti, role, anonName, expiry) |
| `share_denylist` | Revoked JTIs |
| `users` | Local auth users (bcrypt hashed) |
| `nodes` | Standalone node records |
| `edges` | Standalone edge records |
| `menus` | Menu configurations |

The schema lives in `app/db/postgres/migrations/` and is applied automatically on startup by embedded goose migrations. To add a new migration, drop a `NNNN_name.sql` file in that directory.

## Migrating from RethinkDB

`cmd/migrate-rb2pg` copies data from an existing RethinkDB database into Postgres. It is idempotent (rows already present are skipped), so it is safe to run more than once.

```bash
go run ./cmd/migrate-rb2pg \
  -rb-address localhost:28015 \
  -rb-database samus \
  -pg-dsn "postgres://postgres:postgres@localhost:5432/samus?sslmode=disable"
```

Behavior notes:

- Preserves UUIDs and timestamps exactly; RethinkDB zero-time values become NULL.
- `diagram` / `snapshot_json` (stored as JSON strings in RethinkDB) are validated and normalized into `jsonb`.
- Tables missing in RethinkDB are skipped; rows whose parent DAG is gone are reported as orphans and skipped.
- Per-table counts are logged (`inserted` / `already present` / `orphaned` / `failed`); a non-zero exit code indicates real failures.

For the full step-by-step production cutover (freeze, preflight, migrate, verify, rollback), see [docs/cutover-runbook.md](docs/cutover-runbook.md).

## Deployment

The repo includes a `Dockerfile` and `fly.toml` for [Fly.io](https://fly.io) deployment. The image runs the API binary, which reads its config from `./samus.toml` (or the path given by `--config`). Provide a `samus.toml` with a `[postgres] dsn` pointing at your Postgres instance; `samus.tmpl` is an env-substitution template for rendering that config from environment variables (e.g. `POSTGRES_DSN`). CI is required to pass before deploy.

## CI/CD

GitHub Actions drives quality gates and deploys:

| Workflow | Triggers | Checks / actions |
|---|---|---|
| `.github/workflows/ci.yml` | every PR + push to `main` | gitleaks secret scan, golangci-lint, `go test -race` against a Postgres 16 service container (`TEST_DATABASE_URL`), `CGO_ENABLED=0 go build`, `go vet`, `go mod tidy` drift check |
| `.github/workflows/deploy.yml` | push to `main` | build image → push to GHCR (`ghcr.io/smetroid/d3d-api`) → `flyctl deploy` |

Repository secrets required for deploy: `FLY_API_TOKEN` (from `flyctl auth token` or the Fly dashboard). GHCR auth uses the automatic `GITHUB_TOKEN`; no extra secret needed.

Branch protection on `main` should require the `CI` workflow to pass before merging.

## Pre-commit hooks

A `.pre-commit-config.yaml` runs local quality gates before every commit:

- `gitleaks` — secrets scanning
- `golangci-lint` — linting (`.golangci.yml`)
- `gofmt`, `go mod tidy` drift check
- generic file hygiene (`trailing-whitespace`, `end-of-file-fixer`, `check-yaml`, `check-added-large-files`)

Install once:

```bash
make precommit-install   # pre-commit install
```

Run manually at any time:

```bash
make precommit-run       # pre-commit run --all-files
```

The `gitleaks` allowlist (public test fixtures in `app/auth/ldap/ldap_test.go`, docs samples) lives in `.gitleaks.toml`.
