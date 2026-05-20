package k8spolicy

import (
	"graph/internal/models"
	"graph/internal/policy"

	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

const sourceName = "k8s"

// BuildAllowTuples expands every NetworkPolicy into pod-level AllowTuples.
// One tuple per (src, dst, port, direction, policy-rule).
func BuildAllowTuples(nodesNsIndex map[string]map[string][]*models.WorkloadNode, policies []networkingv1.NetworkPolicy) []policy.AllowTuple {
	var allTuples []policy.AllowTuple
	for _, networkPolicy := range policies {
		srcNodes := getSourceNodes(networkPolicy, nodesNsIndex)
		egressPartials := getTargetEgressTuples(networkPolicy, nodesNsIndex)
		ingressPartials := getTargetIngressTuples(networkPolicy, nodesNsIndex)

		for _, srcNode := range srcNodes {
			for _, tuple := range egressPartials {
				tuple.SrcID = srcNode.ID
				allTuples = append(allTuples, tuple)
			}
			for _, tuple := range ingressPartials {
				// ingress: the applied-to workload is the destination, peer is the source
				tuple.SrcID = tuple.DstID
				tuple.DstID = srcNode.ID
				allTuples = append(allTuples, tuple)
			}
		}
	}
	return allTuples
}

func getTargetEgressTuples(networkPolicy networkingv1.NetworkPolicy, nodes map[string]map[string][]*models.WorkloadNode) []policy.AllowTuple {
	var out []policy.AllowTuple
	for ruleIndex, rule := range networkPolicy.Spec.Egress {
		ports := convertPorts(rule.Ports)
		for _, peer := range rule.To {
			out = append(out, generateTuples(networkPolicy.Name, networkPolicy.Namespace, ruleIndex, models.DirectionEgress, peer, ports, nodes)...)
		}
	}
	return out
}

func getTargetIngressTuples(networkPolicy networkingv1.NetworkPolicy, nodes map[string]map[string][]*models.WorkloadNode) []policy.AllowTuple {
	var out []policy.AllowTuple
	for ruleIndex, rule := range networkPolicy.Spec.Ingress {
		ports := convertPorts(rule.Ports)
		for _, peer := range rule.From {
			out = append(out, generateTuples(networkPolicy.Name, networkPolicy.Namespace, ruleIndex, models.DirectionIngress, peer, ports, nodes)...)
		}
	}
	return out
}

// convertPorts translates rule-level NetworkPolicyPort entries into models.Port.
// Empty input returns nil — generateTuples expands nil into a single all-ports tuple.
func convertPorts(rulePorts []networkingv1.NetworkPolicyPort) []models.Port {
	if len(rulePorts) == 0 {
		return nil
	}
	out := make([]models.Port, 0, len(rulePorts))
	for _, rulePort := range rulePorts {
		port := models.Port{Protocol: "TCP"}
		if rulePort.Protocol != nil {
			port.Protocol = string(*rulePort.Protocol)
		}
		if rulePort.Port != nil {
			if rulePort.Port.Type == intstr.String {
				port.Name = rulePort.Port.StrVal
			} else {
				port.Port = int(rulePort.Port.IntVal)
			}
		}
		if rulePort.EndPort != nil {
			port.EndPort = int(*rulePort.EndPort)
		}
		out = append(out, port)
	}
	return out
}

// generateTuples produces one tuple per (peer-match × port).
// Returned tuples have SrcID empty — BuildAllowTuples fills it from the policy's selected workloads.
func generateTuples(policyName, policyNamespace string, ruleIndex int, direction models.Direction, peer networkingv1.NetworkPolicyPeer, ports []models.Port, nodesIndex map[string]map[string][]*models.WorkloadNode) []policy.AllowTuple {
	var dstIDs []string

	// namespace only
	if peer.PodSelector == nil && peer.NamespaceSelector != nil {
		if isCatchAll(peer.NamespaceSelector.MatchLabels, len(peer.NamespaceSelector.MatchExpressions)) {
			return nil
		}
		for _, nsNodes := range nodesIndex {
			nsNode := findNSNode(nsNodes)
			if nsNode == nil || !labelsMatch(peer.NamespaceSelector.MatchLabels, nsNode.Labels) {
				continue
			}
			dstIDs = append(dstIDs, nsNode.ID)
		}
	}

	// pod selector only — same namespace
	if peer.PodSelector != nil && peer.NamespaceSelector == nil {
		if isCatchAll(peer.PodSelector.MatchLabels, len(peer.PodSelector.MatchExpressions)) {
			if nsNode := findNSNode(nodesIndex[policyNamespace]); nsNode != nil {
				dstIDs = append(dstIDs, nsNode.ID)
			}
		} else {
			for _, target := range indexLabelMatch(peer.PodSelector.MatchLabels, nodesIndex[policyNamespace]) {
				dstIDs = append(dstIDs, target.ID)
			}
		}
	}

	// both — pods in namespaces matching namespaceSelector
	if peer.PodSelector != nil && peer.NamespaceSelector != nil {
		catchAllPod := isCatchAll(peer.PodSelector.MatchLabels, len(peer.PodSelector.MatchExpressions))
		catchAllNS := isCatchAll(peer.NamespaceSelector.MatchLabels, len(peer.NamespaceSelector.MatchExpressions))
		for _, nsNodes := range nodesIndex {
			nsNode := findNSNode(nsNodes)
			if !catchAllNS {
				if nsNode == nil || !labelsMatch(peer.NamespaceSelector.MatchLabels, nsNode.Labels) {
					continue
				}
			}
			if catchAllPod {
				if nsNode != nil {
					dstIDs = append(dstIDs, nsNode.ID)
				}
				continue
			}
			for _, target := range indexLabelMatch(peer.PodSelector.MatchLabels, nsNodes) {
				dstIDs = append(dstIDs, target.ID)
			}
		}
	}

	// IP block — external traffic
	if peer.IPBlock != nil {
		dstIDs = append(dstIDs, peer.IPBlock.CIDR)
	}

	contributors := []policy.PolicyRef{{
		Source:    sourceName,
		Name:      policyName,
		Namespace: policyNamespace,
		RuleIndex: ruleIndex,
	}}

	effectivePorts := ports
	if len(effectivePorts) == 0 {
		effectivePorts = []models.Port{{Protocol: "TCP"}}
	}

	out := make([]policy.AllowTuple, 0, len(dstIDs)*len(effectivePorts))
	for _, dstID := range dstIDs {
		for _, port := range effectivePorts {
			out = append(out, policy.AllowTuple{
				DstID:        dstID,
				Port:         port,
				Direction:    direction,
				Contributors: contributors,
			})
		}
	}
	return out
}

// GetNodePolicies returns all NetworkPolicies that select the given workload node
// via PodSelector. Namespace nodes only match catch-all selectors.
func GetNodePolicies(node models.WorkloadNode, policies []networkingv1.NetworkPolicy) []networkingv1.NetworkPolicy {
	var matches []networkingv1.NetworkPolicy
	for _, networkPolicy := range policies {
		podSelector := networkPolicy.Spec.PodSelector
		catchAll := isCatchAll(podSelector.MatchLabels, len(podSelector.MatchExpressions))
		if node.Type == models.NodeTypeNamespace {
			if catchAll {
				matches = append(matches, networkPolicy)
			}
			continue
		}
		if labelsMatch(podSelector.MatchLabels, node.Labels) {
			matches = append(matches, networkPolicy)
		}
	}
	return matches
}

func getSourceNodes(networkPolicy networkingv1.NetworkPolicy, nodes map[string]map[string][]*models.WorkloadNode) []*models.WorkloadNode {
	nsNodes := nodes[networkPolicy.Namespace]
	if isCatchAll(networkPolicy.Spec.PodSelector.MatchLabels, len(networkPolicy.Spec.PodSelector.MatchExpressions)) {
		if nsNode := findNSNode(nsNodes); nsNode != nil {
			return []*models.WorkloadNode{nsNode}
		}
	}
	return indexLabelMatch(networkPolicy.Spec.PodSelector.MatchLabels, nsNodes)
}

func isCatchAll(matchLabels map[string]string, nExpressions int) bool {
	return len(matchLabels) == 0 && nExpressions == 0
}

func findNSNode(labelIndex map[string][]*models.WorkloadNode) *models.WorkloadNode {
	for _, nodes := range labelIndex {
		for _, node := range nodes {
			if node.Type == models.NodeTypeNamespace {
				return node
			}
		}
	}
	return nil
}

func labelsMatch(selector map[string]string, labels map[string]string) bool {
	for key, value := range selector {
		if labels[key] != value {
			return false
		}
	}
	return true
}

func indexLabelMatch(selectors map[string]string, labelIndex map[string][]*models.WorkloadNode) []*models.WorkloadNode {
	if len(selectors) == 0 {
		return nil
	}
	matches := map[*models.WorkloadNode]bool{}
	first := true

	for key, value := range selectors {
		mapKey := makeLabelIndexKey(key, value)
		matchingNodes, exists := labelIndex[mapKey]
		if !exists {
			return nil
		}

		if first {
			for _, node := range matchingNodes {
				matches[node] = true
			}
			first = false
			continue
		}

		localMatch := map[*models.WorkloadNode]bool{}
		for _, node := range matchingNodes {
			if matches[node] {
				localMatch[node] = true
			}
		}
		matches = localMatch
		if len(matches) == 0 {
			return nil
		}
	}
	out := make([]*models.WorkloadNode, 0, len(matches))
	for node := range matches {
		out = append(out, node)
	}
	return out
}

func makeLabelIndexKey(key, value string) string {
	return key + "=" + value
}

func buildWorkloadIndex(nodes []models.WorkloadNode) map[string][]*models.WorkloadNode {
	index := make(map[string][]*models.WorkloadNode, len(nodes))
	for position := range nodes {
		node := &nodes[position]
		for key, value := range node.Labels {
			entry := makeLabelIndexKey(key, value)
			index[entry] = append(index[entry], node)
		}
	}
	return index
}
