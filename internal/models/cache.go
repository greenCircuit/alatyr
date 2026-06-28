package models

// Cache holds per-namespace state reused across graph builds.
// NsIndex skips the k8s pod/cronjob fetch on cache hit; EvaluationResults
// skips per-engine policy evaluation. Populated by store, read by graph.
type Cache struct {
	NsIndex           map[string]NSIndex
	EvaluationResults map[string]EvaluationResult // keyed by PolicySource.Name()
}

// Workload resolves a node id within a namespace to its cached WorkloadNode and
// the namespace object's labels. Returns (nil, nsLabels) when the ns is cached
// but holds no such workload, and (nil, nil) when the ns is absent (external
// CIDR dst or an unloaded namespace). Single lookup shared across packages.
func (c *Cache) Workload(nodeId, ns string) (*WorkloadNode, map[string]string) {
	nsIndex, ok := c.NsIndex[ns]
	if !ok {
		return nil, nil
	}
	var nsLabels map[string]string
	if nsIndex.NSNode != nil {
		nsLabels = nsIndex.NSNode.Labels
	}
	for i := range nsIndex.Workloads {
		if nsIndex.Workloads[i].ID == nodeId {
			return &nsIndex.Workloads[i], nsLabels
		}
	}
	return nil, nsLabels
}
