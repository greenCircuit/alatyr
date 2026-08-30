package istio

type policySignals struct {
	namespaces    map[string]bool // explicit ns entries (from source.namespaces)
	notNamespaces map[string]bool // inverted ns entries (from source.notNamespaces)
	ipBlocks      map[string]bool // explicit CIDR entries (from source.ipBlocks)
	notIpBlocks   map[string]bool // inverted CIDR entries (from source.notIpBlocks)
	ports         map[string]bool // explicit port entries (from to.operation.ports)
	notPorts      map[string]bool // inverted port entries (from to.operation.notPorts)
}

func newPolicySignals() policySignals {
	return policySignals{
		namespaces:    map[string]bool{},
		notNamespaces: map[string]bool{},
		ipBlocks:      map[string]bool{},
		notIpBlocks:   map[string]bool{},
		ports:         map[string]bool{},
		notPorts:      map[string]bool{},
	}
}