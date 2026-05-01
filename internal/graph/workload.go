package graph

import (
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	"main/internal/k8s"
)

// BuildGraph fetches resources per namespace and correlates them into a Graph.
func BuildGraph(namespaces []string, client *k8s.Client) (Graph, error) {
	nodesByNS := map[string][]WorkloadNode{}
	for _, ns := range namespaces {
		nodes, err := buildWorkloadNodesForNS(ns, client)
		if err != nil {
			return Graph{}, err
		}
		nodesByNS[ns] = nodes
	}

	policies, err := fetchPolicies(namespaces, client)
	if err != nil {
		return Graph{}, err
	}

	edges := buildEdges(nodesByNS, policies)

	var allNodes []WorkloadNode
	for _, nsNodes := range nodesByNS {
		allNodes = append(allNodes, nsNodes...)
	}

	return Graph{Nodes: allNodes, Edges: edges}, nil
}

// buildWorkloadNodesForNS fetches pods for a namespace and returns deduplicated WorkloadNodes.
func buildWorkloadNodesForNS(ns string, client *k8s.Client) ([]WorkloadNode, error) {
	pods, err := client.GetPods(ns)
	if err != nil {
		return nil, err
	}

	seen := map[string]bool{}
	var nodes []WorkloadNode

	for _, pod := range pods {
		uid := ownerUID(pod)
		if seen[uid] {
			continue
		}
		seen[uid] = true

		nodes = append(nodes, WorkloadNode{
			ID:        uid,
			Label:     workloadLabel(pod),
			Namespace: pod.Namespace,
			Type:      NodeTypeDeployment,
			Labels:    pod.Labels,
		})
	}

	return nodes, nil
}

// fetchPolicies fetches NetworkPolicies for all requested namespaces.
func fetchPolicies(namespaces []string, client *k8s.Client) ([]networkingv1.NetworkPolicy, error) {
	var policies []networkingv1.NetworkPolicy
	for _, ns := range namespaces {
		nsPolicies, err := client.GetPolicies(ns)
		if err != nil {
			return nil, err
		}
		policies = append(policies, nsPolicies...)
	}
	return policies, nil
}

// ownerUID returns the UID of the pod's first owner, falling back to the pod's own UID.
func ownerUID(pod corev1.Pod) string {
	if len(pod.OwnerReferences) > 0 {
		return string(pod.OwnerReferences[0].UID)
	}
	return string(pod.UID)
}

// workloadLabel returns a human-readable name for the workload node.
func workloadLabel(pod corev1.Pod) string {
	if name, ok := pod.Labels["app.kubernetes.io/name"]; ok {
		return name
	}
	if name, ok := pod.Labels["app"]; ok {
		return name
	}
	if len(pod.OwnerReferences) > 0 {
		return pod.OwnerReferences[0].Name
	}
	return pod.Name
}

// buildEdges correlates NetworkPolicies against workload nodes and returns policy edges.
// TODO add bidiretionllity
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
				e.Source = src.ID
				allEdges = append(allEdges, e)
			}
		}
	}
	return allEdges
}

// getSourceNodes finds all workload nodes the policy applies to via podSelector.
func getSourceNodes(policy networkingv1.NetworkPolicy, nodes map[string][]WorkloadNode) []WorkloadNode {
	nsNodes := nodes[policy.Namespace]
	if len(policy.Spec.PodSelector.MatchLabels) == 0 {
		return nsNodes
	}
	return findNodeByLabel(policy.Spec.PodSelector.MatchLabels, nsNodes)
}

func getTargetEgressNodes(policy networkingv1.NetworkPolicy, nodes map[string][]WorkloadNode) []PolicyEdge {
	var matches []PolicyEdge
	for _, rule := range policy.Spec.Egress {
		for _, peer := range rule.To {
			if peer.PodSelector != nil {
				targets := findNodeByLabel(peer.PodSelector.MatchLabels, nodes[policy.Namespace])
				for _, target := range targets {
					edge := PolicyEdge{
						Direction: DirectionEgress,
						Namespace: policy.Namespace,
						Target:    target.ID,
						Level:     EdgeLevelWorkload,
					}
					matches = append(matches, edge)
				}
			}
		}
	}
	return matches
}

func getTargetIngressNodes(policy networkingv1.NetworkPolicy, nodes map[string][]WorkloadNode) []PolicyEdge {
	var matches []PolicyEdge
	for _, rule := range policy.Spec.Ingress {
		for _, peer := range rule.From {
			if peer.PodSelector != nil {
				targets := findNodeByLabel(peer.PodSelector.MatchLabels, nodes[policy.Namespace])
				for _, target := range targets {
					edge := PolicyEdge{
						Direction: DirectionIngress,
						Namespace: policy.Namespace,
						Target:    target.ID,
						Level:     EdgeLevelWorkload,
					}
					matches = append(matches, edge)
				}
			}
		}
	}
	return matches
}

// findNodeByLabel returns nodes whose labels contain all key-value pairs in the given label map.
func findNodeByLabel(labelMap map[string]string, nodes []WorkloadNode) []WorkloadNode {
	var matches []WorkloadNode
	for _, node := range nodes {
		matched := true
		for k, v := range labelMap {
			if node.Labels[k] != v {
				matched = false
				break
			}
		}
		if matched {
			matches = append(matches, node)
		}
	}
	return matches
}

// getStatusKeys computes status badge keys for a workload node given the full policy set.
func getStatusKeys(node WorkloadNode, policies []networkingv1.NetworkPolicy) []StatusKey {
	// TODO
	return nil
}
