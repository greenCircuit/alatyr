package store

import (
	"graph/internal/models"
)

// things that are caching to rebuild data
type Cache struct {
	EvaluationResults   map[string]models.EvaluationResult
	NsIndex				map[string]models.NSIndex	
}

// NodeRule mirrors models.Rule but resolves DstID to a human-readable label
// and namespace via the cache's NSIndex. SrcID is dropped — the clicked node
// is always the source for outbound rules.
type NodeRule struct {
	Direction    models.Direction   `json:"direction"`
	Port         models.Port        `json:"port"`
	L7Match      *models.L7Match    `json:"l7Match,omitempty"`
	Action       models.RuleAction  `json:"action"`
	Contributors []models.PolicyRef `json:"contributors,omitempty"`
	DstID        string             `json:"dstId"`
	DstLabel     string             `json:"dstLabel,omitempty"`     // empty for CIDR / unresolved IDs
	DstNamespace string             `json:"dstNamespace,omitempty"`
}

// NodeInfo bundles per-engine data for one workload: rules where the node is
// the source, plus the full list of policies that select the node (including
// policies that select but emit no rule). One entry per engine.
type NodeInfo struct {
	Rules    []NodeRule         `json:"rules"`
	Policies []models.PolicyRef `json:"policies"`
}

// structs to figure out if 2 nodes have can talk to each other
type ReachabilityResult struct {
	Verdict string                   `json:"verdict"` // "allow" | "deny" | "partial"
	Engines map[string]EngineVerdict `json:"engines"` // "k8s" -> ..., "istio" -> ...
	Path    string                   `json:"path,omitempty"`
	Reason  string                   `json:"reason"`
}

type EngineVerdict struct {
	Status      string             `json:"status"` // "allow" | "deny" | "not enforced"
	Egress      DirectionVerdict   `json:"egress"`
	Ingress     DirectionVerdict   `json:"ingress"`
	SrcPolicies []models.PolicyRef `json:"srcPolicies,omitempty"`
	DstPolicies []models.PolicyRef `json:"dstPolicies,omitempty"`
}

type DirectionVerdict struct {
	Locked       bool       `json:"locked"`
	AllowMatches []NodeRule `json:"allowMatches,omitempty"`
	DenyMatches  []NodeRule `json:"denyMatches,omitempty"`
	Reason       string     `json:"reason"`
}                                                                                                     
		