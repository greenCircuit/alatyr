package graph


// WorkloadNode mirrors the UI WorkloadNode interface.
type WorkloadNode struct {
	ID        string            `json:"id"`
	Label     string            `json:"label"`
	Namespace string            `json:"namespace"`
	Type      NodeType          `json:"type"`
	Labels    map[string]string `json:"labels"`
	Statuses  []StatusKey       `json:"statuses,omitempty"`
}

// NodeType mirrors WorkloadNode.type.
type NodeType string

const (
	NodeTypeService    NodeType = "service"    // deployment + ClusterIP service
	NodeTypeDeployment NodeType = "deployment" // pod/deployment with no service exposure
	NodeTypeHeadless   NodeType = "headless"   // headless service (direct pod addressing)
	NodeTypeExternal   NodeType = "external"   // traffic origin outside the cluster
	NodeTypeNamespace  NodeType = "namespace"  // traffic targets entire ns 
)


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