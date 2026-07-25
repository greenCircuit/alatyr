package models

type WorkloadNode struct {
	ID        string            `json:"id"`
	Label     string            `json:"label"`
	Namespace string            `json:"namespace"`
	Type      NodeType          `json:"type"`
	Labels    map[string]string `json:"labels"`
	Statuses []StatusKey `json:"statuses,omitempty"`
	CidrType  CidrType   `json:"cidrType"`
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
	NodeTypeCIDR       NodeType = "cidr"       // synthetic node for a k8s NetworkPolicy ipBlock CIDR peer
)

type CidrType string

const (
	CIDRk8sSvc CidrType = "scv CIDR"   
	CIDRk8sPod CidrType = "pod CIDR"   
	CIDRLan    CidrType = "lan CIDR"   
	CIDRWan    CidrType = "wan CIDR"   
)


// CIDRIDPrefix namespaces synthetic CIDR node IDs so they can never collide
// with k8s UIDs (GUIDs) or the "ns-<name>" scheme used by namespace nodes.
// Shared across engines and store so ID construction and matching stay in sync.
const CIDRIDPrefix = "cidr:"

// NSIndex bundles per-namespace state passed from the graph layer down to
// each policy engine. Workloads includes the synthetic namespace node (Type
// NodeTypeNamespace) appended last by buildNsIndex; LabelIndex is built from
// the same slice so pointer lookups remain valid.
type NSIndex struct {
	NSNode     *WorkloadNode
	LabelIndex map[string][]*WorkloadNode // indexed pointers into Workloads
	Workloads  []WorkloadNode             // workloads + the trailing namespace node
}