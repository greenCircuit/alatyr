package store

import (
	"graph/internal/models"
)

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
	DenyAllMatches 		[]models.NodeRule `json:"denyAllMatches,omitempty"`
	OtherAllowMatches   []models.NodeRule `json:"allowOtherMatches,omitempty"`
	AllowMatches   		[]models.NodeRule `json:"allowMatches,omitempty"`
	DenyMatches    		[]models.NodeRule `json:"denyMatches,omitempty"`
	// ipBlock.except carve-outs. Kept out of DenyMatches so the reason and the
	// remediation don't claim a deny object exists, but ranked above AllowMatches
	// so the covering 0.0.0.0/0 allow can't win the peer back.
	CarveOutMatches		[]models.NodeRule `json:"carveOutMatches,omitempty"`
	Reason         		DirectionReason   `json:"reason"`
	Culprits            []models.PolicyRef `json:"culprits,omitempty"` // what polices broke connection
	Ports 				[]models.Port	  `json:"ports,omitempty"`
	AllPorts			bool			  `json:"allPorts,omitempty"`
}                                                                                                     

type NeighborRef struct {
	Rule     models.Rule
	Workload models.WorkloadNode
}

type NodeNeighbors struct {
	In 		[]NeighborRef
	Out		[]NeighborRef
}

// Canonical definitions live in models (models/reachability.go) so the Issue
// endpoint can type its reason fields without a cyclic import. Aliased here so
// existing store + test references keep compiling unchanged.
type DirectionReason = models.DirectionReason

const (
	ReasonPermitted     = models.ReasonPermitted
	ReasonExplicitDeny  = models.ReasonExplicitDeny
	ReasonCarvedOut     = models.ReasonCarvedOut
	ReasonDefaultDeny   = models.ReasonDefaultDeny
	ReasonLockedNoMatch = models.ReasonLockedNoMatch
	ReasonNoOpinion     = models.ReasonNoOpinion
)