package policy

import (
	"context"

	"graph/internal/models"
)

type PolicySource interface {
	// Stable identifier. Used as map key in Edge.Contributors and as
	// value in the API ?policySource= param.
	Name() string

	// Fetch all policy objects for the requested namespaces.
	// Implementations cache results internally; subsequent calls in the
	// same request are no-ops.
	Fetch(ctx context.Context, namespaces []string) error

	// Coverage reports whether this source is enforcing on the given
	// workload in the given direction.
	//   DefaultAllow  — no policy of this source selects the workload+dir
	//   DefaultDeny   — at least one policy selects it; only explicit
	//                   AllowTuples will produce edges
	//   NotApplicable — source cannot enforce here at all
	//                   (e.g. Istio + out-of-mesh workload)
	Coverage(node *models.WorkloadNode, dir models.Direction) CoverageMode

	// AllowTuples returns explicit allow rules expanded to
	// pod-level tuples. Only meaningful for workloads with
	// Coverage == DefaultDeny.
	AllowRules(nodes []models.WorkloadNode) []Rule

	// DenyTuples returns explicit deny rules (Istio DENY action, etc.).
	// Subtracted from the intersection result regardless of which
	// source allowed the tuple.
	DenyRules(nodes []models.WorkloadNode) []Rule

	// StatusKeys reports per-workload diagnostics owned by this source.
	// Aggregated by the graph layer onto WorkloadNode.Statuses.
	StatusKeys(nodes []models.WorkloadNode) []StatusKeyAssignment
}

type Rule struct {
	SrcID		 string
	DstID 		 string
	Port         models.Port
	Direction    models.Direction
	Contributors []PolicyRef
}

type PolicyRef struct {
	Source    string
	Name      string
	Namespace string
	RuleIndex int
}

type CoverageMode int

const (
	CoverageDefaultAllow CoverageMode = iota
	CoverageDefaultDeny
	CoverageNotApplicable
)

type StatusKeyAssignment struct {
	WorkloadID string
	Key        models.StatusKey
}
