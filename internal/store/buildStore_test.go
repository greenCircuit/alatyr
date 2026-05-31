package store

import (
	"testing"

	"graph/internal/models"
)

// Focused tests on IsNodesReachable. Cover the verdict states an operator
// actually reads from the UI: allow, explicit-deny, default-deny, not enforced,
// plus the namespace-node matcher (pod→ns rule should permit pod→pod traffic
// when the destination's ns is the matcher). Internal struct shape (engine
// map keys, per-direction Reason strings) is left to end-to-end tests so
// refactors don't churn this file.

const (
	srcNs   = "src-ns"
	dstNs   = "dst-ns"
	srcNsID = "ns/src-ns"
	dstNsID = "ns/dst-ns"
	srcID   = "src-ns/src-pod"
	dstID   = "dst-ns/dst-pod"
)

// buildCache builds a minimal cache with two NSIndex entries (so the ns-node
// IDs resolve) and one engine's EvaluationResult containing the given rules
// and lock state.
func buildCache(engine string, allow, deny []models.Rule, srcEgressLocked, dstIngressLocked bool) *models.Cache {
	srcNsNode := &models.WorkloadNode{ID: srcNsID, Namespace: srcNs, Type: models.NodeTypeNamespace}
	dstNsNode := &models.WorkloadNode{ID: dstNsID, Namespace: dstNs, Type: models.NodeTypeNamespace}

	allowByNs := map[string][]models.Rule{}
	denyByNs := map[string][]models.Rule{}
	for _, r := range allow {
		// Allow egress lives in src ns, ingress lives in dst ns. Mirror the
		// store walk so a rule shows up under the right ns key.
		if r.Direction == models.DirectionEgress {
			allowByNs[srcNs] = append(allowByNs[srcNs], r)
		} else {
			allowByNs[dstNs] = append(allowByNs[dstNs], r)
		}
	}
	for _, r := range deny {
		if r.Direction == models.DirectionEgress {
			denyByNs[srcNs] = append(denyByNs[srcNs], r)
		} else {
			denyByNs[dstNs] = append(denyByNs[dstNs], r)
		}
	}

	return &models.Cache{
		NsIndex: map[string]models.NSIndex{
			srcNs: {NSNode: srcNsNode},
			dstNs: {NSNode: dstNsNode},
		},
		EvaluationResults: map[string]models.EvaluationResult{
			engine: {
				AllowByNs: allowByNs,
				DenyByNs:  denyByNs,
				PolicyStatuses: map[string]models.PolicyStatus{
					srcID: {EgressLocked: srcEgressLocked},
					dstID: {IngressLocked: dstIngressLocked},
				},
			},
		},
	}
}

func TestIsNodesReachable_AllowPath(t *testing.T) {
	// Both directions locked, both have a matching allow rule → reachable.
	allow := []models.Rule{
		{SrcID: srcID, DstID: dstID, Direction: models.DirectionEgress, Action: models.ActionAllow},
		{SrcID: srcID, DstID: dstID, Direction: models.DirectionIngress, Action: models.ActionAllow},
	}
	cache := buildCache("k8s", allow, nil, true, true)
	got := IsNodesReachable(cache, srcID, srcNs, dstID, dstNs)

	if got.Verdict != "allow" {
		t.Fatalf("verdict: want allow, got %q (reason=%q)", got.Verdict, got.Reason)
	}
	if got.Engines["k8s"].Status != "allow" {
		t.Errorf("engine status: want allow, got %q", got.Engines["k8s"].Status)
	}
}

func TestIsNodesReachable_ExplicitDenyBlocks(t *testing.T) {
	// Allow rule present, but a deny on the same path wins.
	allow := []models.Rule{
		{SrcID: srcID, DstID: dstID, Direction: models.DirectionIngress, Action: models.ActionAllow},
	}
	deny := []models.Rule{
		{SrcID: srcID, DstID: dstID, Direction: models.DirectionIngress, Action: models.ActionDeny},
	}
	cache := buildCache("istio", allow, deny, false, true)
	got := IsNodesReachable(cache, srcID, srcNs, dstID, dstNs)

	if got.Verdict != "deny" {
		t.Fatalf("verdict: want deny (explicit deny rule), got %q", got.Verdict)
	}
	if got.Engines["istio"].Status != "deny" {
		t.Errorf("engine status: want deny, got %q", got.Engines["istio"].Status)
	}
}

func TestIsNodesReachable_DefaultDenyWhenLockedWithoutAllow(t *testing.T) {
	// Ingress locked, no allow rule → default-deny.
	cache := buildCache("k8s", nil, nil, false, true)
	got := IsNodesReachable(cache, srcID, srcNs, dstID, dstNs)

	if got.Verdict != "deny" {
		t.Fatalf("verdict: want deny (default-deny via lock), got %q", got.Verdict)
	}
	if got.Engines["k8s"].Ingress.Reason == "" {
		t.Error("ingress reason: expected non-empty explanation for default-deny")
	}
}

func TestIsNodesReachable_NotEnforcedWhenNoOpinion(t *testing.T) {
	// No locks, no matching rules → engine has no opinion → "not enforced".
	cache := buildCache("k8s", nil, nil, false, false)
	got := IsNodesReachable(cache, srcID, srcNs, dstID, dstNs)

	if got.Verdict != "allow" {
		t.Fatalf("verdict: want allow (transparent engine permits), got %q", got.Verdict)
	}
	if got.Engines["k8s"].Status != "not enforced" {
		t.Errorf("engine status: want \"not enforced\", got %q", got.Engines["k8s"].Status)
	}
}

func TestIsNodesReachable_NsNodeMatcherPermitsPodToPod(t *testing.T) {
	// Rule says "src pod → entire dst ns" (DstID = ns-node ID). When asking
	// about src pod → dst pod, the dst-ns matcher must catch this rule.
	allow := []models.Rule{
		{SrcID: srcID, DstID: dstNsID, Direction: models.DirectionEgress, Action: models.ActionAllow},
		{SrcID: srcID, DstID: dstID, Direction: models.DirectionIngress, Action: models.ActionAllow},
	}
	cache := buildCache("k8s", allow, nil, true, true)
	got := IsNodesReachable(cache, srcID, srcNs, dstID, dstNs)

	if got.Verdict != "allow" {
		t.Fatalf("verdict: want allow (ns-node matcher should resolve pod→ns rule), got %q (reason=%q)",
			got.Verdict, got.Reason)
	}
	if len(got.Engines["k8s"].Egress.AllowMatches) == 0 {
		t.Error("egress allow matches: expected the pod→ns rule to be captured")
	}
}

func TestIsNodesReachable_AllowOnOneEngineDeniedOnAnotherBlocks(t *testing.T) {
	// Multi-engine AND: k8s allows but istio denies → overall deny.
	cache := buildCache("k8s",
		[]models.Rule{
			{SrcID: srcID, DstID: dstID, Direction: models.DirectionEgress, Action: models.ActionAllow},
			{SrcID: srcID, DstID: dstID, Direction: models.DirectionIngress, Action: models.ActionAllow},
		}, nil, true, true)
	// Tack on a second engine that denies.
	cache.EvaluationResults["istio"] = models.EvaluationResult{
		DenyByNs: map[string][]models.Rule{
			dstNs: {{SrcID: srcID, DstID: dstID, Direction: models.DirectionIngress, Action: models.ActionDeny}},
		},
		PolicyStatuses: map[string]models.PolicyStatus{
			dstID: {IngressLocked: true},
		},
	}
	got := IsNodesReachable(cache, srcID, srcNs, dstID, dstNs)

	if got.Verdict != "deny" {
		t.Fatalf("verdict: want deny (istio blocks), got %q", got.Verdict)
	}
	if got.Engines["k8s"].Status != "allow" {
		t.Errorf("k8s engine: want allow, got %q", got.Engines["k8s"].Status)
	}
	if got.Engines["istio"].Status != "deny" {
		t.Errorf("istio engine: want deny, got %q", got.Engines["istio"].Status)
	}
}
