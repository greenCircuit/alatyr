package graph

import (
	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func buildEdges(nodesNsIndex map[string]map[string][]*WorkloadNode, policies []networkingv1.NetworkPolicy) []PolicyEdge {
	var allEdges []PolicyEdge
	for _, policy := range policies {
		srcNodes := getSourceNodes(policy, nodesNsIndex)             // all nodes that this policies are applied to
		egressTargets := getTargetEgressNodes(policy, nodesNsIndex)  // all nodes that these target nodes can ingress
		ingressTargets := getTargetIngressNodes(policy, nodesNsIndex) // all nodes that target nodes do ingress

		for _, src := range srcNodes {
			for _, e := range egressTargets {
				e.Source = src.ID
				allEdges = append(allEdges, e)
			}
			for _, e := range ingressTargets {
				e.Source = e.Target
				e.Target = src.ID
				allEdges = append(allEdges, e)
			}
		}
	}
	return allEdges
}

func getTargetEgressNodes(policy networkingv1.NetworkPolicy, nodes map[string]map[string][]*WorkloadNode) []PolicyEdge {
	var matches []PolicyEdge
	for _, rule := range policy.Spec.Egress {
		ports := convertPorts(rule.Ports)
		for _, peer := range rule.To {
			matches = append(matches, generateEdges(policy.Name, policy.Namespace, DirectionEgress, peer, ports, nodes)...)
		}
	}
	return matches
}

func getTargetIngressNodes(policy networkingv1.NetworkPolicy, nodes map[string]map[string][]*WorkloadNode) []PolicyEdge {
	var matches []PolicyEdge
	for _, rule := range policy.Spec.Ingress {
		ports := convertPorts(rule.Ports)
		for _, peer := range rule.From {
			matches = append(matches, generateEdges(policy.Name, policy.Namespace, DirectionIngress, peer, ports, nodes)...)
		}
	}
	return matches
}

// convertPorts translates rule-level NetworkPolicyPort entries into graph.Port.
// Empty input returns nil → UI renders the edge as "all ports".
func convertPorts(rulePorts []networkingv1.NetworkPolicyPort) []Port {
	if len(rulePorts) == 0 {
		return nil
	}
	out := make([]Port, 0, len(rulePorts))
	for _, p := range rulePorts {
		port := Port{Protocol: "TCP"}
		if p.Protocol != nil {
			port.Protocol = string(*p.Protocol)
		}
		if p.Port != nil {
			if p.Port.Type == intstr.String {
				port.Name = p.Port.StrVal
			} else {
				port.Port = int(p.Port.IntVal)
			}
		}
		if p.EndPort != nil {
			port.EndPort = int(*p.EndPort)
		}
		out = append(out, port)
	}
	return out
}

// main function that generates edges
func generateEdges(policyName, srcNs string, direction Direction, peer networkingv1.NetworkPolicyPeer, ports []Port, nodesIndex map[string]map[string][]*WorkloadNode) []PolicyEdge {
	var matches []PolicyEdge

	// namespace only
	if peer.PodSelector == nil && peer.NamespaceSelector != nil {
		// empty namespaceSelector = any namespace — too broad to draw as arrows, skip
		if isCatchAll(peer.NamespaceSelector.MatchLabels, len(peer.NamespaceSelector.MatchExpressions)) {
			return nil
		}
		for _, nsNodes := range nodesIndex {
			nsNode := findNSNode(nsNodes)
			if nsNode == nil || !labelsMatch(peer.NamespaceSelector.MatchLabels, nsNode.Labels) {
				continue
			}
			matches = append(matches, PolicyEdge{
				PolicyName: policyName,
				Direction:  direction,
				Namespace:  srcNs,
				Target:     nsNode.ID,
				Level:      EdgeLevelNamespace,
				Ports:      ports,
			})
		}
	}

	// pod selector only — same namespace
	if peer.PodSelector != nil && peer.NamespaceSelector == nil {
		// empty podSelector = all pods in same namespace — collapse to namespace node
		if isCatchAll(peer.PodSelector.MatchLabels, len(peer.PodSelector.MatchExpressions)) {
			if nsNode := findNSNode(nodesIndex[srcNs]); nsNode != nil {
				matches = append(matches, PolicyEdge{
					PolicyName: policyName,
					Direction:  direction,
					Namespace:  srcNs,
					Target:     nsNode.ID,
					Level:      EdgeLevelNamespace,
					Ports:      ports,
				})
			}
			return matches
		}
		for _, target := range indexLabelMatch(peer.PodSelector.MatchLabels, nodesIndex[srcNs]) {
			matches = append(matches, PolicyEdge{
				PolicyName: policyName,
				Direction:  direction,
				Namespace:  srcNs,
				Target:     target.ID,
				Level:      EdgeLevelWorkload,
				Ports:      ports,
			})
		}
	}

	// both — pods in namespaces matching namespaceSelector
	if peer.PodSelector != nil && peer.NamespaceSelector != nil {
		catchAllPod := isCatchAll(peer.PodSelector.MatchLabels, len(peer.PodSelector.MatchExpressions))
		catchAllNS := isCatchAll(peer.NamespaceSelector.MatchLabels, len(peer.NamespaceSelector.MatchExpressions))
		for ns, nsNodes := range nodesIndex {
			nsNode := findNSNode(nsNodes)
			if !catchAllNS {
				if nsNode == nil || !labelsMatch(peer.NamespaceSelector.MatchLabels, nsNode.Labels) {
					continue
				}
			}
			// empty podSelector within matched namespace → use namespace node
			if catchAllPod {
				if nsNode != nil {
					matches = append(matches, PolicyEdge{
						PolicyName: policyName,
						Direction:  direction,
						Namespace:  ns,
						Target:     nsNode.ID,
						Level:      EdgeLevelNamespace,
						Ports:      ports,
					})
				}
				continue
			}
			for _, target := range indexLabelMatch(peer.PodSelector.MatchLabels, nsNodes) {
				matches = append(matches, PolicyEdge{
					PolicyName: policyName,
					Direction:  direction,
					Namespace:  ns,
					Target:     target.ID,
					Level:      EdgeLevelWorkload,
					Ports:      ports,
				})
			}
		}
	}

	// IP block — external traffic
	if peer.IPBlock != nil {
		matches = append(matches, PolicyEdge{
			PolicyName: policyName,
			Direction:  direction,
			Namespace:  srcNs,
			Target:     peer.IPBlock.CIDR,
			Level:      EdgeLevelWorkload,
			Ports:      ports,
		})
	}

	return matches
}

func isCatchAll(matchLabels map[string]string, nExpressions int) bool {
	return len(matchLabels) == 0 && nExpressions == 0
}

func findNSNode(labelIndex map[string][]*WorkloadNode) *WorkloadNode {
	for _, nodes := range labelIndex {
		for _, n := range nodes {
			if n.Type == NodeTypeNamespace {
				return n
			}
		}
	}
	return nil
}

func labelsMatch(selector map[string]string, labels map[string]string) bool {
	for k, v := range selector {
		if labels[k] != v {
			return false
		}
	}
	return true
}

// use indexing of each label per ns to find out what nodes would match selector labels
// at this point it is just single ns index, upsteam funtions already did ns logic
func indexLabelMatch(selectors map[string]string, labelIndex map[string][]*WorkloadNode) []*WorkloadNode {
	if len(selectors) == 0 {
		return nil
	}
	matches := map[*WorkloadNode]bool{}
	first := true

	for key, value := range selectors {
		mapKey := makeLabelIndexKey(key, value)
		// labelSelector has to exist inside provided map
		matchingNodes, exists := labelIndex[mapKey]
		if !exists {
			return nil
		}

		// will seed initial data of first loop
		if first {
			for _, node := range matchingNodes {
				matches[node] = true
			}
			first = false
			continue
		}

		// on consecutive loops, will check what been recorded
		// and matcheces will be itersection what matches and current itteratio
		// finding all nodes that are availabe in both maps
		localMatch := map[*WorkloadNode]bool{}
		for _, node := range matchingNodes {
			if matches[node] {
				localMatch[node] = true
			}
		}

		// now localMatch will have intersection, so assignt it so other loops can compare
		matches = localMatch

		// don't have any intersection between label selectoprs
		if len(matches) == 0 {
			return nil
		}
	}
	out := make([]*WorkloadNode, 0, len(matches))
	for node := range matches {
		out = append(out, node)
	}
	return out
}
