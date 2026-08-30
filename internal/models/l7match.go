package models

// L7Match captures L7 matcher fields from a policy rule's operation block.
// Populated by engines that emit L7 rules (istio); nil for L3-only engines.
type L7Match struct {
	Hosts      []string `json:"hosts,omitempty"`
	Methods    []string `json:"methods,omitempty"`
	Paths      []string `json:"paths,omitempty"`
	NotHosts   []string `json:"notHosts,omitempty"`
	NotMethods []string `json:"notMethods,omitempty"`
	NotPaths   []string `json:"notPaths,omitempty"`
}

// IsEmpty reports whether no L7 matchers are set. Used to decide whether a
// rule's L7Match pointer should be nil (pure L3) vs populated.
func (l L7Match) IsEmpty() bool {
	return len(l.Hosts)+len(l.Methods)+len(l.Paths)+
		len(l.NotHosts)+len(l.NotMethods)+len(l.NotPaths) == 0
}
