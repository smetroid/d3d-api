package postgres

import (
	"context"
	"embed"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pressly/goose/v3"
	"github.com/smetroid/d3d-api/app/models"

	_ "github.com/jackc/pgx/v5/stdlib"
)

//go:embed migrations/*.sql
var embedMigrations embed.FS

// ErrNotFound is returned when a requested record does not exist.
var ErrNotFound = errors.New("not found")

// Postgres is the d3d-api storage layer, replacing RethinkDB. The full
// repository surface (DAGs, history, shares, users, nodes, edges, menus)
// mirrors the method set previously implemented on top of RethinkDB.
type Postgres struct {
	DSN  string `toml:"dsn"`
	pool *pgxpool.Pool
}

// Pool returns the open connection pool. It is nil until Init succeeds.
func (p *Postgres) Pool() *pgxpool.Pool { return p.pool }

// Init opens the connection pool and applies the embedded goose migrations.
func (p *Postgres) Init() error {
	if p.pool != nil {
		return nil
	}
	if p.DSN == "" {
		p.DSN = "postgres://localhost:5432/samus"
	}

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, p.DSN)
	if err != nil {
		return err
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return err
	}
	p.pool = pool

	goose.SetBaseFS(embedMigrations)
	db, err := goose.OpenDBWithDriver("postgres", p.DSN)
	if err != nil {
		return err
	}
	if err := goose.Up(db, "migrations"); err != nil {
		db.Close()
		return err
	}
	return db.Close()
}

// jsonbValue returns nil for empty strings so empty payloads store as NULL
// instead of failing jsonb's JSON parsing.
func jsonbValue(s string) any {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	return s
}

// timeOrNil returns nil for zero times so absent timestamps store as NULL.
func timeOrNil(t time.Time) any {
	if t.IsZero() {
		return nil
	}
	return t
}

// ifNonZero returns nil for empty strings so partial updates skip them,
// mirroring gorethink's omit-empty Update semantics.
func ifNonZero(s string) any {
	if s == "" {
		return nil
	}
	return s
}

// ifNonEmptyJSON returns nil for empty JSON strings so partial updates skip
// them, but falls back to the string itself so it is written as a jsonb value.
func ifNonEmptyJSON(s string) any {
	return jsonbValue(s)
}

// ifNonZeroTime returns nil for zero times so partial updates skip them.
func ifNonZeroTime(t time.Time) any {
	return timeOrNil(t)
}

// ─── DAGs ───────────────────────────────────────────────────────────────────

func (p *Postgres) CreateDAG(dag models.Dag) (string, error) {
	ids, err := p.CreateDAGs([]models.Dag{dag})
	if err != nil {
		return "", err
	}
	if len(ids) < 1 {
		return dag.Id, nil
	}
	return ids[0], nil
}

func (p *Postgres) CreateDAGs(dags []models.Dag) (ids []string, err error) {
	ctx := context.Background()
	for _, dag := range dags {
		if dag.Id == "" {
			dag.Id = uuid.New().String()
		}
		_, err = p.pool.Exec(ctx, `
			INSERT INTO dags (id, name, description, diagram, created, updated)
			VALUES ($1, $2, $3, $4, $5, $6)`,
			dag.Id, dag.Name, dag.Description, jsonbValue(dag.Diagram),
			timeOrNil(dag.Created), timeOrNil(dag.Updated))
		if err != nil {
			return
		}
		ids = append(ids, dag.Id)
	}
	return
}

func (p *Postgres) GetDAG(id string) (dag models.Dag, err error) {
	if _, parseErr := uuid.Parse(id); parseErr != nil {
		return models.Dag{}, ErrNotFound
	}
	var created, updated *time.Time
	var diagram *string
	err = p.pool.QueryRow(context.Background(), `
		SELECT id, name, description, diagram, created, updated, public, embed_revision
		FROM dags WHERE id = $1`, id).Scan(
		&dag.Id, &dag.Name, &dag.Description, &diagram, &created, &updated, &dag.Public, &dag.EmbedRevision)
	if errors.Is(err, pgx.ErrNoRows) {
		return models.Dag{}, ErrNotFound
	}
	if diagram != nil {
		dag.Diagram = *diagram
	}
	if created != nil {
		dag.Created = *created
	}
	if updated != nil {
		dag.Updated = *updated
	}
	return
}

func (p *Postgres) DeleteDAG(id string) error {
	_, err := p.pool.Exec(context.Background(), `DELETE FROM dags WHERE id = $1`, id)
	return err
}

// UpdateDAG merges only the non-zero fields. Every content save atomically
// increments embed_revision so caches can reliably detect stale renders.
func (p *Postgres) UpdateDAG(id string, updates models.Dag) error {
	fields := map[string]any{
		"name":        ifNonZero(updates.Name),
		"description": ifNonZero(updates.Description),
		"diagram":     ifNonEmptyJSON(updates.Diagram),
		"created":     ifNonZeroTime(updates.Created),
		"updated":     ifNonZeroTime(updates.Updated),
	}

	// Build partial SET clause for non-nil fields, then append the atomic bump.
	var sets []string
	args := make([]any, 0, len(fields)+1)
	i := 1
	for col, val := range fields {
		if val == nil {
			continue
		}
		sets = append(sets, fmt.Sprintf("%s = $%d", col, i))
		args = append(args, val)
		i++
	}
	sets = append(sets, "embed_revision = embed_revision + 1")
	args = append(args, id)
	_, err := p.pool.Exec(context.Background(),
		"UPDATE dags SET "+strings.Join(sets, ", ")+" WHERE id = $"+fmt.Sprintf("%d", i),
		args...)
	return err
}

// SetPublic toggles whether a DAG is publicly readable without authentication.
func (p *Postgres) SetPublic(id string, public bool) error {
	_, err := p.pool.Exec(context.Background(),
		`UPDATE dags SET public = $1 WHERE id = $2`, public, id)
	return err
}

// GetDAGPublic returns the DAG only when it is marked public; callers should
// treat any error as "not found or not public" to avoid leaking existence.
func (p *Postgres) GetDAGPublic(id string) (dag models.Dag, err error) {
	var diagram *string
	var created, updated *time.Time
	err = p.pool.QueryRow(context.Background(), `
		SELECT id, name, description, diagram, created, updated, embed_revision
		FROM dags WHERE id = $1 AND public = TRUE`, id).Scan(
		&dag.Id, &dag.Name, &dag.Description, &diagram, &created, &updated, &dag.EmbedRevision)
	if err != nil {
		return
	}
	if diagram != nil {
		dag.Diagram = *diagram
	}
	if created != nil {
		dag.Created = *created
	}
	if updated != nil {
		dag.Updated = *updated
	}
	dag.Public = true
	return
}

// partialUpdate builds an UPDATE that sets only the non-nil fields.
func (p *Postgres) partialUpdate(table, id string, fields map[string]any) error {
	if len(fields) == 0 {
		return nil
	}
	var sets []string
	args := make([]any, 0, len(fields)+1)
	i := 1
	for col, val := range fields {
		if val == nil {
			continue
		}
		sets = append(sets, col+" = $"+fmt.Sprintf("%d", i))
		args = append(args, val)
		i++
	}
	if len(sets) == 0 {
		return nil
	}
	args = append(args, id)
	_, err := p.pool.Exec(context.Background(),
		"UPDATE "+table+" SET "+strings.Join(sets, ", ")+" WHERE id = $"+fmt.Sprintf("%d", i), args...)
	return err
}

func (p *Postgres) FindRelatedDAG(dag models.Dag) (relatedDAG models.Dag, foundDAG bool, err error) {
	return p.findOneDAG(`
		SELECT id, name, description, diagram, created, updated, public, embed_revision FROM dags
		WHERE name = $1 AND description = $2
		  AND diagram IS NOT DISTINCT FROM $3::jsonb`,
		dag.Name, dag.Description, jsonbValue(dag.Diagram))
}

func (p *Postgres) findOneDAG(query string, args ...any) (dag models.Dag, foundOne bool, err error) {
	var created, updated *time.Time
	var diagram *string
	err = p.pool.QueryRow(context.Background(), query, args...).Scan(
		&dag.Id, &dag.Name, &dag.Description, &diagram, &created, &updated, &dag.Public, &dag.EmbedRevision)
	if diagram != nil {
		dag.Diagram = *diagram
	}
	if created != nil {
		dag.Created = *created
	}
	if updated != nil {
		dag.Updated = *updated
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return models.Dag{}, false, nil
	}
	if err != nil {
		return models.Dag{}, false, err
	}
	return dag, true, nil
}

func (p *Postgres) GetDAGsSummary(queryArgs map[string][]string) (dagsSummary []map[string]interface{}, err error) {
	rows, err := p.pool.Query(context.Background(), `
		SELECT id, name, description, diagram::text AS diagram, created, updated
		FROM dags`)
	if err != nil {
		return
	}
	defer rows.Close()
	dagsSummary, err = pgx.CollectRows(rows, pgx.RowToMap)
	if dagsSummary == nil {
		dagsSummary = make([]map[string]interface{}, 0)
	}
	return
}

// ─── Nodes ──────────────────────────────────────────────────────────────────

func (p *Postgres) CreateNode(node models.Node) (string, error) {
	ids, err := p.CreateNodes([]models.Node{node})
	if err != nil {
		return "", err
	}
	if len(ids) < 1 {
		return node.Id, nil
	}
	return ids[0], nil
}

func (p *Postgres) CreateNodes(nodes []models.Node) (ids []string, err error) {
	ctx := context.Background()
	for _, node := range nodes {
		if node.Id == "" {
			node.Id = uuid.New().String()
		}
		_, err = p.pool.Exec(ctx, `
			INSERT INTO nodes (id, v, parent, value_label, value_type,
			                   value_cluster_label_pos, value_style, created)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
			node.Id, node.V, node.Parent, node.ValueLabel, node.ValueType,
			node.ValueClusterLabelPos, node.ValueStyle, timeOrNil(node.Created))
		if err != nil {
			return
		}
		ids = append(ids, node.Id)
	}
	return
}

func (p *Postgres) GetNode(id string) (node models.Node, err error) {
	var created *time.Time
	var valueLabel *map[string]string
	err = p.pool.QueryRow(context.Background(), `
		SELECT id, v, parent, value_label, value_type, value_cluster_label_pos,
		       value_style, created
		FROM nodes WHERE id = $1`, id).Scan(
		&node.Id, &node.V, &node.Parent, &valueLabel, &node.ValueType,
		&node.ValueClusterLabelPos, &node.ValueStyle, &created)
	if valueLabel != nil {
		node.ValueLabel = *valueLabel
	}
	if created != nil {
		node.Created = *created
	}
	return
}

func (p *Postgres) DeleteNode(id string) error {
	_, err := p.pool.Exec(context.Background(), `DELETE FROM nodes WHERE id = $1`, id)
	return err
}

func (p *Postgres) FindRelatedNode(node models.Node) (relatedNode models.Node, foundNode bool, err error) {
	return p.findOneNode(`
		SELECT id, v, parent, value_label, value_type, value_cluster_label_pos,
		       value_style, created FROM nodes
		WHERE v = $1 AND parent = $2
		  AND value_label IS NOT DISTINCT FROM $3::jsonb
		  AND value_type = $4 AND value_cluster_label_pos = $5 AND value_style = $6`,
		node.V, node.Parent, node.ValueLabel, node.ValueType,
		node.ValueClusterLabelPos, node.ValueStyle)
}

func (p *Postgres) findOneNode(query string, args ...any) (node models.Node, foundOne bool, err error) {
	var created *time.Time
	var valueLabel *map[string]string
	err = p.pool.QueryRow(context.Background(), query, args...).Scan(
		&node.Id, &node.V, &node.Parent, &valueLabel, &node.ValueType,
		&node.ValueClusterLabelPos, &node.ValueStyle, &created)
	if valueLabel != nil {
		node.ValueLabel = *valueLabel
	}
	if created != nil {
		node.Created = *created
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return models.Node{}, false, nil
	}
	if err != nil {
		return models.Node{}, false, err
	}
	return node, true, nil
}

func (p *Postgres) GetNodesSummary(queryArgs map[string][]string) (nodesSummary []map[string]interface{}, err error) {
	rows, err := p.pool.Query(context.Background(), `
		SELECT id, v, parent, value_label, value_type, value_cluster_label_pos,
		       value_style, created
		FROM nodes`)
	if err != nil {
		return
	}
	defer rows.Close()
	for rows.Next() {
		var n models.Node
		var created *time.Time
		var valueLabel *map[string]string
		if err = rows.Scan(&n.Id, &n.V, &n.Parent, &valueLabel, &n.ValueType,
			&n.ValueClusterLabelPos, &n.ValueStyle, &created); err != nil {
			return
		}
		if valueLabel != nil {
			n.ValueLabel = *valueLabel
		}
		if created != nil {
			n.Created = *created
		}
		nodesSummary = append(nodesSummary, map[string]interface{}{
			"id":              n.Id,
			"v":               n.V,
			"parent":          n.Parent,
			"valueLabel":      n.ValueLabel,
			"labelType":       n.ValueType,
			"clusterLabelPos": n.ValueClusterLabelPos,
			"clusterStyle":    n.ValueStyle,
			"created":         n.Created,
		})
	}
	if nodesSummary == nil {
		nodesSummary = make([]map[string]interface{}, 0)
	}
	return
}

// ─── Edges ──────────────────────────────────────────────────────────────────

func (p *Postgres) CreateEdge(edge models.Edge) (string, error) {
	ids, err := p.CreateEdges([]models.Edge{edge})
	if err != nil {
		return "", err
	}
	if len(ids) < 1 {
		return edge.Id, nil
	}
	return ids[0], nil
}

func (p *Postgres) CreateEdges(edges []models.Edge) (ids []string, err error) {
	ctx := context.Background()
	for _, edge := range edges {
		if edge.Id == "" {
			edge.Id = uuid.New().String()
		}
		_, err = p.pool.Exec(ctx, `
			INSERT INTO edges (id, v, w, label, created)
			VALUES ($1, $2, $3, $4, $5)`,
			edge.Id, edge.V, edge.W, edge.Label, timeOrNil(edge.Created))
		if err != nil {
			return
		}
		ids = append(ids, edge.Id)
	}
	return
}

func (p *Postgres) GetEdge(id string) (edge models.Edge, err error) {
	var created *time.Time
	var label *map[string]string
	err = p.pool.QueryRow(context.Background(), `
		SELECT id, v, w, label, created FROM edges WHERE id = $1`, id).Scan(
		&edge.Id, &edge.V, &edge.W, &label, &created)
	if label != nil {
		edge.Label = *label
	}
	if created != nil {
		edge.Created = *created
	}
	return
}

func (p *Postgres) DeleteEdge(id string) error {
	_, err := p.pool.Exec(context.Background(), `DELETE FROM edges WHERE id = $1`, id)
	return err
}

func (p *Postgres) FindRelatedEdge(edge models.Edge) (relatedEdge models.Edge, foundEdge bool, err error) {
	return p.findOneEdge(`
		SELECT id, v, w, label, created FROM edges
		WHERE v = $1 AND w = $2 AND label IS NOT DISTINCT FROM $3::jsonb`,
		edge.V, edge.W, edge.Label)
}

func (p *Postgres) findOneEdge(query string, args ...any) (edge models.Edge, foundOne bool, err error) {
	var created *time.Time
	var label *map[string]string
	err = p.pool.QueryRow(context.Background(), query, args...).Scan(
		&edge.Id, &edge.V, &edge.W, &label, &created)
	if label != nil {
		edge.Label = *label
	}
	if created != nil {
		edge.Created = *created
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return models.Edge{}, false, nil
	}
	if err != nil {
		return models.Edge{}, false, err
	}
	return edge, true, nil
}

func (p *Postgres) GetEdgesSummary(queryArgs map[string][]string) (edgesSummary []map[string]interface{}, err error) {
	rows, err := p.pool.Query(context.Background(), `
		SELECT id, v, w, label, created FROM edges`)
	if err != nil {
		return
	}
	defer rows.Close()
	for rows.Next() {
		var e models.Edge
		var created *time.Time
		if err = rows.Scan(&e.Id, &e.V, &e.W, &e.Label, &created); err != nil {
			return
		}
		if created != nil {
			e.Created = *created
		}
		edgesSummary = append(edgesSummary, map[string]interface{}{
			"id":      e.Id,
			"v":       e.V,
			"w":       e.W,
			"label":   e.Label,
			"created": e.Created,
		})
	}
	if edgesSummary == nil {
		edgesSummary = make([]map[string]interface{}, 0)
	}
	return
}

// ─── Menus ──────────────────────────────────────────────────────────────────

func (p *Postgres) CreateMenu(menu models.Menu) (string, error) {
	ids, err := p.CreateMenus([]models.Menu{menu})
	if err != nil {
		return "", err
	}
	if len(ids) < 1 {
		return menu.Id, nil
	}
	return ids[0], nil
}

func (p *Postgres) CreateMenus(menus []models.Menu) (ids []string, err error) {
	ctx := context.Background()
	for _, menu := range menus {
		if menu.Id == "" {
			menu.Id = uuid.New().String()
		}
		_, err = p.pool.Exec(ctx, `
			INSERT INTO menus (id, name, parent, options, created)
			VALUES ($1, $2, $3, $4, $5)`,
			menu.Id, menu.Name, menu.Parent, menu.Options, timeOrNil(menu.Created))
		if err != nil {
			return
		}
		ids = append(ids, menu.Id)
	}
	return
}

func (p *Postgres) GetMenu(id string) (menu models.Menu, err error) {
	var created *time.Time
	err = p.pool.QueryRow(context.Background(), `
		SELECT id, name, parent, options, created FROM menus WHERE id = $1`, id).Scan(
		&menu.Id, &menu.Name, &menu.Parent, &menu.Options, &created)
	if created != nil {
		menu.Created = *created
	}
	return
}

func (p *Postgres) DeleteMenu(id string) error {
	_, err := p.pool.Exec(context.Background(), `DELETE FROM menus WHERE id = $1`, id)
	return err
}

func (p *Postgres) UpdateMenu(id string, updates models.Menu) error {
	return p.partialUpdate("menus", id, map[string]any{
		"name":    ifNonZero(updates.Name),
		"parent":  ifNonZero(updates.Parent),
		"options": ifNonZero(updates.Options),
		"created": ifNonZeroTime(updates.Created),
	})
}

func (p *Postgres) FindRelatedMenu(menu models.Menu) (relatedMenu models.Menu, foundMenu bool, err error) {
	return p.findOneMenu(`
		SELECT id, name, parent, options, created FROM menus
		WHERE parent = $1 AND options = $2`,
		menu.Parent, menu.Options)
}

func (p *Postgres) findOneMenu(query string, args ...any) (menu models.Menu, foundOne bool, err error) {
	var created *time.Time
	err = p.pool.QueryRow(context.Background(), query, args...).Scan(
		&menu.Id, &menu.Name, &menu.Parent, &menu.Options, &created)
	if created != nil {
		menu.Created = *created
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return models.Menu{}, false, nil
	}
	if err != nil {
		return models.Menu{}, false, err
	}
	return menu, true, nil
}

func (p *Postgres) GetMenusSummary(queryArgs map[string][]string) (menusSummary []map[string]interface{}, err error) {
	rows, err := p.pool.Query(context.Background(), `
		SELECT id, name, parent, options, created FROM menus`)
	if err != nil {
		return
	}
	defer rows.Close()
	menusSummary, err = pgx.CollectRows(rows, pgx.RowToMap)
	if menusSummary == nil {
		menusSummary = make([]map[string]interface{}, 0)
	}
	return
}

func (p *Postgres) GetMenusOptions(queryArgs map[string][]string) (menusOptions map[string]models.Menu, err error) {
	rows, err := p.pool.Query(context.Background(), `
		SELECT id, name, parent, options, created FROM menus`)
	if err != nil {
		return
	}
	defer rows.Close()
	menusOptions = make(map[string]models.Menu)
	for rows.Next() {
		var m models.Menu
		var created *time.Time
		if err = rows.Scan(&m.Id, &m.Name, &m.Parent, &m.Options, &created); err != nil {
			return
		}
		if created != nil {
			m.Created = *created
		}
		menusOptions[m.Id] = m
	}
	return
}

// ─── Shares ─────────────────────────────────────────────────────────────────

func (p *Postgres) InitSharesTables() error { return nil }

func (p *Postgres) GetShareByJti(jti string) (models.Share, error) {
	var s models.Share
	err := p.pool.QueryRow(context.Background(), `
		SELECT id, dag_id, jti, role, anon_name, created_by, expires_at, created_at
		FROM shares WHERE jti = $1`, jti).Scan(
		&s.Id, &s.DagId, &s.Jti, &s.Role, &s.AnonName, &s.CreatedBy, &s.ExpiresAt, &s.CreatedAt)
	return s, err
}

func (p *Postgres) CreateShare(s models.Share) error {
	if s.Id == "" {
		s.Id = uuid.New().String()
	}
	_, err := p.pool.Exec(context.Background(), `
		INSERT INTO shares (id, dag_id, jti, role, anon_name, created_by, expires_at, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		s.Id, s.DagId, s.Jti, s.Role, s.AnonName, s.CreatedBy,
		timeOrNil(s.ExpiresAt), timeOrNil(s.CreatedAt))
	return err
}

func (p *Postgres) RevokeShare(jti string) error {
	_, err := p.pool.Exec(context.Background(), `
		INSERT INTO share_denylist (jti, revoked_at) VALUES ($1, $2)
		ON CONFLICT (jti) DO UPDATE SET revoked_at = EXCLUDED.revoked_at`,
		jti, time.Now())
	return err
}

func (p *Postgres) IsRevoked(jti string) (bool, error) {
	var revoked bool
	err := p.pool.QueryRow(context.Background(), `
		SELECT EXISTS (SELECT 1 FROM share_denylist WHERE jti = $1)`, jti).Scan(&revoked)
	return revoked, err
}

// ─── Users (local auth) ─────────────────────────────────────────────────────

func (p *Postgres) InitUsersTable() error { return nil }

func (p *Postgres) CreateUser(u models.User) error {
	if u.Id == "" {
		u.Id = uuid.New().String()
	}
	_, err := p.pool.Exec(context.Background(), `
		INSERT INTO users (id, username, password_hash, created_at)
		VALUES ($1, $2, $3, $4)`,
		u.Id, u.Username, u.PasswordHash, timeOrNil(u.CreatedAt))
	return err
}

func (p *Postgres) GetUser(username string) (models.User, error) {
	var u models.User
	err := p.pool.QueryRow(context.Background(), `
		SELECT id, username, password_hash, created_at
		FROM users WHERE username = $1 LIMIT 1`, username).Scan(
		&u.Id, &u.Username, &u.PasswordHash, &u.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return models.User{}, nil
	}
	return u, err
}

func (p *Postgres) UpdateUserPassword(username, passwordHash string) error {
	tag, err := p.pool.Exec(context.Background(), `
		UPDATE users SET password_hash = $1 WHERE username = $2`, passwordHash, username)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("user %q not found", username)
	}
	return nil
}

// ─── History ────────────────────────────────────────────────────────────────

const historyLimit = 50

func (p *Postgres) AppendHistory(dagId, snapshotJSON, savedBy string) error {
	_, err := p.pool.Exec(context.Background(), `
		INSERT INTO dag_history (id, dag_id, snapshot_json, saved_by, saved_at)
		VALUES ($1, $2, $3, $4, $5)`,
		uuid.New().String(), dagId, jsonbValue(snapshotJSON), savedBy, time.Now())
	if err != nil {
		return err
	}
	go p.pruneHistory(dagId)
	return nil
}

// pruneHistory keeps only the newest historyLimit snapshots per DAG.
func (p *Postgres) pruneHistory(dagId string) {
	_, err := p.pool.Exec(context.Background(), `
		DELETE FROM dag_history dh
		WHERE dh.dag_id = $1
		  AND dh.id NOT IN (
		      SELECT id FROM dag_history
		      WHERE dag_id = $1
		      ORDER BY saved_at DESC, id DESC
		      LIMIT $2
		  )`, dagId, historyLimit)
	if err != nil {
		// Pruning is best-effort; a busy loop retrying adds little value here.
		log.Printf("history prune error dag=%s: %v", dagId, err)
	}
}

func (p *Postgres) GetHistory(dagId string) ([]models.DagHistory, error) {
	rows, err := p.pool.Query(context.Background(), `
		SELECT id, dag_id, snapshot_json, saved_by, saved_at
		FROM dag_history WHERE dag_id = $1
		ORDER BY saved_at DESC, id DESC
		LIMIT $2`, dagId, historyLimit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var history []models.DagHistory
	for rows.Next() {
		var h models.DagHistory
		var snapshotJSON *string
		if err = rows.Scan(&h.Id, &h.DagId, &snapshotJSON, &h.SavedBy, &h.SavedAt); err != nil {
			return nil, err
		}
		if snapshotJSON != nil {
			h.SnapshotJSON = *snapshotJSON
		}
		history = append(history, h)
	}
	if history == nil {
		history = []models.DagHistory{}
	}
	return history, rows.Err()
}

func (p *Postgres) RestoreHistory(historyId, dagId string) error {
	var snapshotJSON string
	err := p.pool.QueryRow(context.Background(), `
		SELECT snapshot_json FROM dag_history WHERE id = $1`, historyId).Scan(&snapshotJSON)
	if err != nil {
		return err
	}
	return p.UpdateDAG(dagId, models.Dag{Diagram: snapshotJSON})
}
