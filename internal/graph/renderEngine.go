package graph

import (
	"graph/internal/models"
	"graph/internal/policy"
)

// foldTuplesToEdges collapses pod-level Rules into PolicyEdges.
// Used to render arrow in graphs, to eliminate duplicates ports or l7 rules
func RenderEdges(rules []models.Rule, nodes []models.WorkloadNode) []PolicyEdge {
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
		action          models.RuleAction
	}

	grouped := map[edgeKey]*PolicyEdge{}

	for _, rule := range rules {
		key := edgeKey{
			srcID:           rule.SrcID,
			dstID:           rule.DstID,
			direction:       rule.Direction,
			policyName:      rule.Contributor.Name,
			policyNamespace: rule.Contributor.Namespace,
			policySource:    rule.Contributor.Source,
			action:          rule.Action,
		}
		edge, exists := grouped[key]
		if !exists {
			edge = &PolicyEdge{
				Source:       rule.SrcID,
				Target:       rule.DstID,
				Direction:    rule.Direction,
				PolicyName:   rule.Contributor.Name,
				Namespace:    rule.Contributor.Namespace,
				Level:        edgeLevelFor(rule.SrcID, rule.DstID, nsNodeIDs),
				PolicySource: rule.Contributor.Source,
				Action:       rule.Action,
			}
			grouped[key] = edge
		}

		// Edge dedup is keyed on port number, or l7 policy only
		// this is just so arrows have right information
		if rule.L7Match != nil {
			edge.L7Matches = appendUniqueL7(edge.L7Matches, *rule.L7Match)
		}
		if len(rule.Ports) != 0 {
			edge.Ports = appendUniquePort(edge.Ports, rule.Ports)
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

func appendUniquePort(ports []models.Port, candidates []models.Port) []models.Port {
	seen := make(map[int]bool)
	for _, existing := range ports {
		seen[existing.Port] = true
	}
	for _, candidate := range candidates {
		if seen[candidate.Port] {
			continue
		}
		seen[candidate.Port] = true
		ports = append(ports, candidate)
	}
	return ports
}

// appendUniqueL7 deduplicates L7Match blocks across rules that fold into the
// same edge. Fan-out per port (case 9) attaches the same L7 set to N rules;
// without dedup the detail panel would show N copies of identical L7 data.
func appendUniqueL7(existing []models.L7Match, candidate models.L7Match) []models.L7Match {
	for _, block := range existing {
		if l7Equal(block, candidate) {
			return existing
		}
	}
	return append(existing, candidate)
}

func l7Equal(a, b models.L7Match) bool {
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

// UpdateStatusKeys populates every workload's effective Statuses (intersection
// across all engines) and per-engine StatusesBySource (used by the detail
// panel). statusBySource is keyed first by workload ID, then by PolicySource.Name().
func UpdateStatusKeys(nodes []models.WorkloadNode, statusBySource map[string]map[string]models.PolicyStatus) []models.WorkloadNode {
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
