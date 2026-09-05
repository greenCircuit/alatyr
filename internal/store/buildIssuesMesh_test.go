package store

import (
	"context"
	"testing"

	"alatyr/internal/models"
)

// meshConflictIssues extracts only the MeshConflicts entries so a test can
// assert on the mesh side without noise from policy conflicts.
func meshConflictIssues(issues []models.Issue) []models.Issue {
	var out []models.Issue
	for _, issue := range issues {
		if issue.Type == models.MeshConflicts {
			out = append(out, issue)
		}
	}
	return out
}

// Allow rule + mesh deny on the same pair → one MeshConflicts issue with both
// endpoints and the mesh engine name. Guards the new mesh conflict path.
func TestPolicyIssues_MeshDenyEmitsMeshConflict(t *testing.T) {
	cache := issueCache(map[string]models.EvaluationResult{
		"k8s": engineResult(allowBothDirections(), nil, false, false),
	})
	cache.MeshMembership = map[string]models.MeshMembership{
		srcID: {InMesh: false},
		dstID: {InMesh: true, Mtls: &models.MtlsState{Verdict: models.MeshStrict}},
	}
	stub := &stubMeshSource{
		name:    "istio",
		verdict: models.MeshVerdict{Verdict: "deny", Reason: "STRICT gate"},
	}

	issues := PolicyIssues(context.Background(), cache, stub)

	mesh := meshConflictIssues(issues)
	if len(mesh) != 1 {
		t.Fatalf("mesh issues: want 1, got %d (all=%+v)", len(mesh), issues)
	}
	issue := mesh[0]
	if issue.Engine != "istio" {
		t.Errorf("engine: want istio, got %q", issue.Engine)
	}
	if issue.Src == nil || issue.Src.ID != srcID || issue.Dst == nil || issue.Dst.ID != dstID {
		t.Errorf("endpoints: want %s→%s, got %+v→%+v", srcID, dstID, issue.Src, issue.Dst)
	}
}

// EffectiveSource on the mesh verdict → IngressCulprits carries the PA
// identity so the UI can link to the offending PeerAuthentication.
func TestPolicyIssues_MeshDenyStampsCulpritFromEffectiveSource(t *testing.T) {
	cache := issueCache(map[string]models.EvaluationResult{
		"k8s": engineResult(allowBothDirections(), nil, false, false),
	})
	cache.MeshMembership = map[string]models.MeshMembership{
		srcID: {InMesh: false},
		dstID: {InMesh: true, Mtls: &models.MtlsState{Verdict: models.MeshStrict}},
	}
	stub := &stubMeshSource{
		name: "istio",
		verdict: models.MeshVerdict{
			Verdict: "deny",
			Reason:  "STRICT",
			EffectiveSource: &models.MtlsPolicyApplied{
				Namespace: "istio-system",
				Name:      "root-strict",
			},
		},
	}

	issues := PolicyIssues(context.Background(), cache, stub)
	mesh := meshConflictIssues(issues)
	if len(mesh) != 1 {
		t.Fatalf("mesh issues: want 1, got %d", len(mesh))
	}
	if len(mesh[0].IngressCulprits) != 1 {
		t.Fatalf("IngressCulprits: want 1, got %+v", mesh[0].IngressCulprits)
	}
	culprit := mesh[0].IngressCulprits[0]
	// Source is "pa" (get-manifest.go's manifest-kind dispatch key for
	// PeerAuthentication), not the mesh engine name — "istio" would fetch an
	// AuthorizationPolicy instead and 400.
	if culprit.Source != "pa" || culprit.Name != "root-strict" || culprit.Namespace != "istio-system" {
		t.Errorf("culprit: want pa istio-system/root-strict, got %+v", culprit)
	}
}

// meshSource nil → no MeshConflicts, no panic, no CanReach call. Regression
// guard for callers that pass nil.
func TestPolicyIssues_NilMeshSourceSkipsMeshPath(t *testing.T) {
	cache := issueCache(map[string]models.EvaluationResult{
		"k8s": engineResult(allowBothDirections(), nil, false, false),
	})

	issues := PolicyIssues(context.Background(), cache, nil)

	if got := meshConflictIssues(issues); len(got) != 0 {
		t.Fatalf("mesh issues expected 0 with nil meshSource, got %+v", got)
	}
}

// Mesh allow → no MeshConflicts issue even when a pair has allow rules.
func TestPolicyIssues_MeshAllowSuppressesConflict(t *testing.T) {
	cache := issueCache(map[string]models.EvaluationResult{
		"k8s": engineResult(allowBothDirections(), nil, false, false),
	})
	stub := &stubMeshSource{name: "istio", verdict: models.MeshVerdict{Verdict: "allow"}}

	issues := PolicyIssues(context.Background(), cache, stub)

	if got := meshConflictIssues(issues); len(got) != 0 {
		t.Fatalf("mesh allow should not raise conflict, got %+v", got)
	}
}

// Namespace-typed endpoint on the pair → mesh path SKIPPED even when the mesh
// source would deny. The zero MeshMembership would false-positive a ns node
// as "not in mesh"; the guard exists to prevent that. Same footgun as
// IsEndpointsReachable — asserted independently here.
func TestPolicyIssues_MeshSkippedForNamespaceEndpoint(t *testing.T) {
	// Replace the src pod with a namespace-typed node under the same ID so the
	// allow rule still resolves, but the mesh guard triggers on Type check.
	cache := issueCache(map[string]models.EvaluationResult{
		"k8s": engineResult(allowBothDirections(), nil, false, false),
	})
	cache.NsIndex[srcNs] = models.NSIndex{
		NSNode: cache.NsIndex[srcNs].NSNode,
		Workloads: []models.WorkloadNode{
			{ID: srcID, Namespace: srcNs, Type: models.NodeTypeNamespace},
		},
	}
	cache.RebuildWorkloadIndex()
	stub := &stubMeshSource{name: "istio", verdict: models.MeshVerdict{Verdict: "deny", Reason: "should not run"}}

	issues := PolicyIssues(context.Background(), cache, stub)

	if stub.canReachN != 0 {
		t.Errorf("CanReach called %d times; want 0 for ns endpoint", stub.canReachN)
	}
	if got := meshConflictIssues(issues); len(got) != 0 {
		t.Fatalf("mesh conflict for ns endpoint (footgun), got %+v", got)
	}
}

// Same src→dst pair with multiple allow rules must produce only ONE
// MeshConflicts issue — the pair-level dedup key covers the mesh path too.
func TestPolicyIssues_MeshConflictDedupesSamePair(t *testing.T) {
	allow := append(allowBothDirections(),
		models.Rule{SrcID: srcID, DstID: dstID, Direction: models.DirectionEgress, Action: models.ActionAllow})
	cache := issueCache(map[string]models.EvaluationResult{
		"k8s": engineResult(allow, nil, false, false),
	})
	stub := &stubMeshSource{name: "istio", verdict: models.MeshVerdict{Verdict: "deny", Reason: "STRICT"}}

	issues := PolicyIssues(context.Background(), cache, stub)

	if got := meshConflictIssues(issues); len(got) != 1 {
		t.Fatalf("mesh conflict: want 1 (deduped by pair), got %d", len(got))
	}
}
