package store

import (
	"graph/internal/models"
)

// BuildNodeNeighbor returns a node's policy neighbors per engine: workloads the
// node is a source to (Out) and a destination from (In). Walks every namespace
// bucket — a node's cross-namespace rules land in the policy's ns bucket, not
// the node's own, so scoping to one ns silently drops cross-ns neighbors.
//
// Adjacency, not reachability: allow + deny both emitted (each NeighborRef
// carries rule.Action). No dedup — every rule touching a pair is its own
// NeighborRef, so different ports/policies/actions on the same pair survive.
// Unresolved ids (external CIDR) keep the neighbor with a blank Workload rather
// than dropping it.
func BuildNodeNeighbor(cache *models.Cache, nodeId string) map[string]NodeNeighbors {
	idIndex := buildWorkloadIDIndex(cache) // resolve any id, any ns; built once
	neighborPolicyMap := make(map[string]NodeNeighbors)

	for policyEngine, result := range cache.EvaluationResults {
		var neighbors NodeNeighbors

		collect := func(rules []models.Rule) {
			for _, rule := range rules {
				if rule.SrcID == rule.DstID {
					continue // self-loop: intra-ns policy selecting its own peer
				}
				if rule.DstID == nodeId {
					neighbors.In = append(neighbors.In,
						NeighborRef{Rule: rule, Workload: idIndex[rule.SrcID]})
				}
				if rule.SrcID == nodeId {
					neighbors.Out = append(neighbors.Out,
						NeighborRef{Rule: rule, Workload: idIndex[rule.DstID]})
				}
			}
		}

		for _, rules := range result.AllowByNs {
			collect(rules)
		}
		for _, rules := range result.DenyByNs {
			collect(rules)
		}
		neighborPolicyMap[policyEngine] = neighbors
	}
	return neighborPolicyMap
}
