package models

type IssueType string

const (
	NoDNSEgress 	 	  IssueType = "no dns"
	MeshMisconfig    	  IssueType = "mesh policy"			   // PA duplicates, root-selector-ignored, unset fallbacks
	MeshTransportBlocked  IssueType = "mesh transport blocked" // L3 policy strips a port the mesh dataplane needs (HBONE 15008, future sidecar ports)
	PolicyConflicts  	  IssueType = "policy conflict"		   // multiple polices one allow other deny
	MeshConflicts    	  IssueType = "mesh conflict"		   // have edge, but node outside mesh want to talk to node is strict mesh
	NodeLockOut      	  IssueType = "node lockout"		   // no has all egress/ingress deny all policy so it can't really talk to anyone
	IssuesPartial	 	  IssueType = "partial access"         // no has all egress/ingress deny all policy so it can't really talk to anyone
	// One engine allows a CIDR strictly inside the range another engine allows
	// (a /32 host under a /24). Scope layering, not deny-vs-allow — but the API
	// objects can't say whether the narrowing was deliberate or a fat-fingered
	// mask, so it stays visible and filterable instead of demoted to info.
	IssuesCidrScope       IssueType = "cidr scope mismatch"
	IssuesFailedToFetch   IssueType=  "failed to fetch"         // no has all egress/ingress deny all policy so it can't really talk to anyone
)

type IssueSeverity string

const (
	IssueSeverityCritical IssueSeverity = "critical"
	IssueSeverityHigh     IssueSeverity = "high"
	IssueSeverityWarning  IssueSeverity = "warning"
	IssueSeverityCaution  IssueSeverity = "caution"
	IssueSeverityInfo     IssueSeverity = "info"
	IssueSeveritySecure   IssueSeverity = "secure"
)

// mapping issue types to severity levels
var SeverityByType = map[IssueType]IssueSeverity{
	NodeLockOut:          IssueSeverityHigh,
	PolicyConflicts:      IssueSeverityHigh,
	MeshConflicts:        IssueSeverityHigh,
	MeshTransportBlocked: IssueSeverityHigh,
	IssuesFailedToFetch:  IssueSeverityHigh,
	// warning
	NoDNSEgress:          IssueSeverityWarning,
	IssuesCidrScope:      IssueSeverityWarning,
	// info
	IssuesPartial:        IssueSeverityInfo,
	MeshMisconfig:        IssueSeverityInfo,
}

func SeverityForType(issueType IssueType) IssueSeverity {
	if severity, ok := SeverityByType[issueType]; ok {
		return severity
	}
	return IssueSeverityHigh
}

type Issue struct {
	Type     IssueType     `json:"type"`
	Severity IssueSeverity `json:"severity"`
	Message  string        `json:"message"`
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
	SrcMembership *MeshMembership `json:"srcMembership,omitempty"`
	DstMembership *MeshMembership `json:"dstMembership,omitempty"`
	// Culprits is for node-scoped findings with no ingress/egress direction
	// (e.g. mesh policy hygiene: duplicate PeerAuthentications on one node) —
	// IngressCulprits/EgressCulprits would misname a fix side that doesn't exist.
	Culprits []PolicyRef `json:"culprits,omitempty"`
}
