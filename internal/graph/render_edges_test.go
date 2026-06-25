package graph

import (
	"testing"

	"graph/internal/models"
)

// Covers: action splitting, L7 fold + dedup, edge level inference. One
// scenario exercises:
//   - allow + deny rules on the same (src,dst,direction,policy) tuple
//     produce TWO edges (action is part of the fold key)
//   - L7 fan-out (same L7 across multiple port rules) collapses to one
//     L7Match entry on the edge (dedup)
//   - action stamped on the edge data so frontend can color-code
func TestRenderEdges_ActionSplitAndL7Dedup(t *testing.T) {
	nodes := []models.WorkloadNode{
		{ID: "src"},
		{ID: "dst"},
	}

	l7 := &models.L7Match{Methods: []string{"GET"}, Paths: []string{"/api"}}
	ref := models.PolicyRef{Source: "istio", Name: "p1", Namespace: "ns-a", RuleIndex: 0}

	// Two allow rules sharing the same L7 (fan-out per port) — should fold
	// into one edge with ports {8080, 9090} and a single L7Match entry.
	allow8080 := models.Rule{SrcID: "src", DstID: "dst", Ports: []models.Port{{Port: 8080, Protocol: "TCP"}}, Direction: models.DirectionIngress, Contributor: ref, L7Match: l7, Action: models.ActionAllow}
	allow9090 := allow8080
	allow9090.Ports = []models.Port{{Port: 9090, Protocol: "TCP"}}

	// Same (src,dst,direction,policy) but action=deny — must NOT merge with
	// the allow group; produces a separate edge.
	denyRule := allow8080
	denyRule.Action = models.ActionDeny
	denyRule.L7Match = nil

	edges := RenderEdges([]models.Rule{allow8080, allow9090, denyRule}, nodes)

	if len(edges) != 2 {
		t.Fatalf("got %d edges, want 2 (allow + deny split)", len(edges))
	}

	var allowEdge, denyEdge *PolicyEdge
	for index := range edges {
		switch edges[index].Action {
		case models.ActionAllow:
			allowEdge = &edges[index]
		case models.ActionDeny:
			denyEdge = &edges[index]
		}
	}
	if allowEdge == nil || denyEdge == nil {
		t.Fatalf("missing allow or deny edge: %+v", edges)
	}

	if len(allowEdge.Ports) != 2 {
		t.Errorf("allow edge ports = %+v, want 2 entries (8080, 9090)", allowEdge.Ports)
	}
	if len(allowEdge.L7Matches) != 1 {
		t.Errorf("allow edge L7Matches = %d, want 1 (dedup across port fan-out)", len(allowEdge.L7Matches))
	}
	if len(denyEdge.L7Matches) != 0 {
		t.Errorf("deny edge L7Matches = %d, want 0 (rule had no L7)", len(denyEdge.L7Matches))
	}
}

// Two rules fold into one edge with overlapping ports. Guards against:
//   - append running only on first rule per bucket (brace-scope regression)
//   - appendUniquePort forgetting to mark seen, leading to dupes in candidates
//   - the slice walk being silently swapped back to scalar assignment
func TestRenderEdges_PortDedupAcrossRules(t *testing.T) {
	nodes := []models.WorkloadNode{{ID: "src"}, {ID: "dst"}}
	ref := models.PolicyRef{Source: "k8s", Name: "p1", Namespace: "ns-a"}

	rule1 := models.Rule{
		SrcID: "src", DstID: "dst",
		Ports:       []models.Port{{Port: 80, Protocol: "TCP"}, {Port: 443, Protocol: "TCP"}},
		Direction:   models.DirectionIngress,
		Contributor: ref,
		Action:      models.ActionAllow,
	}
	rule2 := rule1
	rule2.Ports = []models.Port{{Port: 443, Protocol: "TCP"}, {Port: 8080, Protocol: "TCP"}}

	edges := RenderEdges([]models.Rule{rule1, rule2}, nodes)
	if len(edges) != 1 {
		t.Fatalf("got %d edges, want 1 (same key folds)", len(edges))
	}
	if len(edges[0].Ports) != 3 {
		t.Errorf("got ports %+v, want 3 unique (80, 443, 8080)", edges[0].Ports)
	}
}

// Edge view intentionally dedups ports on port number alone, ignoring
// protocol / Name / EndPort. NodeRule (detail panel) preserves those.
// Locks in the conscious choice so a future "fix" doesn't quietly split.
func TestRenderEdges_EdgeIgnoresProtocol(t *testing.T) {
	nodes := []models.WorkloadNode{{ID: "src"}, {ID: "dst"}}
	ref := models.PolicyRef{Source: "k8s", Name: "p1", Namespace: "ns-a"}

	rule1 := models.Rule{
		SrcID: "src", DstID: "dst",
		Ports:       []models.Port{{Port: 80, Protocol: "TCP"}},
		Direction:   models.DirectionIngress,
		Contributor: ref,
		Action:      models.ActionAllow,
	}
	rule2 := rule1
	rule2.Ports = []models.Port{{Port: 80, Protocol: "UDP"}}

	edges := RenderEdges([]models.Rule{rule1, rule2}, nodes)
	if len(edges) != 1 {
		t.Fatalf("got %d edges, want 1", len(edges))
	}
	if len(edges[0].Ports) != 1 {
		t.Errorf("edge should dedup on port number alone, got %+v", edges[0].Ports)
	}
}
