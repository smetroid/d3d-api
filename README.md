# d3d-api

Go REST + WebSocket backend for [d3dweb](https://github.com/smetroid/d3dweb). Stores directed acyclic graphs (DAGs) in RethinkDB and provides real-time collaborative editing via a WebSocket relay.

## Tech stack

| Layer | Library |
|---|---|
| HTTP / WS | [Echo v3](https://github.com/labstack/echo) + [gorilla/websocket](https://github.com/gorilla/websocket) |
| Database | [RethinkDB](https://rethinkdb.com/) via gorethink v4 |
| Auth | JWT (HS256) — LDAP, OAuth, or local bcrypt |
| Config | TOML via BurntSushi/toml |

## Prerequisites

- Go 1.23+
- RethinkDB running on `localhost:28015`
- [`gin`](https://github.com/codegangsta/gin) for live-reload during development (`go install github.com/codegangsta/gin@latest`)

## Local development setup

### 1. Start RethinkDB

```bash
docker run -d --name rethinkdb -p 28015:28015 -p 8080:8080 rethinkdb
# or if the container already exists:
docker start rethinkdb
```

### 2. Configure the API

Copy `samus_dev.toml` as your local config (it is gitignored). The defaults use `localauth` (bcrypt-backed users in RethinkDB) and bind to `:3001`:

```toml
[samus]
    bind_addr    = ":3001"
    signing_key  = "dev-signing-key-change-in-prod"
    auth_provider = "localauth"

[rethinkdb]
    address  = "localhost:28015"
    database = "samus"
```

### 3. Create the first user

```bash
go run cmd/createuser/main.go -config samus_dev.toml -username admin -password changeme
```

### 4. Run the API

```bash
# with live-reload
SAMUS_CONFIG=samus_dev.toml gin --all run samus.go

# without live-reload
SAMUS_CONFIG=samus_dev.toml go run samus.go
```

The API listens on `http://localhost:3001`.

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
| GET | `/dag/:dag/history` | List the last 50 snapshots |
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

Revoked JTIs are stored in `share_denylist` and checked on WS upgrade and token exchange.

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

Set `auth_provider` in your config to one of:

| Value | Description |
|---|---|
| `localauth` | Bcrypt passwords stored in RethinkDB `users` table. Use `cmd/createuser` to seed users. |
| `ldap` | LDAP/AD bind auth. Configure the `[ldap]` block. |
| `oauth` | OAuth2. Configure the `[oauth]` block. |

## Running tests

```bash
go test ./...
```

The `app/collab/` package has unit tests for the hub room registry and write pump. The pagerduty notifier is not compiled by default (build tag excluded).

## RethinkDB tables

| Table | Purpose |
|---|---|
| `dags` | Diagram documents |
| `dag_history` | Last 50 snapshots per diagram (pruned on write) |
| `shares` | Active share tokens (jti, role, anonName, expiry) |
| `share_denylist` | Revoked JTIs |
| `users` | Local auth users (bcrypt hashed) |
| `nodes` | Standalone node records |
| `edges` | Standalone edge records |
| `menus` | Menu configurations |

Tables and secondary indexes are created automatically on startup.

## Deployment

The repo includes a `Dockerfile` and `fly.toml` for [Fly.io](https://fly.io) deployment. Production config is loaded from `samus.toml` (or the path set by `SAMUS_CONFIG`). CI is required to pass before deploy.
