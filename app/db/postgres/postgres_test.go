package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"reflect"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/smetroid/d3d-api/app/auth/socialauth"
	"github.com/smetroid/d3d-api/app/models"
)

// testDSNEnv selects the database used by the integration tests. Tests skip
// when it is unset so `go test ./...` stays green without a local Postgres.
const testDSNEnv = "TEST_DATABASE_URL"

// newTestPostgres opens the repository on TEST_DATABASE_URL, runs migrations,
// and truncates every table for a clean slate per test.
func newTestPostgres(t *testing.T) *Postgres {
	t.Helper()
	dsn := os.Getenv(testDSNEnv)
	if dsn == "" {
		t.Skipf("%s not set; skipping Postgres integration tests", testDSNEnv)
	}
	p := &Postgres{DSN: dsn}
	if err := p.Init(); err != nil {
		t.Fatalf("init: %v", err)
	}
	t.Cleanup(func() { p.Pool().Close() })
	truncateAll(t, p)
	return p
}

func truncateAll(t *testing.T, p *Postgres) {
	t.Helper()
	_, err := p.Pool().Exec(context.Background(), `
		TRUNCATE dag_history, shares, share_denylist, users, menus, edges, nodes, dags,
		         element_shares, group_members, user_groups, memberships, companies CASCADE`)
	if err != nil {
		t.Fatalf("truncate: %v", err)
	}
}

// jsonEqual compares two JSON strings semantically (ignoring key order and
// whitespace, which jsonb normalization does not guarantee).
func jsonEqual(t *testing.T, a, b string) bool {
	t.Helper()
	var va, vb interface{}
	if err := json.Unmarshal([]byte(a), &va); err != nil {
		t.Fatalf("invalid json %q: %v", a, err)
	}
	if err := json.Unmarshal([]byte(b), &vb); err != nil {
		t.Fatalf("invalid json %q: %v", b, err)
	}
	return reflect.DeepEqual(va, vb)
}

// ─── DAGs ────────────────────────────────────────────────────────────────────

func TestPostgres_DAGCRUD(t *testing.T) {
	p := newTestPostgres(t)

	now := time.Now().UTC().Truncate(time.Second)
	dag := models.Dag{
		Name:        "test-dag",
		Description: "a description",
		Diagram:     `{"nodes":[{"id":"a"}],"edges":[]}`,
		Created:     now,
		Updated:     now,
	}

	id, err := p.CreateDAG(dag)
	if err != nil {
		t.Fatal(err)
	}
	if id == "" {
		t.Fatal("expected a generated id")
	}

	got, err := p.GetDAG(id)
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != dag.Name || got.Description != dag.Description {
		t.Errorf("name/description mismatch: got %+v", got)
	}
	if !jsonEqual(t, got.Diagram, dag.Diagram) {
		t.Errorf("diagram mismatch: got %q want %q", got.Diagram, dag.Diagram)
	}
	if !got.Created.Equal(now) || !got.Updated.Equal(now) {
		t.Errorf("timestamps mismatch: created=%v updated=%v", got.Created, got.Updated)
	}

	// Partial update must not clobber the fields left zero.
	if err := p.UpdateDAG(id, models.Dag{Name: "renamed"}); err != nil {
		t.Fatal(err)
	}
	got, err = p.GetDAG(id)
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "renamed" {
		t.Errorf("name not updated: %q", got.Name)
	}
	if got.Description != dag.Description {
		t.Errorf("description clobbered: %q", got.Description)
	}
	if !jsonEqual(t, got.Diagram, dag.Diagram) {
		t.Errorf("diagram clobbered: %q", got.Diagram)
	}

	// FindRelatedDAG matches on name + description + semantic diagram.
	related, found, err := p.FindRelatedDAG(models.Dag{
		Name: "renamed", Description: dag.Description, Diagram: dag.Diagram,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !found || related.Id != id {
		t.Errorf("FindRelatedDAG: found=%v id=%q want %q", found, related.Id, id)
	}

	if err := p.DeleteDAG(id); err != nil {
		t.Fatal(err)
	}
	if _, err := p.GetDAG(id); err == nil {
		t.Error("GetDAG succeeded after delete")
	}
}

// A new DAG has an empty diagram and zero timestamps; those store as NULL and
// must round-trip without scan errors.
func TestPostgres_DAGEmptyFieldsRoundTrip(t *testing.T) {
	p := newTestPostgres(t)

	id, err := p.CreateDAG(models.Dag{Name: "empty"})
	if err != nil {
		t.Fatal(err)
	}
	got, err := p.GetDAG(id)
	if err != nil {
		t.Fatalf("GetDAG on empty fields: %v", err)
	}
	if !got.Created.IsZero() || !got.Updated.IsZero() {
		t.Errorf("expected zero timestamps, got created=%v updated=%v", got.Created, got.Updated)
	}

	// FindRelatedDAG must match rows whose diagram is NULL.
	_, found, err := p.FindRelatedDAG(models.Dag{Name: "empty"})
	if err != nil {
		t.Fatal(err)
	}
	if !found {
		t.Error("FindRelatedDAG did not match NULL-diagram row")
	}

	summary, err := p.GetDAGsSummary(nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(summary) != 1 {
		t.Errorf("GetDAGsSummary: got %d rows, want 1", len(summary))
	}
}

// ─── Nodes ───────────────────────────────────────────────────────────────────

func TestPostgres_NodeCRUD(t *testing.T) {
	p := newTestPostgres(t)

	now := time.Now().UTC().Truncate(time.Second)
	node := models.Node{
		V:                    "n1",
		Parent:               "root",
		ValueLabel:           map[string]string{"label": "hello"},
		ValueType:            "box",
		ValueClusterLabelPos: "c",
		ValueStyle:           "s",
		Created:              now,
	}

	id, err := p.CreateNode(node)
	if err != nil {
		t.Fatal(err)
	}
	if id == "" {
		t.Fatal("expected a generated id")
	}

	got, err := p.GetNode(id)
	if err != nil {
		t.Fatal(err)
	}
	if got.V != node.V || got.Parent != node.Parent || got.ValueType != node.ValueType {
		t.Errorf("scalar fields mismatch: got %+v", got)
	}
	if !reflect.DeepEqual(got.ValueLabel, node.ValueLabel) {
		t.Errorf("value_label mismatch: got %v want %v", got.ValueLabel, node.ValueLabel)
	}
	if !got.Created.Equal(now) {
		t.Errorf("created mismatch: %v", got.Created)
	}

	related, found, err := p.FindRelatedNode(node)
	if err != nil {
		t.Fatal(err)
	}
	if !found || related.Id != id {
		t.Errorf("FindRelatedNode: found=%v id=%q want %q", found, related.Id, id)
	}

	if err := p.DeleteNode(id); err != nil {
		t.Fatal(err)
	}
	if _, err := p.GetNode(id); err == nil {
		t.Error("GetNode succeeded after delete")
	}
}

// A node without a value_label stores NULL and must round-trip cleanly.
func TestPostgres_NodeEmptyLabelRoundTrip(t *testing.T) {
	p := newTestPostgres(t)

	id, err := p.CreateNode(models.Node{V: "bare"})
	if err != nil {
		t.Fatal(err)
	}
	got, err := p.GetNode(id)
	if err != nil {
		t.Fatalf("GetNode on NULL value_label: %v", err)
	}
	if got.ValueLabel != nil {
		t.Errorf("expected nil value_label, got %v", got.ValueLabel)
	}

	_, found, err := p.FindRelatedNode(models.Node{V: "bare"})
	if err != nil {
		t.Fatal(err)
	}
	if !found {
		t.Error("FindRelatedNode did not match NULL-label row")
	}
}

// ─── Edges ───────────────────────────────────────────────────────────────────

func TestPostgres_EdgeCRUD(t *testing.T) {
	p := newTestPostgres(t)

	now := time.Now().UTC().Truncate(time.Second)
	edge := models.Edge{
		V:       "a",
		W:       "b",
		Label:   map[string]string{"label": "edge-label"},
		Created: now,
	}

	id, err := p.CreateEdge(edge)
	if err != nil {
		t.Fatal(err)
	}
	if id == "" {
		t.Fatal("expected a generated id")
	}

	got, err := p.GetEdge(id)
	if err != nil {
		t.Fatal(err)
	}
	if got.V != edge.V || got.W != edge.W {
		t.Errorf("v/w mismatch: got %+v", got)
	}
	if !reflect.DeepEqual(got.Label, edge.Label) {
		t.Errorf("label mismatch: got %v want %v", got.Label, edge.Label)
	}
	if !got.Created.Equal(now) {
		t.Errorf("created mismatch: %v", got.Created)
	}

	related, found, err := p.FindRelatedEdge(edge)
	if err != nil {
		t.Fatal(err)
	}
	if !found || related.Id != id {
		t.Errorf("FindRelatedEdge: found=%v id=%q want %q", found, related.Id, id)
	}

	if err := p.DeleteEdge(id); err != nil {
		t.Fatal(err)
	}
	if _, err := p.GetEdge(id); err == nil {
		t.Error("GetEdge succeeded after delete")
	}
}

func TestPostgres_EdgeEmptyLabelRoundTrip(t *testing.T) {
	p := newTestPostgres(t)

	id, err := p.CreateEdge(models.Edge{V: "a", W: "b"})
	if err != nil {
		t.Fatal(err)
	}
	got, err := p.GetEdge(id)
	if err != nil {
		t.Fatalf("GetEdge on NULL label: %v", err)
	}
	if got.Label != nil {
		t.Errorf("expected nil label, got %v", got.Label)
	}

	_, found, err := p.FindRelatedEdge(models.Edge{V: "a", W: "b"})
	if err != nil {
		t.Fatal(err)
	}
	if !found {
		t.Error("FindRelatedEdge did not match NULL-label row")
	}
}

// ─── Menus ───────────────────────────────────────────────────────────────────

func TestPostgres_MenuCRUD(t *testing.T) {
	p := newTestPostgres(t)

	now := time.Now().UTC().Truncate(time.Second)
	menu := models.Menu{Name: "file", Parent: "root", Options: "opts", Created: now}

	id, err := p.CreateMenu(menu)
	if err != nil {
		t.Fatal(err)
	}
	if id == "" {
		t.Fatal("expected a generated id")
	}

	got, err := p.GetMenu(id)
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != menu.Name || got.Parent != menu.Parent || got.Options != menu.Options {
		t.Errorf("menu mismatch: got %+v", got)
	}

	if err := p.UpdateMenu(id, models.Menu{Name: "edit"}); err != nil {
		t.Fatal(err)
	}
	got, err = p.GetMenu(id)
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "edit" || got.Parent != "root" {
		t.Errorf("partial update wrong: got %+v", got)
	}

	related, found, err := p.FindRelatedMenu(models.Menu{Parent: "root", Options: "opts"})
	if err != nil {
		t.Fatal(err)
	}
	if !found || related.Id != id {
		t.Errorf("FindRelatedMenu: found=%v id=%q want %q", found, related.Id, id)
	}

	options, err := p.GetMenusOptions(nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := options[id]; !ok {
		t.Error("GetMenusOptions missing created menu")
	}

	if err := p.DeleteMenu(id); err != nil {
		t.Fatal(err)
	}
	if _, err := p.GetMenu(id); err == nil {
		t.Error("GetMenu succeeded after delete")
	}
}

// ─── History ─────────────────────────────────────────────────────────────────

func TestPostgres_HistoryOrderingAndPrune(t *testing.T) {
	p := newTestPostgres(t)

	dagID, err := p.CreateDAG(models.Dag{Name: "hist"})
	if err != nil {
		t.Fatal(err)
	}

	const total = historyLimit + 10 // 60 snapshots, well past the 50 cap
	for i := 0; i < total; i++ {
		if err := p.AppendHistory(dagID, fmt.Sprintf(`{"n":%d}`, i), "tester"); err != nil {
			t.Fatal(err)
		}
		time.Sleep(time.Millisecond) // distinct saved_at so ordering is deterministic
	}

	// pruneHistory runs in a background goroutine from AppendHistory; call it
	// synchronously here so the test is deterministic.
	p.pruneHistory(dagID)

	hist, err := p.GetHistory(dagID)
	if err != nil {
		t.Fatal(err)
	}
	if len(hist) != historyLimit {
		t.Fatalf("GetHistory: got %d entries, want %d", len(hist), historyLimit)
	}

	for i := 1; i < len(hist); i++ {
		if hist[i-1].SavedAt.Before(hist[i].SavedAt) {
			t.Error("history is not newest-first")
		}
	}
	if n := snapshotN(t, hist[0].SnapshotJSON); n != total-1 {
		t.Errorf("newest snapshot missing from head: n=%d want %d (%q)", n, total-1, hist[0].SnapshotJSON)
	}
	if n := snapshotN(t, hist[len(hist)-1].SnapshotJSON); n != total-historyLimit {
		t.Errorf("expected %d to survive the prune, got n=%d (%q)",
			total-historyLimit, n, hist[len(hist)-1].SnapshotJSON)
	}
}

// snapshotN decodes {"n": <int>} snapshots produced by the prune test.
func snapshotN(t *testing.T, s string) int {
	t.Helper()
	var v struct {
		N int `json:"n"`
	}
	if err := json.Unmarshal([]byte(s), &v); err != nil {
		t.Fatalf("unexpected snapshot %q: %v", s, err)
	}
	return v.N
}

func TestPostgres_RestoreHistory(t *testing.T) {
	p := newTestPostgres(t)

	dagID, err := p.CreateDAG(models.Dag{Name: "restore", Diagram: `{"v":0}`})
	if err != nil {
		t.Fatal(err)
	}
	if err := p.AppendHistory(dagID, `{"v":1,"op":"set"}`, "tester"); err != nil {
		t.Fatal(err)
	}
	p.pruneHistory(dagID)

	hist, err := p.GetHistory(dagID)
	if err != nil {
		t.Fatal(err)
	}
	if len(hist) != 1 {
		t.Fatalf("want 1 history entry, got %d", len(hist))
	}

	if err := p.RestoreHistory(hist[0].Id, dagID); err != nil {
		t.Fatal(err)
	}
	dag, err := p.GetDAG(dagID)
	if err != nil {
		t.Fatal(err)
	}
	if !jsonEqual(t, dag.Diagram, `{"v":1,"op":"set"}`) {
		t.Errorf("diagram not restored: %q", dag.Diagram)
	}
}

// History is per-DAG: snapshots for one DAG must not leak into another.
func TestPostgres_HistoryIsolation(t *testing.T) {
	p := newTestPostgres(t)

	dagA, _ := p.CreateDAG(models.Dag{Name: "a"})
	dagB, _ := p.CreateDAG(models.Dag{Name: "b"})
	if err := p.AppendHistory(dagA, `{"a":1}`, "u"); err != nil {
		t.Fatal(err)
	}

	histA, err := p.GetHistory(dagA)
	if err != nil {
		t.Fatal(err)
	}
	histB, err := p.GetHistory(dagB)
	if err != nil {
		t.Fatal(err)
	}
	if len(histA) != 1 || len(histB) != 0 {
		t.Errorf("history leaked across DAGs: a=%d b=%d", len(histA), len(histB))
	}
}

// AppendHistory with an empty snapshot stores NULL and must still read back.
func TestPostgres_HistoryEmptySnapshotRoundTrip(t *testing.T) {
	p := newTestPostgres(t)

	dagID, err := p.CreateDAG(models.Dag{Name: "empty-snap"})
	if err != nil {
		t.Fatal(err)
	}
	if err := p.AppendHistory(dagID, "", "tester"); err != nil {
		t.Fatal(err)
	}
	hist, err := p.GetHistory(dagID)
	if err != nil {
		t.Fatalf("GetHistory on NULL snapshot: %v", err)
	}
	if len(hist) != 1 || hist[0].SnapshotJSON != "" {
		t.Errorf("expected one empty snapshot, got %d entries (%q)", len(hist), hist[0].SnapshotJSON)
	}
}

// ─── Shares & denylist ───────────────────────────────────────────────────────

func TestPostgres_ShareCRUD(t *testing.T) {
	p := newTestPostgres(t)

	dagID, err := p.CreateDAG(models.Dag{Name: "shared"})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Truncate(time.Second)
	share := models.Share{
		DagId:     dagID,
		Jti:       uuid.New().String(),
		Role:      "edit",
		AnonName:  "Teal Fox",
		CreatedBy: "admin",
		ExpiresAt: now.Add(7 * 24 * time.Hour),
		CreatedAt: now,
	}
	if err := p.CreateShare(share); err != nil {
		t.Fatal(err)
	}

	got, err := p.GetShareByJti(share.Jti)
	if err != nil {
		t.Fatal(err)
	}
	if got.DagId != dagID || got.Role != "edit" || got.AnonName != "Teal Fox" || got.CreatedBy != "admin" {
		t.Errorf("share mismatch: got %+v", got)
	}
	if !got.ExpiresAt.Equal(share.ExpiresAt) || !got.CreatedAt.Equal(share.CreatedAt) {
		t.Errorf("share timestamps mismatch: got %+v", got)
	}

	// Deleting the DAG cascades its shares.
	if err := p.DeleteDAG(dagID); err != nil {
		t.Fatal(err)
	}
	if _, err := p.GetShareByJti(share.Jti); err == nil {
		t.Error("GetShareByJti succeeded after DAG delete (cascade missing?)")
	}
}

func TestPostgres_ShareDenylist(t *testing.T) {
	p := newTestPostgres(t)

	jti := uuid.New().String()
	revoked, err := p.IsRevoked(jti)
	if err != nil {
		t.Fatal(err)
	}
	if revoked {
		t.Fatal("jti reported revoked before any RevokeShare")
	}

	if err := p.RevokeShare(jti); err != nil {
		t.Fatal(err)
	}
	revoked, err = p.IsRevoked(jti)
	if err != nil {
		t.Fatal(err)
	}
	if !revoked {
		t.Fatal("jti not revoked after RevokeShare")
	}

	// Re-revoking is idempotent.
	if err := p.RevokeShare(jti); err != nil {
		t.Fatal(err)
	}
	revoked, err = p.IsRevoked(jti)
	if err != nil {
		t.Fatal(err)
	}
	if !revoked {
		t.Fatal("jti lost its revoked state after re-revoke")
	}

	other := uuid.New().String()
	revoked, err = p.IsRevoked(other)
	if err != nil {
		t.Fatal(err)
	}
	if revoked {
		t.Fatal("unrelated jti reported revoked")
	}
}

// ─── Users ───────────────────────────────────────────────────────────────────

func TestPostgres_UserCRUD(t *testing.T) {
	p := newTestPostgres(t)

	now := time.Now().UTC().Truncate(time.Second)
	user := models.User{Username: "alice", PasswordHash: "bcrypt-hash", CreatedAt: now}
	if err := p.CreateUser(user); err != nil {
		t.Fatal(err)
	}

	got, err := p.GetUser("alice")
	if err != nil {
		t.Fatal(err)
	}
	if got.Username != "alice" || got.PasswordHash != "bcrypt-hash" {
		t.Errorf("user mismatch: got %+v", got)
	}
	if !got.CreatedAt.Equal(now) {
		t.Errorf("created_at mismatch: %v", got.CreatedAt)
	}

	// Unknown users return a zero value, not an error.
	missing, err := p.GetUser("nobody")
	if err != nil {
		t.Fatal(err)
	}
	if missing.Username != "" {
		t.Errorf("expected empty user for unknown name, got %+v", missing)
	}

	// Username is unique.
	if err := p.CreateUser(models.User{Username: "alice", PasswordHash: "other"}); err == nil {
		t.Error("expected unique constraint violation for duplicate username")
	}
}

// ─── Element Shares ──────────────────────────────────────────────────────────

func TestPostgres_ListInboxShares(t *testing.T) {
	p := newTestPostgres(t)

	now := time.Now().UTC().Truncate(time.Second)

	makeShare := func(kind string, audienceIds []string, createdBy string, revoked bool, expiresAt time.Time) string {
		t.Helper()
		s := models.ElementShare{
			Type:         "node",
			RootIds:      []string{"n1"},
			Cluster:      `{"nodes":[],"edges":[]}`,
			AudienceKind: kind,
			AudienceIds:  audienceIds,
			Role:         "view",
			CreatedBy:    createdBy,
			Revoked:      revoked,
			Tags:         []string{},
			ImportedBy:   []string{},
			CreatedAt:    now,
			ExpiresAt:    expiresAt,
		}
		id, err := p.CreateElementShare(s)
		if err != nil {
			t.Fatalf("CreateElementShare: %v", err)
		}
		return id
	}

	// Set up company and group that alice belongs to.
	coID, err := p.CreateCompany(models.Company{Name: "acme", CreatedBy: "admin"})
	if err != nil {
		t.Fatal(err)
	}
	if err := p.AddMembership(models.Membership{UserId: "alice", CompanyId: coID}); err != nil {
		t.Fatal(err)
	}
	grpID, err := p.CreateGroup(models.Group{Name: "eng", CompanyId: coID})
	if err != nil {
		t.Fatal(err)
	}
	if err := p.AddGroupMember(models.GroupMember{GroupId: grpID, UserId: "alice"}); err != nil {
		t.Fatal(err)
	}

	aliceCompanyIds := []string{coID}
	aliceGroupIds := []string{grpID}

	// Shares that MUST appear in alice's inbox.
	wantUserFromBob := makeShare("user", []string{"alice"}, "bob", false, time.Time{})
	wantUserSelf := makeShare("user", []string{"alice"}, "alice", false, time.Time{}) // "Me" self-share
	wantCompany := makeShare("company", []string{coID}, "bob", false, time.Time{})
	wantGroup := makeShare("group", []string{grpID}, "bob", false, time.Time{})

	// Shares that must NOT appear.
	makeShare("public", nil, "bob", false, time.Time{})                // public → catalog only
	makeShare("user", []string{"carol"}, "bob", false, time.Time{})    // different user
	makeShare("company", []string{"other"}, "bob", false, time.Time{}) // different company
	makeShare("group", []string{"other"}, "bob", false, time.Time{})   // different group
	makeShare("company", []string{coID}, "alice", false, time.Time{})  // own company broadcast
	makeShare("group", []string{grpID}, "alice", false, time.Time{})   // own group broadcast
	makeShare("public", nil, "bob", true, time.Time{})                 // revoked
	makeShare("public", nil, "bob", false, now.Add(-time.Hour))        // expired

	shares, err := p.ListInboxShares("alice", aliceCompanyIds, aliceGroupIds)
	if err != nil {
		t.Fatal(err)
	}

	got := make(map[string]bool, len(shares))
	for _, s := range shares {
		got[s.Id] = true
	}

	for _, wantID := range []string{wantUserFromBob, wantUserSelf, wantCompany, wantGroup} {
		if !got[wantID] {
			t.Errorf("share %s missing from inbox", wantID)
		}
	}
	if len(shares) != 4 {
		t.Errorf("expected 4 inbox shares, got %d", len(shares))
	}
}

// ListInboxShares with empty company/group slices must not return company- or
// group-scoped shares, and must not error.
func TestPostgres_ListInboxShares_NoMemberships(t *testing.T) {
	p := newTestPostgres(t)

	now := time.Now().UTC().Truncate(time.Second)
	makeShare := func(kind string, audienceIds []string) string {
		t.Helper()
		id, err := p.CreateElementShare(models.ElementShare{
			Type: "node", RootIds: []string{"n1"}, Cluster: `{"nodes":[],"edges":[]}`,
			AudienceKind: kind, AudienceIds: audienceIds,
			Role: "view", CreatedBy: "bob",
			Tags: []string{}, ImportedBy: []string{}, CreatedAt: now,
		})
		if err != nil {
			t.Fatalf("CreateElementShare: %v", err)
		}
		return id
	}

	makeShare("public", nil)
	makeShare("company", []string{"some-co"})
	makeShare("group", []string{"some-grp"})
	userID := makeShare("user", []string{"alice"})

	shares, err := p.ListInboxShares("alice", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(shares) != 1 || shares[0].Id != userID {
		t.Errorf("expected only the user-targeted share, got %d shares", len(shares))
	}
}

func TestPostgres_ListCatalogShares(t *testing.T) {
	p := newTestPostgres(t)

	now := time.Now().UTC().Truncate(time.Second)
	future := now.Add(24 * time.Hour)

	makeShare := func(catalog bool, audienceKind string, revoked bool, expiresAt time.Time, title string) string {
		t.Helper()
		id, err := p.CreateElementShare(models.ElementShare{
			Type:         "node",
			Title:        title,
			RootIds:      []string{"n1"},
			Cluster:      `{"nodes":[{"v":"n1","value":{}}],"edges":[]}`,
			AudienceKind: audienceKind,
			AudienceIds:  []string{},
			Role:         "view",
			CreatedBy:    "alice",
			Catalog:      catalog,
			Revoked:      revoked,
			Tags:         []string{},
			ImportedBy:   []string{},
			CreatedAt:    now,
			ExpiresAt:    expiresAt,
		})
		if err != nil {
			t.Fatalf("CreateElementShare: %v", err)
		}
		return id
	}

	wantID := makeShare(true, "public", false, future, "Auth cluster")
	makeShare(false, "public", false, future, "")             // catalog=false → excluded
	makeShare(true, "public", true, future, "")               // revoked → excluded
	makeShare(true, "public", false, now.Add(-time.Hour), "") // expired → excluded
	makeShare(true, "user", false, future, "")                // non-public → excluded

	rows, err := p.ListCatalogShares(10)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 catalog row, got %d", len(rows))
	}
	if rows[0].Id != wantID {
		t.Errorf("expected catalog row %s, got %s", wantID, rows[0].Id)
	}
	if rows[0].Title != "Auth cluster" {
		t.Errorf("expected title 'Auth cluster', got %q", rows[0].Title)
	}
}

func TestPostgres_GetElementShare_Title(t *testing.T) {
	p := newTestPostgres(t)

	id, err := p.CreateElementShare(models.ElementShare{
		Type:         "node",
		Title:        "My titled share",
		RootIds:      []string{"n1"},
		Cluster:      `{"nodes":[],"edges":[]}`,
		AudienceKind: "public",
		AudienceIds:  []string{},
		Role:         "view",
		CreatedBy:    "alice",
		Tags:         []string{},
		ImportedBy:   []string{},
		CreatedAt:    time.Now().UTC().Truncate(time.Second),
	})
	if err != nil {
		t.Fatalf("CreateElementShare: %v", err)
	}

	got, err := p.GetElementShare(id)
	if err != nil {
		t.Fatalf("GetElementShare: %v", err)
	}
	if got.Title != "My titled share" {
		t.Errorf("expected title %q, got %q", "My titled share", got.Title)
	}
}

func TestGetUserReturnsLocalProviderDefaults(t *testing.T) {
	p := newTestPostgres(t)

	u := models.User{
		Id:           uuid.New().String(),
		Username:     "alice",
		PasswordHash: "hash",
		CreatedAt:    time.Now().UTC().Truncate(time.Second),
	}
	if err := p.CreateUser(u); err != nil {
		t.Fatalf("create: %v", err)
	}

	got, err := p.GetUser("alice")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Provider != "local" {
		t.Errorf("Provider = %q, want %q", got.Provider, "local")
	}
	if got.ProviderID != "" || got.Email != "" || got.DisplayName != "" {
		t.Errorf("expected empty social fields, got %+v", got)
	}
}

func TestUpsertSocialUserCreatesThenUpdates(t *testing.T) {
	p := newTestPostgres(t)

	profile := socialauth.SocialUserProfile{
		Provider:    "github",
		ProviderID:  "583231",
		Email:       "me@example.com",
		DisplayName: "Enrique Carranco",
		Username:    "smetroid",
	}

	created, err := p.UpsertSocialUser(profile)
	if err != nil {
		t.Fatalf("first upsert: %v", err)
	}
	if created.Username != "github:smetroid" {
		t.Errorf("Username = %q, want %q", created.Username, "github:smetroid")
	}
	if created.Id == "" {
		t.Error("expected a generated id")
	}

	// A second login must return the same row with refreshed profile data.
	profile.DisplayName = "E. Carranco"
	profile.Email = "new@example.com"
	updated, err := p.UpsertSocialUser(profile)
	if err != nil {
		t.Fatalf("second upsert: %v", err)
	}
	if updated.Id != created.Id {
		t.Errorf("id changed on re-login: %q -> %q", created.Id, updated.Id)
	}
	if updated.DisplayName != "E. Carranco" || updated.Email != "new@example.com" {
		t.Errorf("profile not refreshed: %+v", updated)
	}
}

func TestUpsertSocialUserDoesNotCollideWithLocalAccount(t *testing.T) {
	p := newTestPostgres(t)

	// A local account already owns the bare handle.
	local := models.User{
		Id:           uuid.New().String(),
		Username:     "smetroid",
		PasswordHash: "hash",
		CreatedAt:    time.Now().UTC().Truncate(time.Second),
	}
	if err := p.CreateUser(local); err != nil {
		t.Fatalf("create local: %v", err)
	}

	social, err := p.UpsertSocialUser(socialauth.SocialUserProfile{
		Provider:   "github",
		ProviderID: "583231",
		Username:   "smetroid",
	})
	if err != nil {
		t.Fatalf("upsert must not collide with the local account: %v", err)
	}
	if social.Id == local.Id {
		t.Fatal("social login must never adopt an existing local account")
	}
}

// I5 regression: a GitHub user who renames must not permanently 500-lock a
// later signer who claims their now-available old handle. UpsertSocialUser's
// DO UPDATE must keep the stored username in sync with the provider, so the
// old `provider:handle` is freed as soon as the renamed user logs back in.
func TestUpsertSocialUserRenameFreesUsernameForReuse(t *testing.T) {
	p := newTestPostgres(t)

	original, err := p.UpsertSocialUser(socialauth.SocialUserProfile{
		Provider:   "github",
		ProviderID: "1",
		Username:   "alice",
	})
	if err != nil {
		t.Fatalf("initial upsert: %v", err)
	}
	if original.Username != "github:alice" {
		t.Fatalf("Username = %q, want %q", original.Username, "github:alice")
	}

	// The original owner renames on GitHub and logs back in: same
	// provider_id, new handle. Without the fix, username is never updated by
	// the ON CONFLICT DO UPDATE, so "github:alice" stays permanently claimed.
	renamed, err := p.UpsertSocialUser(socialauth.SocialUserProfile{
		Provider:   "github",
		ProviderID: "1",
		Username:   "bob",
	})
	if err != nil {
		t.Fatalf("rename upsert: %v", err)
	}
	if renamed.Id != original.Id {
		t.Fatalf("rename must update the existing row, got a new id: %q -> %q", original.Id, renamed.Id)
	}
	if renamed.Username != "github:bob" {
		t.Fatalf("Username = %q, want %q after rename", renamed.Username, "github:bob")
	}

	// A different person now claims the freed-up "alice" handle. This is a
	// genuinely new account (different provider_id) and must succeed, not
	// collide with the stale username the original owner left behind.
	newClaimant, err := p.UpsertSocialUser(socialauth.SocialUserProfile{
		Provider:   "github",
		ProviderID: "2",
		Username:   "alice",
	})
	if err != nil {
		t.Fatalf("reuse of freed username must succeed, got: %v", err)
	}
	if newClaimant.Id == renamed.Id {
		t.Fatal("the new claimant must be a distinct account from the renamed original")
	}
	if newClaimant.Username != "github:alice" {
		t.Fatalf("Username = %q, want %q", newClaimant.Username, "github:alice")
	}
}

// I5 regression: when the username collision is genuine — a different
// provider_id whose desired username is still actively held by another row
// — UpsertSocialUser must return a distinct, recognizable error rather than
// letting the underlying unique-constraint violation surface as an opaque
// failure the caller can't act on.
func TestUpsertSocialUserGenuineUsernameCollisionReturnsDistinctError(t *testing.T) {
	p := newTestPostgres(t)

	holder, err := p.UpsertSocialUser(socialauth.SocialUserProfile{
		Provider:   "github",
		ProviderID: "1",
		Username:   "alice",
	})
	if err != nil {
		t.Fatalf("initial upsert: %v", err)
	}

	_, err = p.UpsertSocialUser(socialauth.SocialUserProfile{
		Provider:   "github",
		ProviderID: "2",
		Username:   "alice",
	})
	if !errors.Is(err, ErrUsernameTaken) {
		t.Fatalf("err = %v, want ErrUsernameTaken", err)
	}

	// The collision must not have mutated the existing holder's row.
	still, getErr := p.GetUserByProvider("github", "1")
	if getErr != nil {
		t.Fatalf("get holder: %v", getErr)
	}
	if still.Id != holder.Id || still.Username != "github:alice" {
		t.Fatalf("holder row changed after failed collision: %+v", still)
	}
}

func TestGetUserByProviderMissingReturnsZero(t *testing.T) {
	p := newTestPostgres(t)

	got, err := p.GetUserByProvider("github", "nobody")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Id != "" {
		t.Errorf("expected zero User, got %+v", got)
	}
}
