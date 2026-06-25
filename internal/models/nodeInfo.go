package models

// NodeRule mirrors Rule but resolves DstID to a human-readable label
// and namespace via the cache's NSIndex. SrcID is dropped — the clicked node
// is always the source for outbound rules.
type NodeRule struct {
	Direction    Direction   `json:"direction"`
	Ports        []Port      `json:"ports"`
	L7Match      *L7Match    `json:"l7Match,omitempty"`
	Action       RuleAction  `json:"action"`
	Contributor  PolicyRef   `json:"contributor,omitempty"`
	DstID        string      `json:"dstId"`
	DstLabel     string      `json:"dstLabel,omitempty"`
	DstNamespace string      `json:"dstNamespace,omitempty"`
}

// NodeInfo bundles per-engine data for one workload: rules where the node is
// the source, plus the full list of policies that select the node (including
// policies that select but emit no rule). One entry per engine.
type NodeInfo struct {
	Rules    []NodeRule  `json:"rules"`
	Policies []PolicyRef `json:"policies"`
}
