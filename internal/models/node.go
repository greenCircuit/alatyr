package models

type WorkloadNode struct {
	ID        string            `json:"id"`
	Label     string            `json:"label"`
	Namespace string            `json:"namespace"`
	Type      NodeType          `json:"type"`
	Labels    map[string]string `json:"labels"`
	Statuses []StatusKey `json:"statuses,omitempty"`
	StatusesBySource map[string][]StatusKey `json:"statusesBySource,omitempty"`
}

// NodeType mirrors WorkloadNode.type.
type NodeType string

const (
	NodeTypeService    NodeType = "service"    // deployment + ClusterIP service
	NodeTypeDeployment NodeType = "deployment" // pod/deployment with no service exposure
	NodeTypeHeadless   NodeType = "headless"   // headless service (direct pod addressing)
	NodeTypeExternal   NodeType = "external"   // traffic origin outside the cluster
	NodeTypeNamespace  NodeType = "namespace"  // traffic targets entire ns
	NodeTypeCronJob    NodeType = "cronjob"    // cronjob — shown when actively running
)

// NSIndex bundles per-namespace state passed from the graph layer down to
// each policy engine. Workloads includes the synthetic namespace node (Type
// NodeTypeNamespace) appended last by buildNsIndex; LabelIndex is built from
// the same slice so pointer lookups remain valid.
type NSIndex struct {
	NSNode     *WorkloadNode
	LabelIndex map[string][]*WorkloadNode // indexed pointers into Workloads
	Workloads  []WorkloadNode             // workloads + the trailing namespace node
}