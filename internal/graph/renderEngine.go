package graph

import (
	"fmt"

	"graph/internal/models"
	"graph/internal/policy"
)

// Which endpoint of a rule holds the workload (the other one is the peer).
const (
	sideSrc = "src"
	sideDst = "dst"
)

// Namespaces with a single workload keep per-workload arrows — collapsing there
// trades precision for nothing.
const minCollapseMembers = 2

// nsMembership indexes real workloads per namespace so the collapse pass can ask
// "did EVERY workload in this namespace resolve the same rule?". Namespace, CIDR
// and external nodes are not members — they're endpoints, not selectable pods.
type nsMembership struct {
	namespaceOf map[string]string // workload node ID → namespace
	memberCount map[string]int    // namespace → workload count
	nsNodeID    map[string]string // namespace → its "ns-<name>" node ID
}

func buildNsMembership(nodes []models.WorkloadNode) nsMembership {
	membership := nsMembership{
		namespaceOf: map[string]string{},
		memberCount: map[string]int{},
		nsNodeID:    map[string]string{},
	}
	for _, node := range nodes {
		switch node.Type {
		case models.NodeTypeNamespace:
			membership.nsNodeID[node.Namespace] = node.ID
		case models.NodeTypeCIDR, models.NodeTypeExternal:
			// endpoints, never policy-selected members
		default:
			if node.Namespace == "" {
				continue
			}
			membership.namespaceOf[node.ID] = node.Namespace
			membership.memberCount[node.Namespace]++
		}
	}
	return membership
}

// uniformRuleKey fingerprints everything about a rule EXCEPT which workload it
// applies to. Rules sharing a key state one fact about different workloads of
// the same namespace — the fan-out a cluster-wide policy (Calico all(), a k8s
// blanket rule) produces. Empty key = the rule has no workload endpoint in a
// known namespace, so it is never a collapse candidate.
func uniformRuleKey(rule models.Rule, membership nsMembership) (key, namespace, workloadID, side string) {
	// Engine-asserted scope only. A selector that merely happens to cover every
	// pod present today is not a namespace-wide claim, and collapsing it would
	// make the arrow shape flip the moment someone adds an unlabeled pod.
	if !rule.NamespaceWide {
		return "", "", "", ""
	}
	switch {
	case membership.namespaceOf[rule.SrcID] != "":
		namespace, workloadID, side = membership.namespaceOf[rule.SrcID], rule.SrcID, sideSrc
		key = rule.DstID
	case membership.namespaceOf[rule.DstID] != "":
		namespace, workloadID, side = membership.namespaceOf[rule.DstID], rule.DstID, sideDst
		key = rule.SrcID
	default:
		return "", "", "", ""
	}
	// peer|side|ns|direction|action|coverage|policy identity|ports|l7|selectors
	key = fmt.Sprintf("%s|%s|%s|%s|%d|%s|%s/%s/%s|%v|%t|%t|%v|%v|%v",
		key, side, namespace, rule.Direction, rule.Action, rule.Coverage,
		rule.Contributor.Source, rule.Contributor.Namespace, rule.Contributor.Name,
		rule.Ports, rule.AllPorts, rule.AllL7, rule.L7Match, rule.SrcSelector, rule.DstSelector)
	return key, namespace, workloadID, side
}

// collapseUniformRules folds a whole namespace worth of identical per-workload
// rules into one namespace-level rule. A cluster-wide policy otherwise emits one
// arrow per pod to the same peer — thousands of arrows carrying a single fact,
// which buries every other engine's edges and costs layout time.
//
// Collapse only when EVERY workload in the namespace resolved this exact rule:
// first-match precedence lets a narrower policy override some pods, and in that
// case the per-workload arrows differ and are worth drawing. Per-workload truth
// stays on the node's Coverage/status badges either way.
// Returns the rewritten rule slice plus, per collapsed arrow (edgeGroupKey), how
// many per-workload rules it stands in for.
func collapseUniformRules(rules []models.Rule, nodes []models.WorkloadNode) ([]models.Rule, map[string]int) {
	membership := buildNsMembership(nodes)

	keys := make([]string, len(rules))
	namespaces := make([]string, len(rules))
	sides := make([]string, len(rules))
	coveredWorkloads := map[string]map[string]bool{}
	for index, rule := range rules {
		key, namespace, workloadID, side := uniformRuleKey(rule, membership)
		if key == "" {
			continue
		}
		keys[index], namespaces[index], sides[index] = key, namespace, side
		if coveredWorkloads[key] == nil {
			coveredWorkloads[key] = map[string]bool{}
		}
		coveredWorkloads[key][workloadID] = true
	}

	emitted := map[string]bool{}
	aggregatedFrom := map[string]int{}
	out := make([]models.Rule, 0, len(rules))
	for index, rule := range rules {
		key := keys[index]
		covered := len(coveredWorkloads[key])
		namespace := namespaces[index]
		if key == "" || covered < minCollapseMembers ||
			covered != membership.memberCount[namespace] || membership.nsNodeID[namespace] == "" {
			out = append(out, rule)
			continue
		}
		if emitted[key] {
			continue // one namespace-level rule per uniform group
		}
		emitted[key] = true
		if sides[index] == sideSrc {
			rule.SrcID = membership.nsNodeID[namespace]
		} else {
			rule.DstID = membership.nsNodeID[namespace]
		}
		aggregatedFrom[edgeGroupKey(rule)] = covered
		out = append(out, rule)
	}
	return out, aggregatedFrom
}

// edgeGroupKey — identity of the arrow a rule folds into. policySource is part of
// it so identically-named policies from different engines (k8s "allow-ingress" +
// istio "allow-ingress") stay separate arrows instead of merging.
func edgeGroupKey(rule models.Rule) string {
	return fmt.Sprintf("%s|%s|%s|%s|%s|%s|%d",
		rule.SrcID, rule.DstID, rule.Direction,
		rule.Contributor.Name, rule.Contributor.Namespace, rule.Contributor.Source, rule.Action)
}

// foldTuplesToEdges collapses pod-level Rules into PolicyEdges.
// Used to render arrow in graphs, to eliminate duplicates ports or l7 rules
func RenderEdges(rules []models.Rule, nodes []models.WorkloadNode) []PolicyEdge {
	rules, aggregatedFrom := collapseUniformRules(rules, nodes)

	nsNodeIDs := map[string]bool{}
	for _, node := range nodes {
		if node.Type == models.NodeTypeNamespace {
			nsNodeIDs[node.ID] = true
		}
	}

	grouped := map[string]*PolicyEdge{}

	for _, rule := range rules {
		key := edgeGroupKey(rule)
		edge, exists := grouped[key]
		if !exists {
			// An aggregated arrow ends on a namespace node but states a per-workload
			// fact, so it keeps workload level — the ns-edge toggle hides genuine
			// ns-scoped policy, not this.
			level := edgeLevelFor(rule.SrcID, rule.DstID, nsNodeIDs)
			if aggregatedFrom[key] > 0 {
				level = models.EdgeLevelWorkload
			}
			edge = &PolicyEdge{
				Source:         rule.SrcID,
				Target:         rule.DstID,
				Direction:      rule.Direction,
				PolicyName:     rule.Contributor.Name,
				Namespace:      rule.Contributor.Namespace,
				Level:          level,
				PolicySource:   rule.Contributor.Source,
				Action:         rule.Action,
				Coverage:       rule.Coverage,
				AggregatedFrom: aggregatedFrom[key],
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
