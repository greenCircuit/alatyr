package graph

import (
	"graph/internal/models"
	"graph/internal/policy"
)

// PolicyEdge mirrors the UI PolicyEdge interface.
type PolicyEdge struct {
	ID           string            `json:"id"`
	Source       string            `json:"source"`       // src workload/ns node id
	Target       string            `json:"target"`       // dst workload/ns node id
	Direction    models.Direction  `json:"direction"`
	PolicyName   string            `json:"policyName"`
	Namespace    string            `json:"namespace"`
	Level        models.EdgeLevel  `json:"level"`
	Ports        []models.Port     `json:"ports,omitempty"`
	PolicySource string            `json:"policySource"` // engine that produced this edge (e.g. "k8s", "istio")
	L7Matches    []policy.L7Match  `json:"l7Matches,omitempty"` // accumulated L7 blocks; empty for pure-L3 edges
	Action       policy.RuleAction `json:"action"`       // 0 = Allow, 1 = Deny
}

// Bundle mirrors the UI Bundle interface used for edge aggregation.
// Multiple PolicyEdges between the same source/target pair are collapsed into one Bundle.
type Bundle struct {
	ID        string           `json:"id"`
	Source    string           `json:"source"`
	Target    string           `json:"target"`
	Policies  []PolicyEdge     `json:"policies"`
	HasNS     bool             `json:"hasNS"`
	Direction models.Direction `json:"direction"`
}

// Graph is the top-level structure returned to the UI.
type Graph struct {
	Nodes []models.WorkloadNode `json:"nodes"`
	Edges []PolicyEdge          `json:"edges"`
}
