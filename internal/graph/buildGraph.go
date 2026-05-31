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
	for _, ns := range namespaces {
		nsIndex, ok := cache.NsIndex[ns]
		if !ok {
			continue
		}
		allNodes = append(allNodes, nsIndex.Workloads...)
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
	}

	allNodes = UpdateStatusKeys(allNodes, statusBySourcePerNode)
	edges := RenderEdges(allRules, allNodes)

	return Graph{Nodes: allNodes, Edges: edges}
}
