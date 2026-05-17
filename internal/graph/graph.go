package graph


// Direction mirrors PolicyEdge.direction.
type Direction string

const (
	DirectionIngress Direction = "ingress"
	DirectionEgress  Direction = "egress"
	DirectionBoth    Direction = "both"
)

// EdgeLevel mirrors PolicyEdge.level.
type EdgeLevel string


// Port mirrors the anonymous port object in PolicyEdge.ports.
// Name is set for named ports (e.g. "http") and is not resolved against pod specs.
// EndPort is set when the rule specifies a range (port..endPort inclusive).
type Port struct {
	Port     int    `json:"port"`
	EndPort  int    `json:"endPort,omitempty"`
	Name     string `json:"name,omitempty"`
	Protocol string `json:"protocol"`
}

const (
	EdgeLevelWorkload  EdgeLevel = "workload"
	EdgeLevelNamespace EdgeLevel = "namespace"
)

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
