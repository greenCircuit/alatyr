package graph

import (
	"graph/internal/models"
	"graph/internal/policy"
)

// foldTuplesToEdges collapses pod-level AllowTuples into PolicyEdges.
// Tuples are grouped by (srcID, dstID, direction, contributing policy).
// Ports for the same group are merged; level is determined by whether either
// endpoint is a namespace node.
func foldTuplesToEdges(tuples []policy.Rule, nodes []models.WorkloadNode) []PolicyEdge {
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
	for _, tuple := range tuples {
		for _, ref := range tuple.Contributors {
			key := edgeKey{
				srcID:           tuple.SrcID,
				dstID:           tuple.DstID,
				direction:       tuple.Direction,
				policyName:      ref.Name,
				policyNamespace: ref.Namespace,
			}
			edge, exists := grouped[key]
			if !exists {
				edge = &PolicyEdge{
					Source:     tuple.SrcID,
					Target:     tuple.DstID,
					Direction:  tuple.Direction,
					PolicyName: ref.Name,
					Namespace:  ref.Namespace,
					Level:      edgeLevelFor(tuple.SrcID, tuple.DstID, nsNodeIDs),
				}
				grouped[key] = edge
			}
			edge.Ports = appendUniquePort(edge.Ports, tuple.Port)
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
