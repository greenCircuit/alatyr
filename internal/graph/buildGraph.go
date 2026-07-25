package graph

import (
	"graph/internal/models"
)

// BuildGraph assembles the final Graph from a populated Cache.
// Pure — reads cache.NsIndex for nodes and cache.EvaluationResults for
// rules and per-engine PolicyStatus. Caller must populate the cache for
// every requested namespace via store.PopulateCache before calling this.
// Engine names come from cache.EvaluationResults keys (== PolicySource.Name()).
func BuildGraph(cache *models.Cache, namespaces []string) Graph {
	var allNodes []models.WorkloadNode
	seenNodeIDs := map[string]bool{}
	for _, ns := range namespaces {
		nsIndex, ok := cache.NsIndex[ns]
		if !ok {
			continue
		}
		for _, node := range nsIndex.Workloads {
			if seenNodeIDs[node.ID] {
				continue
			}
			seenNodeIDs[node.ID] = true
			allNodes = append(allNodes, node)
		}
	}

	var allRules []models.Rule
	statusBySourcePerNode := map[string]map[string]models.PolicyStatus{}

	for engineName, result := range cache.EvaluationResults {
		for _, rules := range result.AllowByNs {
			allRules = append(allRules, rules...)
		}
		for _, rules := range result.DenyByNs {
			allRules = append(allRules, rules...)
		}
		for nodeID, status := range result.PolicyStatuses {
			if statusBySourcePerNode[nodeID] == nil {
				statusBySourcePerNode[nodeID] = map[string]models.PolicyStatus{}
			}
			statusBySourcePerNode[nodeID][engineName] = status
		}
		// Engine-synthesized nodes (e.g. CIDR peers). Deduped by ID so the
		// same CIDR referenced from multiple ns / engines collapses to one node.
		for nodeID, node := range result.Nodes {
			if seenNodeIDs[nodeID] {
				continue
			}
			seenNodeIDs[nodeID] = true
			allNodes = append(allNodes, node)
		}
	}

	allNodes = UpdateStatusKeys(allNodes, statusBySourcePerNode)
	edges := RenderEdges(allRules, allNodes)

	return Graph{Nodes: allNodes, Edges: edges}
}
