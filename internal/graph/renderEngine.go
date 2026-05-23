package graph

import (
	"graph/internal/models"
	"graph/internal/policy"
)

// foldTuplesToEdges collapses pod-level Rules into PolicyEdges.
// Tuples are grouped by (srcID, dstID, direction, contributing policy).
// Ports for the same group are merged; level is determined by whether either
// endpoint is a namespace node.
func renderEdges(rules []policy.Rule, nodes []models.WorkloadNode) []PolicyEdge {
	nsNodeIDs := map[string]bool{}
	for _, node := range nodes {
		if node.Type == models.NodeTypeNamespace {
			nsNodeIDs[node.ID] = true
		}
	}

	type edgeKey struct {
		srcID, dstID    string
		direction       models.Direction
		policyName      string
		policyNamespace string
	}

	grouped := map[edgeKey]*PolicyEdge{}
	for _, rule := range rules{
		for _, ref := range rule.Contributors {
			key := edgeKey{
				srcID:           rule.SrcID,
				dstID:           rule.DstID,
				direction:       rule.Direction,
				policyName:      ref.Name,
				policyNamespace: ref.Namespace,
			}
			edge, exists := grouped[key]
			if !exists {
				edge = &PolicyEdge{
					Source:     rule.SrcID,
					Target:     rule.DstID,
					Direction:  rule.Direction,
					PolicyName: ref.Name,
					Namespace:  ref.Namespace,
					Level:      edgeLevelFor(rule.SrcID, rule.DstID, nsNodeIDs),
				}
				grouped[key] = edge
			}
			edge.Ports = appendUniquePort(edge.Ports, rule.Port)
		}
	}

	out := make([]PolicyEdge, 0, len(grouped))
	for _, edge := range grouped {
		out = append(out, *edge)
	}
	return out
}

func edgeLevelFor(srcID, dstID string, nsNodeIDs map[string]bool) models.EdgeLevel {
	if nsNodeIDs[srcID] || nsNodeIDs[dstID] {
		return models.EdgeLevelNamespace
	}
	return models.EdgeLevelWorkload
}

func appendUniquePort(ports []models.Port, candidate models.Port) []models.Port {
	for _, existing := range ports {
		if existing == candidate {
			return ports
		}
	}
	return append(ports, candidate)
}

// updateStatusKeys populates every workload's effective Statuses (intersection
// across all engines) and per-engine StatusesBySource (used by the detail
// panel). statusBySource is keyed first by workload ID, then by PolicySource.Name().
func updateStatusKeys(nodes []models.WorkloadNode, statusBySource map[string]map[string]models.PolicyStatus) []models.WorkloadNode {
	for index := range nodes {
		node := &nodes[index]
		bySource := statusBySource[node.ID]
		if len(bySource) == 0 {
			continue
		}

		perEngineKeys := make(map[string][]models.StatusKey, len(bySource))
		perEngineStatus := make([]models.PolicyStatus, 0, len(bySource))
		for sourceName, status := range bySource {
			perEngineKeys[sourceName] = policy.DeriveStatusKeys(status)
			perEngineStatus = append(perEngineStatus, status)
		}
		node.StatusesBySource = perEngineKeys

		effective := policy.IntersectPolicyStatus(perEngineStatus)
		node.Statuses = policy.DeriveStatusKeys(effective)
	}
	return nodes
}
