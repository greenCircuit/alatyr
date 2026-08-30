package models

import "maps"

// Cache holds per-namespace state reused across graph builds.
// NsIndex skips the k8s pod/cronjob fetch on cache hit; EvaluationResults
// skips per-engine policy evaluation. Populated by store, read by graph.
type Cache struct {
	NsIndex           map[string]NSIndex
	EvaluationResults map[string]EvaluationResult // keyed by PolicySource.Name()
	// WorkloadByID resolves any cached node id (workload, cronjob, or ns node)
	// to its WorkloadNode without a namespace hint. Derived from NsIndex —
	// callers that mutate NsIndex must call RebuildWorkloadIndex afterwards.
	WorkloadByID      map[string]WorkloadNode
	MeshMembership	  map[string]MeshMembership
	MeshMetrics       MeshMetrics
	MeshIssues        []Issue
}

// RebuildWorkloadIndex regenerates WorkloadByID from NsIndex so consumers get
// O(1) id lookups instead of scanning every namespace per rule. Not
// goroutine-safe against concurrent readers — call it where NsIndex writes
// already hold the store's write lock.
func (c *Cache) RebuildWorkloadIndex() {
	index := make(map[string]WorkloadNode)
	for _, nsIndex := range c.NsIndex {
		for _, workload := range nsIndex.Workloads {
			index[workload.ID] = workload
		}
	}
	// Engine-synthesized nodes (CIDR peers) live only in eval results, never in
	// NsIndex. Fold them so id lookups resolve external endpoints too. Empty
	// until engines run — safe to call before and after evaluation.
	for _, result := range c.EvaluationResults {
		maps.Copy(index, result.Nodes)
	}
	c.WorkloadByID = index
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
