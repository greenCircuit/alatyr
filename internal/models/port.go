package models

// Port mirrors the anonymous port object in PolicyEdge.ports.
// Name is set for named ports (e.g. "http") and is not resolved against pod specs.
// EndPort is set when the rule specifies a range (port..endPort inclusive).
type Port struct {
	Port     int    `json:"port"`
	EndPort  int    `json:"endPort,omitempty"`
	Name     string `json:"name,omitempty"`
	Protocol string `json:"protocol"`
}
