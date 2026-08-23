package cluster_test

import (
	"encoding/json"
	"testing"

	"github.com/smetroid/d3d-api/app/cluster"
)

func buildJSON(nodes []cluster.Node, edges []cluster.Edge) string {
	b, _ := json.Marshal(map[string]interface{}{"nodes": nodes, "edges": edges})
	return string(b)
}

func nodeIDs(sg cluster.Subgraph) map[string]bool {
	m := make(map[string]bool, len(sg.Nodes))
	for _, n := range sg.Nodes {
		m[n.V] = true
	}
	return m
}

func edgeKeys(sg cluster.Subgraph) map[string]bool {
	m := make(map[string]bool, len(sg.Edges))
	for _, e := range sg.Edges {
		m[e.V+"→"+e.W] = true
	}
	return m
}

// testGraph:
//   root (compound parent of child1, child2)
//   root → n1 → n2 → n3
//   isolated  (disconnected)
var (
	testNodes = []cluster.Node{
		{V: "root"},
		{V: "child1", Parent: "root"},
		{V: "child2", Parent: "root"},
		{V: "n1"},
		{V: "n2"},
		{V: "n3"},
		{V: "isolated"},
	}
	testEdges = []cluster.Edge{
		{V: "root", W: "n1"},
		{V: "n1", W: "n2"},
		{V: "n2", W: "n3"},
	}
	testDiagram = buildJSON(testNodes, testEdges)
)

func TestCompute_Depth0_CompoundDescendants(t *testing.T) {
	sg, err := cluster.Compute(testDiagram, []string{"root"}, 0)
	if err != nil {
		t.Fatal(err)
	}
	ids := nodeIDs(sg)
	for _, want := range []string{"root", "child1", "child2"} {
		if !ids[want] {
			t.Errorf("depth 0: expected %q in subgraph", want)
		}
	}
	for _, noWant := range []string{"n1", "n2", "n3", "isolated"} {
		if ids[noWant] {
			t.Errorf("depth 0: unexpected %q in subgraph", noWant)
		}
	}
	// root→n1 is filtered out because n1 is not included.
	if len(sg.Edges) != 0 {
		t.Errorf("depth 0: expected 0 edges, got %d", len(sg.Edges))
	}
}

func TestCompute_Depth1_OneHopNeighbours(t *testing.T) {
	sg, err := cluster.Compute(testDiagram, []string{"n1"}, 1)
	if err != nil {
		t.Fatal(err)
	}
	ids := nodeIDs(sg)
	// n1 (seed), root (in-neighbor + compound parent of child1/child2), n2 (out-neighbor)
	for _, want := range []string{"n1", "root", "child1", "child2", "n2"} {
		if !ids[want] {
			t.Errorf("depth 1: expected %q in subgraph", want)
		}
	}
	if ids["n3"] {
		t.Error("depth 1: n3 should not be included (2 hops away)")
	}
	eks := edgeKeys(sg)
	for _, want := range []string{"root→n1", "n1→n2"} {
		if !eks[want] {
			t.Errorf("depth 1: expected edge %q", want)
		}
	}
}

func TestCompute_DepthMinus1_FullComponent(t *testing.T) {
	sg, err := cluster.Compute(testDiagram, []string{"root"}, -1)
	if err != nil {
		t.Fatal(err)
	}
	ids := nodeIDs(sg)
	for _, want := range []string{"root", "child1", "child2", "n1", "n2", "n3"} {
		if !ids[want] {
			t.Errorf("depth -1: expected %q in subgraph", want)
		}
	}
	if ids["isolated"] {
		t.Error("depth -1: isolated is in a separate component and should not be included")
	}
	eks := edgeKeys(sg)
	for _, want := range []string{"root→n1", "n1→n2", "n2→n3"} {
		if !eks[want] {
			t.Errorf("depth -1: expected edge %q", want)
		}
	}
}

func TestCompute_CompoundParentIncluded(t *testing.T) {
	// Seeding from a child: its parent container must be included for compound-graph correctness.
	sg, err := cluster.Compute(testDiagram, []string{"child1"}, 0)
	if err != nil {
		t.Fatal(err)
	}
	ids := nodeIDs(sg)
	if !ids["child1"] {
		t.Error("child1 (seed) missing")
	}
	if !ids["root"] {
		t.Error("parent container 'root' should be included")
	}
	// Sibling child2 is not a descendant of child1, so it should not appear.
	if ids["child2"] {
		t.Error("child2 (sibling) should not be included at depth 0")
	}
}

func TestCompute_MissingRootIdIgnored(t *testing.T) {
	sg, err := cluster.Compute(testDiagram, []string{"does-not-exist"}, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(sg.Nodes) != 0 || len(sg.Edges) != 0 {
		t.Errorf("missing root should yield empty subgraph, got %+v", sg)
	}
}

func TestCompute_InvalidJSON(t *testing.T) {
	_, err := cluster.Compute("{bad json", []string{"n1"}, 0)
	if err == nil {
		t.Error("expected error on invalid JSON")
	}
}

func TestValidateRootIds_ReportsMissing(t *testing.T) {
	missing, err := cluster.ValidateRootIds(testDiagram, []string{"root", "n1", "ghost"})
	if err != nil {
		t.Fatal(err)
	}
	if len(missing) != 1 || missing[0] != "ghost" {
		t.Errorf("expected [ghost], got %v", missing)
	}
}

func TestValidateRootIds_AllPresent(t *testing.T) {
	missing, err := cluster.ValidateRootIds(testDiagram, []string{"root", "n1"})
	if err != nil {
		t.Fatal(err)
	}
	if len(missing) != 0 {
		t.Errorf("expected no missing IDs, got %v", missing)
	}
}

func TestValidateRootIds_InvalidJSON(t *testing.T) {
	_, err := cluster.ValidateRootIds("{bad json", []string{"n1"})
	if err == nil {
		t.Error("expected error on invalid JSON")
	}
}
