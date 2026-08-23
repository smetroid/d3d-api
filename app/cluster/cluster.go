// Package cluster computes graphlib subgraph closures for element sharing.
// It operates on the graphlib JSON format used by the dags table:
//
//	{"options":{…}, "nodes":[{"v","value","parent?"}], "edges":[{"v","w","value"}]}
package cluster

import "encoding/json"

// Node is a graphlib node record.
type Node struct {
	V      string                 `json:"v"`
	Value  map[string]interface{} `json:"value"`
	Parent string                 `json:"parent,omitempty"`
}

// Edge is a graphlib edge record.
type Edge struct {
	V     string                 `json:"v"` // source
	W     string                 `json:"w"` // target
	Value map[string]interface{} `json:"value"`
}

// Subgraph is the snapshot payload stored in element_shares.cluster.
type Subgraph struct {
	Nodes []Node `json:"nodes"`
	Edges []Edge `json:"edges"`
}

type graphIndex struct {
	nodeMap  map[string]Node     // v → Node
	children map[string][]string // parentV → []childV
	adjOut   map[string][]string // v → out-neighbour IDs
	adjIn    map[string][]string // v → in-neighbour IDs
	edges    []Edge
}

func buildIndex(nodes []Node, edges []Edge) graphIndex {
	idx := graphIndex{
		nodeMap:  make(map[string]Node, len(nodes)),
		children: make(map[string][]string),
		adjOut:   make(map[string][]string),
		adjIn:    make(map[string][]string),
		edges:    edges,
	}
	for _, n := range nodes {
		idx.nodeMap[n.V] = n
		if n.Parent != "" {
			idx.children[n.Parent] = append(idx.children[n.Parent], n.V)
		}
	}
	for _, e := range edges {
		idx.adjOut[e.V] = append(idx.adjOut[e.V], e.W)
		idx.adjIn[e.W] = append(idx.adjIn[e.W], e.V)
	}
	return idx
}

// addDescendants adds all children (recursively) of nodeId to included.
func addDescendants(nodeId string, idx graphIndex, included map[string]bool) {
	for _, child := range idx.children[nodeId] {
		if !included[child] {
			included[child] = true
			addDescendants(child, idx, included)
		}
	}
}

// addAncestors walks the parent chain upward, adding each container node.
// It does NOT add siblings — only the container nodes themselves.
func addAncestors(nodeId string, idx graphIndex, included map[string]bool) {
	node, ok := idx.nodeMap[nodeId]
	if !ok || node.Parent == "" {
		return
	}
	if !included[node.Parent] {
		included[node.Parent] = true
		addAncestors(node.Parent, idx, included)
	}
}

// Compute returns the subgraph closure for the given rootIds.
//
// depth 0  — root nodes + descendants (compound children, recursively)
// depth N  — + N hops of edge-connected neighbours and their descendants
// depth -1 — entire connected component(s) containing the roots
//
// In all cases: parent container nodes of any included node are also included,
// and only edges whose both endpoints are included are returned.
func Compute(diagramJSON string, rootIds []string, depth int) (Subgraph, error) {
	var wrapper struct {
		Nodes []Node `json:"nodes"`
		Edges []Edge `json:"edges"`
	}
	if err := json.Unmarshal([]byte(diagramJSON), &wrapper); err != nil {
		return Subgraph{}, err
	}

	idx := buildIndex(wrapper.Nodes, wrapper.Edges)
	included := make(map[string]bool, len(rootIds)*2)

	// Seed: roots + their compound descendants.
	for _, id := range rootIds {
		if _, ok := idx.nodeMap[id]; !ok {
			continue
		}
		included[id] = true
		addDescendants(id, idx, included)
	}

	if depth < 0 {
		// Whole connected component: undirected BFS.
		frontier := make([]string, 0, len(included))
		for id := range included {
			frontier = append(frontier, id)
		}
		for len(frontier) > 0 {
			cur := frontier[0]
			frontier = frontier[1:]
			for _, nb := range append(idx.adjOut[cur], idx.adjIn[cur]...) {
				if !included[nb] {
					included[nb] = true
					addDescendants(nb, idx, included)
					frontier = append(frontier, nb)
				}
			}
		}
	} else {
		// N-hop BFS (depth 0 skips this loop entirely).
		frontier := make([]string, 0, len(included))
		for id := range included {
			frontier = append(frontier, id)
		}
		for hop := 0; hop < depth; hop++ {
			next := []string{}
			for _, cur := range frontier {
				for _, nb := range append(idx.adjOut[cur], idx.adjIn[cur]...) {
					if !included[nb] {
						included[nb] = true
						addDescendants(nb, idx, included)
						next = append(next, nb)
					}
				}
			}
			frontier = next
		}
	}

	// Always include parent containers for compound-graph correctness.
	for id := range included {
		addAncestors(id, idx, included)
	}

	// Collect results in original insertion order.
	nodes := make([]Node, 0, len(included))
	for _, n := range wrapper.Nodes {
		if included[n.V] {
			nodes = append(nodes, n)
		}
	}
	edges := make([]Edge, 0)
	for _, e := range wrapper.Edges {
		if included[e.V] && included[e.W] {
			edges = append(edges, e)
		}
	}

	return Subgraph{Nodes: nodes, Edges: edges}, nil
}

// ValidateRootIds returns any rootIds that are not present in the diagram.
func ValidateRootIds(diagramJSON string, rootIds []string) ([]string, error) {
	var wrapper struct {
		Nodes []Node `json:"nodes"`
	}
	if err := json.Unmarshal([]byte(diagramJSON), &wrapper); err != nil {
		return nil, err
	}
	nodeSet := make(map[string]bool, len(wrapper.Nodes))
	for _, n := range wrapper.Nodes {
		nodeSet[n.V] = true
	}
	var missing []string
	for _, id := range rootIds {
		if !nodeSet[id] {
			missing = append(missing, id)
		}
	}
	return missing, nil
}
