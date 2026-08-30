package store

import (
	"testing"

	"graph/internal/models"
)

// GetNodeData feeds the click-on-node detail panel: per-engine outbound rules
// (node is the source) plus the policies that select the node. Reuses the
// srcNs/srcID/dstNs/dstID consts from buildStore_test.go.

func TestGetNodeData_ReturnsOutboundRulesAndPolicies(t *testing.T) {
	allowRule := models.Rule{SrcID: srcID, DstID: dstID, Direction: models.DirectionEgress, Action: models.ActionAllow}
	denyRule := models.Rule{SrcID: srcID, DstID: dstID, Direction: models.DirectionEgress, Action: models.ActionDeny}
	// Rule where the clicked node is the destination, not the source — excluded.
	otherSourceRule := models.Rule{SrcID: dstID, DstID: srcID, Direction: models.DirectionEgress}

	cache := &models.Cache{
		NsIndex: map[string]models.NSIndex{
			dstNs: {Workloads: []models.WorkloadNode{{ID: dstID, Label: "dst-pod", Namespace: dstNs}}},
		},
		EvaluationResults: map[string]models.EvaluationResult{
			"k8s": {
				AllowByNs:    map[string][]models.Rule{srcNs: {allowRule, otherSourceRule}},
				DenyByNs:     map[string][]models.Rule{srcNs: {denyRule}},
				NodePolicies: map[string][]models.PolicyRef{srcID: {{Source: "k8s", Name: "deny-all", Namespace: srcNs}}},
			},
			// Engine with rules only for some other node — must be omitted.
			"istio": {
				AllowByNs: map[string][]models.Rule{srcNs: {{SrcID: dstID, DstID: srcID}}},
			},
		},
	}
	cache.RebuildWorkloadIndex()

	got := GetNodeData(cache, srcID, srcNs)

	engine, ok := got["k8s"]
	if !ok {
		t.Fatalf("k8s engine missing from node data: %v", got)
	}
	// allow + deny kept, the dst-as-source rule dropped.
	if len(engine.Rules) != 2 {
		t.Fatalf("want 2 outbound rules (allow+deny), got %d", len(engine.Rules))
	}
	if len(engine.Policies) != 1 {
		t.Errorf("want 1 selecting policy, got %d", len(engine.Policies))
	}
	// DstID resolved to a human label via the workload index.
	if engine.Rules[0].DstLabel != "dst-pod" || engine.Rules[0].DstNamespace != dstNs {
		t.Errorf("dst not resolved: label=%q ns=%q", engine.Rules[0].DstLabel, engine.Rules[0].DstNamespace)
	}
	if _, ok := got["istio"]; ok {
		t.Error("istio engine has no rules/policies for this node — should be omitted")
	}
}

func TestGetNodeData_PolicyOnlyEngineIncluded(t *testing.T) {
	// Engine selects the node but emits no rule (e.g. default-deny) — still
	// surfaced so the panel shows "selected by X, allows nothing".
	cache := &models.Cache{
		EvaluationResults: map[string]models.EvaluationResult{
			"k8s": {NodePolicies: map[string][]models.PolicyRef{srcID: {{Source: "k8s", Name: "default-deny", Namespace: srcNs}}}},
		},
	}

	got := GetNodeData(cache, srcID, srcNs)

	engine, ok := got["k8s"]
	if !ok {
		t.Fatal("policy-only engine should be present")
	}
	if len(engine.Rules) != 0 || len(engine.Policies) != 1 {
		t.Errorf("want 0 rules / 1 policy, got %d / %d", len(engine.Rules), len(engine.Policies))
	}
}

func TestGetNodeData_EmptyWhenNodeUnreferenced(t *testing.T) {
	cache := &models.Cache{
		EvaluationResults: map[string]models.EvaluationResult{
			"k8s": {AllowByNs: map[string][]models.Rule{srcNs: {{SrcID: dstID, DstID: srcID}}}},
		},
	}

	got := GetNodeData(cache, srcID, srcNs)

	if len(got) != 0 {
		t.Errorf("want empty map for node with no rules/policies, got %v", got)
	}
}
