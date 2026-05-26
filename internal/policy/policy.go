package policy

import (
	"context"

	"graph/internal/models"
)

type PolicySource interface {
	// Stable identifier. Used as map key in Edge.Contributors and as
	// value in the API ?policySource= param.
	Name() string

	// Evaluate fetches policy data for the given namespaces and returns all
	// allow/deny rules and status keys. Fetch strategy is an internal detail.
	// Returns an error when policy data cannot be fetched — callers must
	// surface this rather than render an empty graph (silent data loss).
	Evaluate(ctx context.Context, namespaces []string, index map[string]models.NSIndex) (EvaluationResult, error)
}

type EvaluationResult struct {
	Allow          []Rule
	Deny           []Rule
	PolicyStatuses map[string]models.PolicyStatus // workloadID → per-engine PolicyStatus
}

type Rule struct {
	SrcID		 string
	DstID 		 string
	Port         models.Port
	Direction    models.Direction
	Contributors []PolicyRef
	L7Match      *L7Match   // nil for L3-only engines (k8s); optional for istio
	Action       RuleAction // zero-value = ActionAllow; istio DENY policies stamp ActionDeny
}

type RuleAction int

const (
	ActionAllow RuleAction = iota
	ActionDeny
)

// L7Match captures L7 matcher fields from a policy rule's operation block.
// Populated by engines that emit L7 rules (istio); nil for L3-only engines.
type L7Match struct {
	Hosts      []string `json:"hosts,omitempty"`
	Methods    []string `json:"methods,omitempty"`
	Paths      []string `json:"paths,omitempty"`
	NotHosts   []string `json:"notHosts,omitempty"`
	NotMethods []string `json:"notMethods,omitempty"`
	NotPaths   []string `json:"notPaths,omitempty"`
}

// IsEmpty reports whether no L7 matchers are set. Used to decide whether a
// rule's L7Match pointer should be nil (pure L3) vs populated.
func (l L7Match) IsEmpty() bool {
	return len(l.Hosts)+len(l.Methods)+len(l.Paths)+
		len(l.NotHosts)+len(l.NotMethods)+len(l.NotPaths) == 0
}

type PolicyRef struct {
	Source    string
	Name      string
	Namespace string
	RuleIndex int
}
