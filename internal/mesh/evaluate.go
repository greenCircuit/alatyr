// Package mesh defines the MeshSource abstraction — sibling to policy.PolicySource.
// MeshSources resolve mesh membership + mTLS state on demand for a single
// workload (per detail / reachability request). No eager cluster-wide pass —
// keep the graph payload cheap; UI fetches mesh data only when needed.
//
// See docs/arch/0003-istio-mesh-membership-and-mtls.md.
package mesh

import (
	"context"

	"graph/internal/models"
)

// MeshSource is implemented by each mesh provider. Today only istio is wired.
type MeshSource interface {
	// Membership returns the workload's mesh membership (InMesh + Provider +
	// Mode + waypoint binding). Cheap label-only check; no PA fetch. Caller
	// supplies workload.Labels via the WorkloadNode and the workload's
	// namespace labels separately. Returns nil when this source has nothing
	// to say about the workload.
	Membership(workload models.WorkloadNode, nsLabels map[string]string) *models.MeshMembership

	// ResolveMtls fetches PAs for the workload's namespace + root namespace,
	// walks the precedence hierarchy, and returns the resolved MtlsState.
	// nsLabels gates mesh enrollment. One k8s round-trip per call (no
	// app-level cache).
	ResolveMtls(ctx context.Context, workload models.WorkloadNode, nsLabels map[string]string) (*models.MtlsState, error)

	// CanReach decides whether mesh-layer policy permits src → dst. port=0
	// means "any port" — use workload-level verdict; otherwise check the
	// port-level override first. Two ResolveMtls calls per invocation.
	CanReach(srcMesh models.MeshMembership, dstMesh models.MeshMembership, port uint32) models.MeshVerdict

	// BuildMeshMembership eagerly resolves MeshMembership for every workload
	// in the ns index and returns cluster-wide MeshMetrics alongside. Root PA
	// fetched once; ns PA fetched once per ns. Called from store.PopulateCache
	// to stamp cache.MeshMembership + cache.MeshMetrics.
	BuildMeshMembership(nsIndex map[string]models.NSIndex) (models.MeshBuildResult, error)

	Name() string
}

