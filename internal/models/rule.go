package models

import "time"

// Rule is an engine-emitted allow/deny between two workloads.
// Produced by every PolicySource implementation; consumed by graph rendering.
type Rule struct {
	SrcID       string     `json:"srcId"`
	DstID       string     `json:"dstId"`
	Ports       []Port     `json:"ports"`
	Direction   Direction  `json:"direction"`
	Contributor PolicyRef  `json:"contributor,omitempty"`
	L7Match     *L7Match   `json:"l7Match,omitempty"` // nil for L3-only engines (k8s); optional for istio
	Action      RuleAction `json:"action"`            // zero-value = ActionAllow; istio DENY policies stamp ActionDeny
	AllPorts    bool       `json:"allPorts"`
	AllL7       bool       `json:"allL7"`
	Coverage    Coverage   `json:"coverage,omitempty"` // "" | "deny-all" | "allow-all"
	// NamespaceWide — the winning policy selects EVERY workload in the namespace
	// by construction (catch-all selector), not by coincidence of today's pod set.
	// Engine-asserted intent; the graph layer needs it to collapse fan-out edges
	// without claiming namespace scope a label selector never expressed.
	NamespaceWide bool           `json:"namespaceWide,omitempty"`
	SrcSelector   PolicySelector `json:"srcSelector"`
	DstSelector   PolicySelector `json:"dstSelector"`
}

type RuleAction int

const (
	ActionAllow RuleAction = iota
	ActionDeny
)

type Coverage string

const (
	CoverageDenyAll    Coverage = "deny all"
	CoverageAllowAll   Coverage = "allow all"
	CoverageAllowAllNs Coverage = "allow all ns"
	CoverageRestricted Coverage = "restricted"
	CoverageUnenforced Coverage = "unenforced"
	CoverageAudit      Coverage = "audit"
	CoverageExcept     Coverage = "except" // carve-out inside an allow (k8s ipBlock.except); render distinct from standalone deny
)

// PolicyRef points back to a specific policy (and rule within it) that
// contributed to a Rule. Rendered in the detail panel.
// Order/Tier are Calico-only; other engines leave them zero. CreatedAt is
// populated by all engines from the CRD's ObjectMeta.CreationTimestamp —
// pointer so a nil marshals to omitted rather than year 0001.
type PolicyRef struct {
	Source    string     `json:"source"`
	Name      string     `json:"name"`
	Namespace string     `json:"namespace"`
	Action    string     `json:"action,omitempty"` // "allow" | "deny" | "" (unknown / k8s allow-style)
	Direction Direction  `json:"direction,omitempty"`
	Order     *float64   `json:"order,omitempty"`     // Calico precedence; nil = unset (0 is a valid order)
	Tier      string     `json:"tier,omitempty"`      // Calico tier name
	CreatedAt *time.Time `json:"createdAt,omitempty"` // policy CreationTimestamp
}

// all different ways policy can select src and dst workloads
// will show show why workloads belong to policy
type PolicySelector struct {
	LabelSelector map[string]string `json:"labelSelector,omitempty"`
	NsSelector    map[string]string `json:"nsSelector,omitempty"`
	Namespaces    []string          `json:"namespaces,omitempty"`
}
