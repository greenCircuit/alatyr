package store

import (
	"context"
	"testing"

	"graph/internal/models"
)

// Tests for PolicyIssues — the whole-cluster conflict scan. A "policy conflict"
// is an ALLOW rule whose src→dst path is nevertheless blocked once every
// engine's rules + locks intersect. Reuses the srcID/dstID constants and the
// buildNodeRules helper from buildStore_test.go (same package).

// engineResult builds one engine's EvaluationResult: allow/deny rules grouped
// by ns the way the store walk expects, the NodeRules index reachability reads,
// and the per-direction locks. Mirrors buildCache's engine construction.
func engineResult(allow, deny []models.Rule, srcEgressLocked, dstIngressLocked bool) models.EvaluationResult {
	allowByNs := map[string][]models.Rule{}
	denyByNs := map[string][]models.Rule{}
	for _, rule := range allow {
		if rule.Direction == models.DirectionEgress {
			allowByNs[srcNs] = append(allowByNs[srcNs], rule)
		} else {
			allowByNs[dstNs] = append(allowByNs[dstNs], rule)
		}
	}
	for _, rule := range deny {
		if rule.Direction == models.DirectionEgress {
			denyByNs[srcNs] = append(denyByNs[srcNs], rule)
		} else {
			denyByNs[dstNs] = append(denyByNs[dstNs], rule)
		}
	}
	return models.EvaluationResult{
		AllowByNs: allowByNs,
		DenyByNs:  denyByNs,
		NodeRules: buildNodeRules(append(append([]models.Rule{}, allow...), deny...)),
		PolicyStatuses: map[string]models.PolicyStatus{
			srcID: {EgressLocked: srcEgressLocked},
			dstID: {IngressLocked: dstIngressLocked},
		},
	}
}

// issueCache wires NsIndex with real pod workloads (so WorkloadByID resolves
// src/dst) plus the given engines.
func issueCache(engines map[string]models.EvaluationResult) *models.Cache {
	srcPod := models.WorkloadNode{ID: srcID, Namespace: srcNs, Label: "src-pod"}
	dstPod := models.WorkloadNode{ID: dstID, Namespace: dstNs, Label: "dst-pod"}
	srcNsNode := &models.WorkloadNode{ID: srcNsID, Namespace: srcNs, Type: models.NodeTypeNamespace}
	dstNsNode := &models.WorkloadNode{ID: dstNsID, Namespace: dstNs, Type: models.NodeTypeNamespace}
	cache := &models.Cache{
		NsIndex: map[string]models.NSIndex{
			srcNs: {NSNode: srcNsNode, Workloads: []models.WorkloadNode{srcPod}},
			dstNs: {NSNode: dstNsNode, Workloads: []models.WorkloadNode{dstPod}},
		},
		EvaluationResults: engines,
	}
	cache.RebuildWorkloadIndex()
	return cache
}

func allowBothDirections() []models.Rule {
	return []models.Rule{
		{SrcID: srcID, DstID: dstID, Direction: models.DirectionEgress, Action: models.ActionAllow},
		{SrcID: srcID, DstID: dstID, Direction: models.DirectionIngress, Action: models.ActionAllow},
	}
}

// One engine allows the path while another denies it — the canonical conflict.
// The emitted issue must carry both endpoints so the UI can open reachability.
func TestPolicyIssues_ConflictFlaggedWithEndpoints(t *testing.T) {
	cache := issueCache(map[string]models.EvaluationResult{
		"k8s": engineResult(allowBothDirections(), nil, true, true),
		"istio": engineResult(nil,
			[]models.Rule{{SrcID: srcID, DstID: dstID, Direction: models.DirectionIngress, Action: models.ActionDeny}},
			false, true),
	})

	issues := PolicyIssues(context.Background(), cache)

	if len(issues) != 1 {
		t.Fatalf("issues: want 1 conflict, got %d", len(issues))
	}
	issue := issues[0]
	if issue.Type != models.PolicyConflicts {
		t.Errorf("type: want %q, got %q", models.PolicyConflicts, issue.Type)
	}
	if issue.Src == nil || issue.Src.ID != srcID {
		t.Errorf("src: want endpoint %q, got %+v", srcID, issue.Src)
	}
	if issue.Dst == nil || issue.Dst.ID != dstID {
		t.Errorf("dst: want endpoint %q, got %+v", dstID, issue.Dst)
	}
}

// Allowed and reachable path — no engine blocks it — must produce no issue.
func TestPolicyIssues_NoConflictWhenPathAllowed(t *testing.T) {
	cache := issueCache(map[string]models.EvaluationResult{
		"k8s": engineResult(allowBothDirections(), nil, true, true),
	})

	issues := PolicyIssues(context.Background(), cache)

	if len(issues) != 0 {
		t.Fatalf("issues: want 0 (path permitted), got %d", len(issues))
	}
}

// Multiple allow rules on the same src→dst pair must be deduped to one issue —
// the scan keys by src-dst, not per rule.
func TestPolicyIssues_DedupesSamePair(t *testing.T) {
	allow := append(allowBothDirections(),
		models.Rule{SrcID: srcID, DstID: dstID, Direction: models.DirectionEgress, Action: models.ActionAllow})
	cache := issueCache(map[string]models.EvaluationResult{
		"k8s": engineResult(allow, nil, true, true),
		"istio": engineResult(nil,
			[]models.Rule{{SrcID: srcID, DstID: dstID, Direction: models.DirectionIngress, Action: models.ActionDeny}},
			false, true),
	})

	issues := PolicyIssues(context.Background(), cache)

	if len(issues) != 1 {
		t.Fatalf("issues: want 1 (deduped by src-dst), got %d", len(issues))
	}
}
