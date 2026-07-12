package models

// NodeRule mirrors Rule but resolves both endpoint IDs to human-readable
// labels, namespaces, and kinds via the workload ID index. Node-info payloads
// only care about the dst side (clicked node is the source); reachability
// payloads need both — ingress rules identify the peer by SrcID.
type NodeRule struct {
	Direction    Direction   `json:"direction"`
	Ports        []Port      `json:"ports"`
	L7Match      *L7Match    `json:"l7Match,omitempty"`
	Action       RuleAction  `json:"action"`
	Contributor  PolicyRef   `json:"contributor,omitempty"`
	SrcID        string      `json:"srcId,omitempty"`
	SrcLabel     string      `json:"srcLabel,omitempty"`
	SrcNamespace string      `json:"srcNamespace,omitempty"`
	SrcKind      NodeType    `json:"srcKind,omitempty"`
	DstID        string      `json:"dstId"`
	DstLabel     string      `json:"dstLabel,omitempty"`
	DstNamespace string      `json:"dstNamespace,omitempty"`
	DstKind      NodeType    `json:"dstKind,omitempty"`
	SrcSelector  PolicySelector  `json:"srcSelector,omitempty"`
	DstSelector  PolicySelector  `json:"dstSelector,omitempty"`
}

// NodeInfo bundles per-engine data for one workload: rules where the node is
// the source, plus the full list of policies that select the node (including
// policies that select but emit no rule). One entry per engine.
type NodeInfo struct {
	Rules    []NodeRule  `json:"rules"`
	Policies []PolicyRef `json:"policies"`
}
