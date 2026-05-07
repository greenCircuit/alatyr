package graph

import networkingv1 "k8s.io/api/networking/v1"

func buildEdges(nodesByNS map[string][]WorkloadNode, policies []networkingv1.NetworkPolicy) []PolicyEdge {
	var allEdges []PolicyEdge
	for _, policy := range policies {
		srcNodes := getSourceNodes(policy, nodesByNS)
		egressTargets := getTargetEgressNodes(policy, nodesByNS)
		ingressTargets := getTargetIngressNodes(policy, nodesByNS)

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

func getTargetEgressNodes(policy networkingv1.NetworkPolicy, nodes map[string][]WorkloadNode) []PolicyEdge {
	var matches []PolicyEdge
	for _, rule := range policy.Spec.Egress {
		for _, peer := range rule.To {
			matches = append(matches, generateEdges(policy.Namespace, DirectionEgress, peer, nodes)...)
		}
	}
	return matches
}

func getTargetIngressNodes(policy networkingv1.NetworkPolicy, nodes map[string][]WorkloadNode) []PolicyEdge {
	var matches []PolicyEdge
	for _, rule := range policy.Spec.Ingress {
		for _, peer := range rule.From {
			matches = append(matches, generateEdges(policy.Namespace, DirectionIngress, peer, nodes)...)
		}
	}
	return matches
}

func generateEdges(srcNs string, direction Direction, peer networkingv1.NetworkPolicyPeer, nodes map[string][]WorkloadNode) []PolicyEdge {
	var matches []PolicyEdge

	// namespace only
	if peer.PodSelector == nil && peer.NamespaceSelector != nil {
		// empty namespaceSelector = any namespace — too broad to draw as arrows, skip
		if isCatchAll(peer.NamespaceSelector.MatchLabels, len(peer.NamespaceSelector.MatchExpressions)) {
			return nil
		}
		for _, nsNodes := range nodes {
			nsNode := findNSNode(nsNodes)
			if nsNode == nil || !labelsMatch(peer.NamespaceSelector.MatchLabels, nsNode.Labels) {
				continue
			}
			matches = append(matches, PolicyEdge{
				Direction: direction,
				Namespace: srcNs,
				Target:    nsNode.ID,
				Level:     EdgeLevelNamespace,
			})
		}
	}

	// pod selector only — same namespace
	if peer.PodSelector != nil && peer.NamespaceSelector == nil {
		// empty podSelector = all pods in same namespace — collapse to namespace node
		if isCatchAll(peer.PodSelector.MatchLabels, len(peer.PodSelector.MatchExpressions)) {
			if nsNode := findNSNode(nodes[srcNs]); nsNode != nil {
				matches = append(matches, PolicyEdge{
					Direction: direction,
					Namespace: srcNs,
					Target:    nsNode.ID,
					Level:     EdgeLevelNamespace,
				})
			}
			return matches
		}
		for _, target := range findNodeByLabel(peer.PodSelector.MatchLabels, nodes[srcNs]) {
			matches = append(matches, PolicyEdge{
				Direction: direction,
				Namespace: srcNs,
				Target:    target.ID,
				Level:     EdgeLevelWorkload,
			})
		}
	}

	// both — pods in namespaces matching namespaceSelector
	if peer.PodSelector != nil && peer.NamespaceSelector != nil {
		catchAllPod := isCatchAll(peer.PodSelector.MatchLabels, len(peer.PodSelector.MatchExpressions))
		catchAllNS := isCatchAll(peer.NamespaceSelector.MatchLabels, len(peer.NamespaceSelector.MatchExpressions))
		for ns, nsNodes := range nodes {
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
						Direction: direction,
						Namespace: ns,
						Target:    nsNode.ID,
						Level:     EdgeLevelNamespace,
					})
				}
				continue
			}
			for _, target := range findNodeByLabel(peer.PodSelector.MatchLabels, nsNodes) {
				matches = append(matches, PolicyEdge{
					Direction: direction,
					Namespace: ns,
					Target:    target.ID,
					Level:     EdgeLevelWorkload,
				})
			}
		}
	}

	// IP block — external traffic
	if peer.IPBlock != nil {
		matches = append(matches, PolicyEdge{
			Direction: direction,
			Namespace: srcNs,
			Target:    peer.IPBlock.CIDR,
			Level:     EdgeLevelWorkload,
		})
	}

	return matches
}

func isCatchAll(matchLabels map[string]string, nExpressions int) bool {
	return len(matchLabels) == 0 && nExpressions == 0
}

func findNSNode(nodes []WorkloadNode) *WorkloadNode {
	for i := range nodes {
		if nodes[i].Type == NodeTypeNamespace {
			return &nodes[i]
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
