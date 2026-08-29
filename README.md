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
make postgres-start
```

That starts the existing `pg` container, or creates one if it is missing:

```bash
docker run -d --name pg -p 5432:5432 -e POSTGRES_PASSWORD=postgres -e POSTGRES_DB=samus postgres:16-alpine
```

`POSTGRES_DB=samus` matters because the DSN below names that database. It only takes effect when the data directory is initialized, so a container created **without** it keeps failing with `database "samus" does not exist`. Create the database once by hand rather than recreating the container:

```bash
docker exec pg psql -U postgres -c 'CREATE DATABASE samus;'
```

The schema itself needs no setup. `Init` applies the embedded goose migrations on startup (`app/db/postgres/migrations/`), so an empty database is enough.

### 2. Configure the API

Copy the template; your copy is gitignored, so edit it freely:

```bash
cp samus_dev.toml.example samus_dev.toml
```

It defaults to `auth_provider = "localauth"` (no LDAP server needed) and points at the container from step 1:

```toml
[postgres]
    dsn = "postgres://postgres:postgres@localhost:5432/samus?sslmode=disable"
```

Alternatively, spell out the connection as discrete fields under `[postgresql]`
(a DSN is assembled for you, with the password properly escaped):

```toml
[postgresql]
    address  = "localhost:5432"
    user     = "samus"
    password = "dev-password-change-in-prod"
    database = "samus"
```

A non-empty `dsn` always wins when both forms are present.

Configuring neither is not the same as omitting the section: the DSN then falls back to a passwordless `postgres://localhost:5432/samus`, which will not authenticate against the container from step 1.

### 3. Create the first user

`localauth` stores bcrypt hashed users in the `users` table:

```bash
go run . createUser admin changeme --config samus_dev.toml
```

### 4. Run the API

```bash
go run . --config samus_dev.toml
```

Or with live-reload, which also starts PostgreSQL for you:

```bash
make start-api-service
```

That runs `gin` as a reverse proxy: `gin` listens on **:3000** and rebuilds on change, while the app itself binds `bind_addr` (`:3001`). Make your requests against `http://localhost:3000` so you get the reloading proxy. Running `go run .` directly means no proxy, so use `:3001` instead.

Without `--config`, the app reads `./samus.toml` — the LDAP config, which has no `[postgres]` section. Note that `gin` forwards everything after `run` to the built binary as arguments, so the flag belongs there directly (`gin run --config samus_dev.toml`); passing a filename such as `samus.go` silently consumes the first argument slot and the app falls back to `./samus.toml`.

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

The image runs the API binary, which reads its config from `./samus.toml` (or the path given by `--config`). Provide a `samus.toml` with a `[postgres] dsn` (or a discrete `[postgresql]` block) pointing at your Postgres instance; `samus.tmpl` is an env-substitution template for rendering that config from environment variables (e.g. `POSTGRES_DSN`). CI is required to pass before deploy.

## CI/CD

GitHub Actions drives quality gates and deploys:

| Workflow | Triggers | Checks / actions |
|---|---|---|
| `.github/workflows/ci.yml` | every PR + push to `main` | gitleaks secret scan, golangci-lint, `go test -race` against a Postgres 16 service container (`TEST_DATABASE_URL`), `CGO_ENABLED=0 go build`, `go vet`, `go mod tidy` drift check |
| `.github/workflows/deploy.yml` | push to `main` | build image → push to GHCR (`ghcr.io/smetroid/d3d-api`) → `flyctl deploy` |
| `.github/workflows/vercel-registry-prune.yml` | daily at 06:00 UTC + manual dispatch | delete all but the 25 newest images in the Vercel container registry |

Repository secrets required for deploy: `FLY_API_TOKEN` (from `flyctl auth token` or the Fly dashboard). GHCR auth uses the automatic `GITHUB_TOKEN`; no extra secret needed.

### Vercel container registry pruning

Vercel deployments push one image per commit to the project's registry repository, and nothing removes the old ones. The repository has a hard cap on image count; once it is reached, every build fails at the push step with `denied: repository has reached the maximum allowed number of images` — the image builds fine, only the upload is rejected.

The prune workflow keeps the 25 newest images and deletes the rest. The rule is count-based rather than age-based because the cap itself is a count: "older than N days" can still overflow during a burst of deployments. Deleting an image does not delete its deployment, but rolling back to a deployment whose image is gone requires a rebuild.

The same logic runs locally when a build is blocked and you do not want to wait for the schedule:

```bash
DRY_RUN=1 scripts/prune-vercel-images.sh   # report only
scripts/prune-vercel-images.sh             # prune to the newest 25
KEEP=10 scripts/prune-vercel-images.sh     # keep fewer
```

Repository secret required: `VERCEL_TOKEN`, created at <https://vercel.com/account/tokens> and scoped to the `smetroids-projects` team. Locally the script uses your `vercel login` session instead.

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
