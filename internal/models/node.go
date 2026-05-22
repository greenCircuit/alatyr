package models


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
	NodeTypeCronJob    NodeType = "cronjob"    // cronjob — shown when actively running
)

type NSIndex struct {
      NSNode     *WorkloadNode
      LabelIndex map[string][]*WorkloadNode  // workloads only
      Workloads  []WorkloadNode             // ← NEW: flat slice, no NS node
  }