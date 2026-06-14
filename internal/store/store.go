package store

import (
	"graph/internal/models"
)

// things that are caching to rebuild data
type Cache struct {
	EvaluationResults   map[string]models.EvaluationResult
	NsIndex				map[string]models.NSIndex	
}

// structs to figure out if 2 nodes have can talk to each other
type ReachabilityResult struct {
	Verdict string                            `json:"verdict"` // "allow" | "deny" | "partial"
	Engines map[string]EngineVerdict          `json:"engines"` // "k8s" -> ..., "istio" -> ...
	Mesh    map[string]models.MeshVerdict     `json:"mesh,omitempty"`    // per mesh source — cross-node verdict
	SrcMesh map[string]*models.MeshMembership `json:"srcMesh,omitempty"` // src workload's mesh state per source
	DstMesh map[string]*models.MeshMembership `json:"dstMesh,omitempty"` // dst workload's mesh state per source
	Path    string                            `json:"path,omitempty"`
	Reason  string                            `json:"reason"`
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
	AllowMatches []models.NodeRule `json:"allowMatches,omitempty"`
	DenyMatches  []models.NodeRule `json:"denyMatches,omitempty"`
	Reason       string     `json:"reason"`
}                                                                                                     
		