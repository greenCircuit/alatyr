package graph

import (
	"testing"

	"graph/internal/models"
	"graph/internal/policy"
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

	l7 := &policy.L7Match{Methods: []string{"GET"}, Paths: []string{"/api"}}
	ref := policy.PolicyRef{Source: "istio", Name: "p1", Namespace: "ns-a", RuleIndex: 0}

	// Two allow rules sharing the same L7 (fan-out per port) — should fold
	// into one edge with ports {8080, 9090} and a single L7Match entry.
	allow8080 := policy.Rule{SrcID: "src", DstID: "dst", Port: models.Port{Port: 8080, Protocol: "TCP"}, Direction: models.DirectionIngress, Contributors: []policy.PolicyRef{ref}, L7Match: l7, Action: policy.ActionAllow}
	allow9090 := allow8080
	allow9090.Port = models.Port{Port: 9090, Protocol: "TCP"}

	// Same (src,dst,direction,policy) but action=deny — must NOT merge with
	// the allow group; produces a separate edge.
	denyRule := allow8080
	denyRule.Action = policy.ActionDeny
	denyRule.L7Match = nil

	edges := renderEdges([]policy.Rule{allow8080, allow9090, denyRule}, nodes)

	if len(edges) != 2 {
		t.Fatalf("got %d edges, want 2 (allow + deny split)", len(edges))
	}

	var allowEdge, denyEdge *PolicyEdge
	for index := range edges {
		switch edges[index].Action {
		case policy.ActionAllow:
			allowEdge = &edges[index]
		case policy.ActionDeny:
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
