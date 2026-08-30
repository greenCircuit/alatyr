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
	ref := models.PolicyRef{Source: "istio", Name: "p1", Namespace: "ns-a"}

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

// Cluster-wide policy fan-out: a Calico all() egress rule resolves identically
// on every workload of a namespace. Covers, in one scenario:
//   - uniform group (all 3 workloads of ns-a) collapses to ONE namespace-level
//     edge, Level=namespace, so the canvas shows one arrow instead of N
//   - a namespace where only SOME workloads resolved the rule (first-match
//     precedence overrode the rest) keeps its per-workload arrows — those differ
//   - a rule whose peer differs is a separate group, never merged in
func TestRenderEdges_CollapsesUniformNamespaceFanOut(t *testing.T) {
	nodes := []models.WorkloadNode{
		{ID: "a1", Namespace: "ns-a", Type: models.NodeTypeDeployment},
		{ID: "a2", Namespace: "ns-a", Type: models.NodeTypeDeployment},
		{ID: "a3", Namespace: "ns-a", Type: models.NodeTypeDeployment},
		{ID: "ns-ns-a", Namespace: "ns-a", Type: models.NodeTypeNamespace},
		{ID: "b1", Namespace: "ns-b", Type: models.NodeTypeDeployment},
		{ID: "b2", Namespace: "ns-b", Type: models.NodeTypeDeployment},
		{ID: "ns-ns-b", Namespace: "ns-b", Type: models.NodeTypeNamespace},
		{ID: models.CIDRIDPrefix + "10.0.0.0/8", Type: models.NodeTypeCIDR},
		{ID: models.CIDRIDPrefix + "0.0.0.0/0", Type: models.NodeTypeCIDR},
	}
	ref := models.PolicyRef{Source: "calico", Name: "egress-lan"}

	lanRule := func(workloadID string) models.Rule {
		return models.Rule{
			SrcID: workloadID, DstID: models.CIDRIDPrefix + "10.0.0.0/8",
			Direction: models.DirectionEgress, Contributor: ref,
			Action: models.ActionAllow, Coverage: models.CoverageRestricted, AllPorts: true,
			NamespaceWide: true,
		}
	}
	// ns-a: every workload resolved it. ns-b: only b1 (b2 hit a narrower policy).
	rules := []models.Rule{lanRule("a1"), lanRule("a2"), lanRule("a3"), lanRule("b1")}
	// Different peer → its own group; ns-a is uniform here too.
	wanRule := lanRule("a1")
	wanRule.DstID = models.CIDRIDPrefix + "0.0.0.0/0"
	wanA2, wanA3 := wanRule, wanRule
	wanA2.SrcID, wanA3.SrcID = "a2", "a3"
	rules = append(rules, wanRule, wanA2, wanA3)

	edges := RenderEdges(rules, nodes)

	bySource := map[string]*PolicyEdge{}
	for index := range edges {
		bySource[edges[index].Source+"->"+edges[index].Target] = &edges[index]
	}
	if len(edges) != 3 {
		t.Fatalf("got %d edges, want 3 (ns-a lan, ns-a wan, b1 lan): %+v", len(edges), edges)
	}
	collapsed := bySource["ns-ns-a->"+models.CIDRIDPrefix+"10.0.0.0/8"]
	if collapsed == nil {
		t.Fatalf("uniform ns-a fan-out not collapsed to the namespace node: %+v", edges)
	}
	// Level stays workload: the fact is per-workload, only the drawing aggregates.
	// Namespace level would put it under the UI's ns-edge toggle, which hides
	// genuine ns-scoped policy — not "can this pod reach 10.0.0.0/8".
	if collapsed.Level != models.EdgeLevelWorkload {
		t.Errorf("collapsed edge level: want workload, got %q", collapsed.Level)
	}
	if collapsed.AggregatedFrom != 3 {
		t.Errorf("collapsed edge must report the 3 workloads it stands for, got %d", collapsed.AggregatedFrom)
	}
	if perWorkload := bySource["b1->"+models.CIDRIDPrefix+"10.0.0.0/8"]; perWorkload != nil && perWorkload.AggregatedFrom != 0 {
		t.Errorf("un-collapsed edge must not claim aggregation, got %d", perWorkload.AggregatedFrom)
	}
	if bySource["ns-ns-a->"+models.CIDRIDPrefix+"0.0.0.0/0"] == nil {
		t.Errorf("second uniform group (different peer) not collapsed: %+v", edges)
	}
	if bySource["b1->"+models.CIDRIDPrefix+"10.0.0.0/8"] == nil {
		t.Errorf("partial ns-b coverage must stay per-workload: %+v", edges)
	}
}

// Single-workload namespaces keep the workload endpoint — collapsing there
// trades node precision for no reduction in arrows.
func TestRenderEdges_KeepsSingleWorkloadNamespace(t *testing.T) {
	nodes := []models.WorkloadNode{
		{ID: "only", Namespace: "ns-a", Type: models.NodeTypeDeployment},
		{ID: "ns-ns-a", Namespace: "ns-a", Type: models.NodeTypeNamespace},
		{ID: models.CIDRIDPrefix + "10.0.0.0/8", Type: models.NodeTypeCIDR},
	}
	rule := models.Rule{
		SrcID: "only", DstID: models.CIDRIDPrefix + "10.0.0.0/8",
		Direction:   models.DirectionEgress,
		Contributor: models.PolicyRef{Source: "calico", Name: "egress-lan"},
		AllPorts:    true,
	}

	edges := RenderEdges([]models.Rule{rule}, nodes)
	if len(edges) != 1 || edges[0].Source != "only" {
		t.Fatalf("single-workload ns must keep the workload endpoint, got %+v", edges)
	}
}
