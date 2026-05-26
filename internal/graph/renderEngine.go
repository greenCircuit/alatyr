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

	// edgeKey includes policySource so identically-named policies from
	// different engines (e.g. k8s "allow-ingress" + istio "allow-ingress")
	// stay as separate edges instead of collapsing.
	type edgeKey struct {
		srcID, dstID    string
		direction       models.Direction
		policyName      string
		policyNamespace string
		policySource    string
		action          policy.RuleAction
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
				policySource:    ref.Source,
				action:          rule.Action,
			}
			edge, exists := grouped[key]
			if !exists {
				edge = &PolicyEdge{
					Source:       rule.SrcID,
					Target:       rule.DstID,
					Direction:    rule.Direction,
					PolicyName:   ref.Name,
					Namespace:    ref.Namespace,
					Level:        edgeLevelFor(rule.SrcID, rule.DstID, nsNodeIDs),
					PolicySource: ref.Source,
					Action:       rule.Action,
				}
				grouped[key] = edge
			}
			edge.Ports = appendUniquePort(edge.Ports, rule.Port)
			if rule.L7Match != nil {
				edge.L7Matches = appendUniqueL7(edge.L7Matches, *rule.L7Match)
			}
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

// appendUniqueL7 deduplicates L7Match blocks across rules that fold into the
// same edge. Fan-out per port (case 9) attaches the same L7 set to N rules;
// without dedup the detail panel would show N copies of identical L7 data.
func appendUniqueL7(existing []policy.L7Match, candidate policy.L7Match) []policy.L7Match {
	for _, block := range existing {
		if l7Equal(block, candidate) {
			return existing
		}
	}
	return append(existing, candidate)
}

func l7Equal(a, b policy.L7Match) bool {
	return stringsEqual(a.Hosts, b.Hosts) &&
		stringsEqual(a.Methods, b.Methods) &&
		stringsEqual(a.Paths, b.Paths) &&
		stringsEqual(a.NotHosts, b.NotHosts) &&
		stringsEqual(a.NotMethods, b.NotMethods) &&
		stringsEqual(a.NotPaths, b.NotPaths)
}

func stringsEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for index := range a {
		if a[index] != b[index] {
			return false
		}
	}
	return true
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
