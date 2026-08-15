// Command migrate-rb2pg copies d3d-api data from RethinkDB into Postgres,
// preserving UUIDs and timestamps. It is idempotent: rows that already exist
// in Postgres (matched by primary key) are skipped, so it is safe to re-run.
//
// Usage:
//
//	migrate-rb2pg \
//	  -rb-address localhost:28015 \
//	  -rb-database samus \
//	  -pg-dsn postgres://user:pass@host:5432/samus
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/smetroid/d3d-api/app/db/postgres"
	r "gopkg.in/gorethink/gorethink.v4"
)

type tableSpec struct {
	rbTable   string
	pgTable   string
	conflict  string   // conflict target column (primary key)
	columns   []string // Postgres columns, in insert order
	parentKey string   // row key referencing a parent table (e.g. "dag_id"); "" = none
	parentIDs map[string]bool
	extract   func(row map[string]interface{}, warn func(string)) []interface{}
}

func main() {
	var rbAddr, rbDB, pgDSN string
	flag.StringVar(&rbAddr, "rb-address", "localhost:28015", "RethinkDB address (host:port)")
	flag.StringVar(&rbDB, "rb-database", "samus", "RethinkDB database")
	flag.StringVar(&pgDSN, "pg-dsn", "", "Postgres DSN (required)")
	flag.Parse()

	if pgDSN == "" {
		fmt.Fprintln(os.Stderr, "error: -pg-dsn is required")
		os.Exit(2)
	}

	ctx := context.Background()

	session, err := r.Connect(r.ConnectOpts{Address: rbAddr, Database: rbDB})
	if err != nil {
		log.Fatalf("rethinkdb connect: %v", err)
	}
	defer session.Close()

	p := &postgres.Postgres{DSN: pgDSN}
	if err := p.Init(); err != nil {
		log.Fatalf("postgres init: %v", err)
	}
	pool := p.Pool()
	if pool == nil {
		log.Fatal("postgres pool not open")
	}
	defer pool.Close()

	specs := []tableSpec{
		{
			rbTable: "dags", pgTable: "dags", conflict: "id",
			columns: []string{"id", "name", "description", "diagram", "created", "updated"},
			extract: func(row map[string]interface{}, warn func(string)) []interface{} {
				return []interface{}{
					row["id"], row["name"], row["description"],
					jsonText(row["diagram"], warn),
					timeVal(row["created"], warn), timeVal(row["updated"], warn),
				}
			},
		},
		{
			rbTable: "dag_history", pgTable: "dag_history", conflict: "id",
			parentKey: "dag_id",
			columns:   []string{"id", "dag_id", "snapshot_json", "saved_by", "saved_at"},
			extract: func(row map[string]interface{}, warn func(string)) []interface{} {
				return []interface{}{
					row["id"], row["dag_id"],
					jsonText(row["snapshot_json"], warn),
					row["saved_by"], timeVal(row["saved_at"], warn),
				}
			},
		},
		{
			rbTable: "shares", pgTable: "shares", conflict: "id",
			parentKey: "dag_id",
			columns:   []string{"id", "dag_id", "jti", "role", "anon_name", "created_by", "expires_at", "created_at"},
			extract: func(row map[string]interface{}, warn func(string)) []interface{} {
				return []interface{}{
					row["id"], row["dag_id"], row["jti"], row["role"], row["anon_name"],
					row["created_by"], timeVal(row["expires_at"], warn), timeVal(row["created_at"], warn),
				}
			},
		},
		{
			// RethinkDB stored the denylist under its default primary key "id".
			rbTable: "share_denylist", pgTable: "share_denylist", conflict: "jti",
			columns: []string{"jti", "revoked_at"},
			extract: func(row map[string]interface{}, warn func(string)) []interface{} {
				return []interface{}{row["id"], timeVal(row["revoked_at"], warn)}
			},
		},
		{
			rbTable: "users", pgTable: "users", conflict: "id",
			columns: []string{"id", "username", "password_hash", "created_at"},
			extract: func(row map[string]interface{}, warn func(string)) []interface{} {
				return []interface{}{
					row["id"], row["username"], row["password_hash"],
					timeVal(row["created_at"], warn),
				}
			},
		},
		{
			rbTable: "nodes", pgTable: "nodes", conflict: "id",
			columns: []string{"id", "v", "parent", "value_label", "value_type", "value_cluster_label_pos", "value_style", "created"},
			extract: func(row map[string]interface{}, warn func(string)) []interface{} {
				return []interface{}{
					row["id"], row["v"], row["parent"],
					jsonValue(row["valueLabel"], row["value_label"], warn),
					row["labelType"], row["clusterLabelPos"], row["clusterStyle"],
					timeVal(row["created"], warn),
				}
			},
		},
		{
			rbTable: "edges", pgTable: "edges", conflict: "id",
			columns: []string{"id", "v", "w", "label", "created"},
			extract: func(row map[string]interface{}, warn func(string)) []interface{} {
				return []interface{}{
					row["id"], row["v"], row["w"],
					jsonValue(row["label"], nil, warn),
					timeVal(row["created"], warn),
				}
			},
		},
		{
			rbTable: "menus", pgTable: "menus", conflict: "id",
			columns: []string{"id", "name", "parent", "options", "created"},
			extract: func(row map[string]interface{}, warn func(string)) []interface{} {
				return []interface{}{
					row["id"], row["name"], row["parent"], row["options"],
					timeVal(row["created"], warn),
				}
			},
		},
	}

	// Ensure the source tables exist before attempting the copy.
	tableList := listTables(session, rbDB)

	dagIDs := idSet(session, rbDB, "dags")
	for i := range specs {
		if specs[i].parentKey == "dag_id" {
			specs[i].parentIDs = dagIDs
		}
	}

	var totalRows, totalInserted, totalSkipped, totalOrphaned, totalFailed int
	for _, spec := range specs {
		if !contains(tableList, spec.rbTable) {
			log.Printf("%s: table does not exist in RethinkDB, skipping", spec.rbTable)
			continue
		}
		nRows, nInserted, nSkipped, nOrphaned, nFailed := copyTable(ctx, pool, session, rbDB, spec)
		log.Printf("%s: %d rows, %d inserted, %d already present, %d orphaned, %d failed",
			spec.rbTable, nRows, nInserted, nSkipped, nOrphaned, nFailed)
		totalRows += nRows
		totalInserted += nInserted
		totalSkipped += nSkipped
		totalOrphaned += nOrphaned
		totalFailed += nFailed
	}

	log.Printf("done: %d rows read, %d inserted, %d already present, %d orphaned, %d failed", totalRows, totalInserted, totalSkipped, totalOrphaned, totalFailed)
	if totalFailed > 0 {
		os.Exit(1)
	}
}

func copyTable(ctx context.Context, pool *pgxpool.Pool, session *r.Session, rbDB string, spec tableSpec) (rows, inserted, skipped, orphaned, failed int) {
	cursor, err := r.DB(rbDB).Table(spec.rbTable).Run(session)
	if err != nil {
		log.Printf("%s: read failed: %v", spec.rbTable, err)
		failed++
		return
	}
	defer cursor.Close()

	sql := fmt.Sprintf(
		"INSERT INTO %s (%s) VALUES (%s) ON CONFLICT (%s) DO NOTHING",
		spec.pgTable,
		strings.Join(spec.columns, ", "),
		placeholders(len(spec.columns)),
		spec.conflict,
	)

	var batch []map[string]interface{}
	if err := cursor.All(&batch); err != nil {
		log.Printf("%s: read failed: %v", spec.rbTable, err)
		failed++
		return
	}
	rows = len(batch)

	for _, row := range batch {
		if spec.parentKey != "" {
			if pid, ok := row[spec.parentKey].(string); !ok || !spec.parentIDs[pid] {
				orphaned++
				continue
			}
		}
		var warned []string
		warn := func(msg string) { warned = append(warned, msg) }
		vals := spec.extract(row, warn)
		if vals == nil {
			failed++
			continue
		}
		tag, err := pool.Exec(ctx, sql, vals...)
		if err != nil {
			log.Printf("%s: insert failed id=%v: %v", spec.rbTable, row["id"], err)
			failed++
			continue
		}
		if tag.RowsAffected() == 0 {
			skipped++
		} else {
			inserted++
		}
		for _, w := range warned {
			log.Printf("%s: warning id=%v: %s", spec.rbTable, row["id"], w)
		}
	}
	return
}

func listTables(session *r.Session, rbDB string) []string {
	cursor, err := r.DB(rbDB).TableList().Run(session)
	if err != nil {
		return nil
	}
	var tables []string
	_ = cursor.All(&tables)
	return tables
}

// idSet returns the set of primary keys of a table, used to detect rows whose
// parent no longer exists (orphans) before attempting the insert.
func idSet(session *r.Session, rbDB, table string) map[string]bool {
	set := map[string]bool{}
	cursor, err := r.DB(rbDB).Table(table).Pluck("id").Run(session)
	if err != nil {
		return set
	}
	var ids []map[string]string
	if err := cursor.All(&ids); err != nil {
		return set
	}
	for _, m := range ids {
		if id, ok := m["id"]; ok {
			set[id] = true
		}
	}
	return set
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

func placeholders(n int) string {
	parts := make([]string, n)
	for i := range parts {
		parts[i] = fmt.Sprintf("$%d", i+1)
	}
	return strings.Join(parts, ", ")
}

// timeVal converts a RethinkDB timestamp to a *time.Time, returning nil for
// zero/absent values so they store as NULL in Postgres.
func timeVal(v interface{}, warn func(string)) interface{} {
	switch t := v.(type) {
	case nil:
		return nil
	case time.Time:
		if t.IsZero() {
			return nil
		}
		return t
	case string:
		parsed, err := time.Parse(time.RFC3339, t)
		if err != nil {
			warn(fmt.Sprintf("unparseable timestamp %q", t))
			return nil
		}
		return parsed
	default:
		warn(fmt.Sprintf("unexpected timestamp type %T", v))
		return nil
	}
}

// jsonText converts a field stored as a JSON string into a value usable by the
// jsonb columns, normalizing key order and rejecting invalid JSON.
func jsonText(v interface{}, warn func(string)) interface{} {
	s, ok := v.(string)
	if !ok || strings.TrimSpace(s) == "" {
		return nil
	}
	var parsed interface{}
	if err := json.Unmarshal([]byte(s), &parsed); err != nil {
		warn(fmt.Sprintf("invalid JSON in field: %v", err))
		return nil
	}
	normalized, err := json.Marshal(parsed)
	if err != nil {
		warn(fmt.Sprintf("re-encode JSON: %v", err))
		return nil
	}
	return string(normalized)
}

// jsonValue converts a field stored either as a JSON object (map) or a JSON
// string into a value usable by the jsonb columns.
func jsonValue(v interface{}, fallback interface{}, warn func(string)) interface{} {
	if v == nil {
		v = fallback
	}
	switch val := v.(type) {
	case nil:
		return nil
	case string:
		return jsonText(val, warn)
	case map[string]interface{}:
		b, err := json.Marshal(val)
		if err != nil {
			warn(fmt.Sprintf("marshal object: %v", err))
			return nil
		}
		return string(b)
	default:
		b, err := json.Marshal(v)
		if err != nil {
			warn(fmt.Sprintf("marshal %T: %v", v, err))
			return nil
		}
		return string(b)
	}
}
