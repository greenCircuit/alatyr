package graph

// StatusKey mirrors the UI's StatusKey union type — computed by the backend,
// rendered by the UI as icon badges on workload nodes.
type StatusKey string

const (
	StatusInternetIngress  StatusKey = "internet-ingress"
	StatusInternetEgress   StatusKey = "internet-egress"
	StatusNoPolicy         StatusKey = "no-policy"
	StatusIsolated         StatusKey = "isolated"
	StatusOrphanedSelector StatusKey = "orphaned-selector"
	StatusCrossNamespace   StatusKey = "cross-namespace"
	StatusDNSMissing       StatusKey = "dns-missing"
	StatusKubeAPIAccess    StatusKey = "kube-api-access"
	StatusIngressExposed   StatusKey = "ingress-exposed"
)

// NodeType mirrors WorkloadNode.type.
type NodeType string

const (
	NodeTypeService    NodeType = "service"    // deployment + ClusterIP service
	NodeTypeDeployment NodeType = "deployment" // pod/deployment with no service exposure
	NodeTypeHeadless   NodeType = "headless"   // headless service (direct pod addressing)
	NodeTypeExternal   NodeType = "external"   // traffic origin outside the cluster
)

// Direction mirrors PolicyEdge.direction.
type Direction string

const (
	DirectionIngress Direction = "ingress"
	DirectionEgress  Direction = "egress"
	DirectionBoth    Direction = "both"
)

// EdgeLevel mirrors PolicyEdge.level.
type EdgeLevel string

const (
	EdgeLevelWorkload  EdgeLevel = "workload"
	EdgeLevelNamespace EdgeLevel = "namespace"
)

// Port mirrors the anonymous port object in PolicyEdge.ports.
type Port struct {
	Port     int    `json:"port"`
	Protocol string `json:"protocol"`
}

// WorkloadNode mirrors the UI WorkloadNode interface.
type WorkloadNode struct {
	ID        string            `json:"id"`
	Label     string            `json:"label"`
	Namespace string            `json:"namespace"`
	Type      NodeType          `json:"type"`
	Labels    map[string]string `json:"labels"`
	Statuses  []StatusKey       `json:"statuses,omitempty"`
}

// PolicyEdge mirrors the UI PolicyEdge interface.
type PolicyEdge struct {
	ID         string    `json:"id"`
	Source     string    `json:"source"`
	Target     string    `json:"target"`
	Direction  Direction `json:"direction"`
	PolicyName string    `json:"policyName"`
	Namespace  string    `json:"namespace"`
	Level      EdgeLevel `json:"level"`
	Ports      []Port    `json:"ports,omitempty"`
}

// Bundle mirrors the UI Bundle interface used for edge aggregation.
// Multiple PolicyEdges between the same source/target pair are collapsed into one Bundle.
type Bundle struct {
	ID        string       `json:"id"`
	Source    string       `json:"source"`
	Target    string       `json:"target"`
	Policies  []PolicyEdge `json:"policies"`
	HasNS     bool         `json:"hasNS"`
	Direction Direction    `json:"direction"`
}

// Graph is the top-level structure returned to the UI.
type Graph struct {
	Nodes []WorkloadNode `json:"nodes"`
	Edges []PolicyEdge   `json:"edges"`
}
