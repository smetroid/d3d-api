package controllers

import "testing"

func TestCountClusterElements(t *testing.T) {
	tests := []struct {
		name      string
		json      string
		wantNodes int
		wantEdges int
	}{
		{
			name:      "empty cluster",
			json:      `{"nodes":[],"edges":[]}`,
			wantNodes: 0,
			wantEdges: 0,
		},
		{
			name:      "two nodes no edges",
			json:      `{"nodes":[{"v":"n1"},{"v":"n2"}],"edges":[]}`,
			wantNodes: 2,
			wantEdges: 0,
		},
		{
			name:      "nodes and edges",
			json:      `{"nodes":[{"v":"n1"},{"v":"n2"}],"edges":[{"v":"n1","w":"n2"}]}`,
			wantNodes: 2,
			wantEdges: 1,
		},
		{
			name:      "invalid JSON returns zeros",
			json:      `not json`,
			wantNodes: 0,
			wantEdges: 0,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			n, e := countClusterElements(tc.json)
			if n != tc.wantNodes {
				t.Errorf("nodeCount: got %d, want %d", n, tc.wantNodes)
			}
			if e != tc.wantEdges {
				t.Errorf("edgeCount: got %d, want %d", e, tc.wantEdges)
			}
		})
	}
}

func TestDetectType(t *testing.T) {
	diagram := `{"nodes":[{"v":"n1","value":{}},{"v":"n2","value":{}}],"edges":[]}`

	if got := detectType(diagram, []string{"n1"}); got != "node" {
		t.Errorf("single node root: got %q, want %q", got, "node")
	}
	if got := detectType(diagram, []string{"n1", "n2"}); got != "cluster" {
		t.Errorf("multi-root: got %q, want %q", got, "cluster")
	}
	if got := detectType(diagram, []string{}); got != "cluster" {
		t.Errorf("empty roots: got %q, want %q", got, "cluster")
	}
	if got := detectType(diagram, []string{"missing"}); got != "node" {
		t.Errorf("unknown root: got %q, want %q", got, "node")
	}
}
