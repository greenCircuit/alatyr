package models

type IssueType string

const (
	NoDNSEgress 	 IssueType = "no dns"
	MeshMisconfig    IssueType = "mesh policy"			// multiple peear auth, missing ports for ingress egress
	PolicyConflicts  IssueType = "policy conflict"		// multiple polices one allow other deny
	MeshConflicts    IssueType = "mesh conflict"		// have edge, but node outside mesh want to talk to node is strict mesh 
	NodeLockOut      IssueType = "node lockout"		    // no has all egress/ingress deny all policy so it can't really talk to anyone
	IssuesPartial	 IssueType = "partial access"       // no has all egress/ingress deny all policy so it can't really talk to anyone
)

type Issue struct {
	Type    IssueType     `json:"type"`
	Message string        `json:"message"`
	IngressCulprits 	  []PolicyRef `json:"ingressCulprits,omitempty"`
	EgressCulprits  	  []PolicyRef `json:"egressCulprits,omitempty"`
	IngressAllowed  	  []PolicyRef `json:"ingressAllowed,omitempty"`
	EgressAllowed   	  []PolicyRef `json:"egressAllowed,omitempty"`
	IngressReason         DirectionReason `json:"ingressReason,omitempty"`
	EgressReason          DirectionReason `json:"egressReason,omitempty"`
	Engine  string        `json:"engine,omitempty"`
	// Edge-scoped issues (policy conflict) carry both endpoints so the UI can
	// open the reachability panel for src→dst. Node-scoped issues (lockout)
	// carry Node instead.
	Src  *WorkloadNode `json:"src,omitempty"`
	Dst  *WorkloadNode `json:"dst,omitempty"`
	Node *WorkloadNode `json:"node,omitempty"`
}