package store

import (
	"context"
	"testing"

	"graph/internal/mesh"
	"graph/internal/models"
)

// stubMeshSource is a mesh.MeshSource that records CanReach calls and returns
// a canned verdict. Membership/ResolveMtls/BuildMeshMembership are never hit
// by IsEndpointsReachable or MeshReachabilityUsingNodes — the new call path
// only touches CanReach + Name.
type stubMeshSource struct {
	name       string
	verdict    models.MeshVerdict
	canReachN  int
	lastSrc    models.MeshMembership
	lastDst    models.MeshMembership
	lastPort   uint32
}

func (s *stubMeshSource) Name() string { return s.name }

func (s *stubMeshSource) CanReach(src, dst models.MeshMembership, port uint32) models.MeshVerdict {
	s.canReachN++
	s.lastSrc = src
	s.lastDst = dst
	s.lastPort = port
	return s.verdict
}

func (s *stubMeshSource) Membership(models.WorkloadNode, map[string]string) *models.MeshMembership {
	return nil
}
func (s *stubMeshSource) ResolveMtls(context.Context, models.WorkloadNode, map[string]string) (*models.MtlsState, error) {
	return nil, nil
}
func (s *stubMeshSource) BuildMeshMembership(map[string]models.NSIndex) (models.MeshBuildResult, error) {
	return models.MeshBuildResult{}, nil
}

var _ mesh.MeshSource = (*stubMeshSource)(nil)

// allowingPolicyCache reuses buildCache with an allow-both-directions rule set
// so the policy-layer verdict is "allow" — mesh alone drives IsEndpointsReachable.
func allowingPolicyCache() *models.Cache {
	return buildCache("k8s", egressTo(dstID), ingressFrom(srcID))
}

// seedWorkload writes a workload into cache.WorkloadByID and, for ns nodes,
// into NsIndex.NSNode so cache lookups from the reachability code find them.
func seedWorkload(cache *models.Cache, node models.WorkloadNode) {
	if cache.WorkloadByID == nil {
		cache.WorkloadByID = map[string]models.WorkloadNode{}
	}
	cache.WorkloadByID[node.ID] = node
}

// meshSource nil short-circuits to the raw policy verdict.
func TestIsEndpointsReachable_NilMeshSourceReturnsPolicyOnly(t *testing.T) {
	cache := allowingPolicyCache()

	got := IsEndpointsReachable(context.Background(), cache, nil, srcID, srcNs, dstID, dstNs)

	if got.Verdict != "allow" {
		t.Fatalf("verdict: want allow, got %q (%s)", got.Verdict, got.Reason)
	}
	if got.Mesh != nil {
		t.Errorf("Mesh should be nil when no mesh source, got %+v", got.Mesh)
	}
}

// Missing src workload in cache → mesh skipped (no CanReach call). Guards
// against a bogus zero-value mesh verdict against a workload that doesn't exist.
func TestIsEndpointsReachable_MissingSrcSkipsMesh(t *testing.T) {
	cache := allowingPolicyCache()
	seedWorkload(cache, models.WorkloadNode{ID: dstID, Namespace: dstNs, Type: models.NodeTypeDeployment})
	stub := &stubMeshSource{name: "istio", verdict: models.MeshVerdict{Verdict: "deny", Reason: "should not run"}}

	got := IsEndpointsReachable(context.Background(), cache, stub, srcID, srcNs, dstID, dstNs)

	if stub.canReachN != 0 {
		t.Errorf("CanReach called %d times; want 0 when src not in cache", stub.canReachN)
	}
	if got.Verdict != "allow" {
		t.Errorf("verdict: want allow (mesh skipped), got %q", got.Verdict)
	}
}

// Namespace-typed endpoint on either side → mesh skipped. The zero
// MeshMembership would false-positive a ns node as "not in mesh". This is the
// exact footgun the new comment calls out.
func TestIsEndpointsReachable_NamespaceEndpointSkipsMesh(t *testing.T) {
	cases := []struct {
		name      string
		srcType   models.NodeType
		dstType   models.NodeType
	}{
		{"src is namespace", models.NodeTypeNamespace, models.NodeTypeDeployment},
		{"dst is namespace", models.NodeTypeDeployment, models.NodeTypeNamespace},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cache := allowingPolicyCache()
			seedWorkload(cache, models.WorkloadNode{ID: srcID, Namespace: srcNs, Type: tc.srcType})
			seedWorkload(cache, models.WorkloadNode{ID: dstID, Namespace: dstNs, Type: tc.dstType})
			stub := &stubMeshSource{name: "istio", verdict: models.MeshVerdict{Verdict: "deny", Reason: "should not run"}}

			got := IsEndpointsReachable(context.Background(), cache, stub, srcID, srcNs, dstID, dstNs)

			if stub.canReachN != 0 {
				t.Errorf("CanReach called for %s; want 0", tc.name)
			}
			if got.Verdict != "allow" {
				t.Errorf("verdict: want allow, got %q", got.Verdict)
			}
		})
	}
}

// Policy allows + mesh denies → overall verdict flips to deny with the mesh
// reason. Guards the fuse layer.
func TestIsEndpointsReachable_MeshDenyOverridesAllow(t *testing.T) {
	cache := allowingPolicyCache()
	seedWorkload(cache, models.WorkloadNode{ID: srcID, Namespace: srcNs, Type: models.NodeTypeDeployment})
	seedWorkload(cache, models.WorkloadNode{ID: dstID, Namespace: dstNs, Type: models.NodeTypeDeployment})
	stub := &stubMeshSource{name: "istio", verdict: models.MeshVerdict{Verdict: "deny", Reason: "STRICT gate"}}

	got := IsEndpointsReachable(context.Background(), cache, stub, srcID, srcNs, dstID, dstNs)

	if got.Verdict != "deny" {
		t.Fatalf("verdict: want deny (mesh blocks), got %q", got.Verdict)
	}
	if got.Reason == "" || got.Reason == "all engines permit" {
		t.Errorf("reason should mention mesh, got %q", got.Reason)
	}
	if got.Mesh == nil || got.Mesh["istio"].Verdict != "deny" {
		t.Errorf("Mesh map should carry the deny verdict, got %+v", got.Mesh)
	}
	if stub.lastPort != 0 {
		t.Errorf("mesh CanReach called with port %d; want 0 (workload verdict)", stub.lastPort)
	}
}

// Policy allows + mesh allows → allow, mesh verdict recorded.
func TestIsEndpointsReachable_MeshAllowKeepsAllow(t *testing.T) {
	cache := allowingPolicyCache()
	seedWorkload(cache, models.WorkloadNode{ID: srcID, Namespace: srcNs, Type: models.NodeTypeDeployment})
	seedWorkload(cache, models.WorkloadNode{ID: dstID, Namespace: dstNs, Type: models.NodeTypeDeployment})
	stub := &stubMeshSource{name: "istio", verdict: models.MeshVerdict{Verdict: "allow", Reason: "mesh permits"}}

	got := IsEndpointsReachable(context.Background(), cache, stub, srcID, srcNs, dstID, dstNs)

	if got.Verdict != "allow" {
		t.Fatalf("verdict: want allow, got %q (%s)", got.Verdict, got.Reason)
	}
	if got.Mesh["istio"].Verdict != "allow" {
		t.Errorf("mesh verdict not surfaced, got %+v", got.Mesh)
	}
}

// MeshReachabilityUsingNodes: missing cache entries produce zero
// MeshMembership on both sides. Contract: no panic, zero passed through to
// CanReach as-is.
func TestMeshReachabilityUsingNodes_MissingMembershipDecodesAsZero(t *testing.T) {
	cache := &models.Cache{MeshMembership: map[string]models.MeshMembership{}}
	stub := &stubMeshSource{name: "istio", verdict: models.MeshVerdict{Verdict: "allow"}}

	MeshReachabilityUsingNodes(cache, stub,
		models.WorkloadNode{ID: "srcX"}, models.WorkloadNode{ID: "dstY"})

	if stub.lastSrc.InMesh || stub.lastDst.InMesh {
		t.Errorf("missing entries should produce zero MeshMembership, got src=%+v dst=%+v",
			stub.lastSrc, stub.lastDst)
	}
	if stub.lastPort != 0 {
		t.Errorf("port: want 0 (workload verdict), got %d", stub.lastPort)
	}
}

// MeshReachabilityUsingNodes forwards populated cache entries verbatim.
func TestMeshReachabilityUsingNodes_ForwardsCacheEntries(t *testing.T) {
	cache := &models.Cache{
		MeshMembership: map[string]models.MeshMembership{
			"srcX": {InMesh: true, Provider: "istio", Mode: "ambient"},
			"dstY": {InMesh: true, Provider: "istio", Mode: "ambient",
				Mtls: &models.MtlsState{Verdict: models.MeshStrict}},
		},
	}
	stub := &stubMeshSource{name: "istio", verdict: models.MeshVerdict{Verdict: "allow"}}

	MeshReachabilityUsingNodes(cache, stub,
		models.WorkloadNode{ID: "srcX"}, models.WorkloadNode{ID: "dstY"})

	if !stub.lastSrc.InMesh || stub.lastSrc.Provider != "istio" {
		t.Errorf("src not forwarded, got %+v", stub.lastSrc)
	}
	if stub.lastDst.Mtls == nil || stub.lastDst.Mtls.Verdict != models.MeshStrict {
		t.Errorf("dst Mtls not forwarded, got %+v", stub.lastDst.Mtls)
	}
}
