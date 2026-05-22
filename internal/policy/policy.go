package policy

import (
	"context"

	"graph/internal/models"
)

type PolicySource interface {
	// Stable identifier. Used as map key in Edge.Contributors and as
	// value in the API ?policySource= param.
	Name() string

	// Coverage reports whether this source enforces on the given workload+direction.
	//   DefaultAllow  — no policy selects the workload; traffic flows freely
	//   DefaultDeny   — at least one policy selects it; only Evaluate() rules produce edges
	//   NotApplicable — source cannot enforce here (e.g. Istio + out-of-mesh workload)
	Coverage(node *models.WorkloadNode, dir models.Direction) CoverageMode

	// Evaluate fetches policy data for the given namespaces and returns all
	// allow/deny rules and status keys. Fetch strategy is an internal detail.
	Evaluate(ctx context.Context, namespaces []string, index map[string]models.NSIndex) EvaluationResult
}

type EvaluationResult struct {
	Allow      []Rule
	Deny       []Rule
	StatusKeys []StatusKeyAssignment
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
