package graph

import "graph/internal/models"

// PolicyEdge mirrors the UI PolicyEdge interface.
type PolicyEdge struct {
	ID         string           `json:"id"`
	Source     string           `json:"source"`
	Target     string           `json:"target"`
	Direction  models.Direction `json:"direction"`
	PolicyName string           `json:"policyName"`
	Namespace  string           `json:"namespace"`
	Level      models.EdgeLevel `json:"level"`
	Ports      []models.Port    `json:"ports,omitempty"`
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
