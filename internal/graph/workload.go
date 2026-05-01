package graph

import (
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	"main/internal/k8s"
)

// BuildGraph fetches resources per namespace and correlates them into a Graph.
func BuildGraph(namespaces []string) Graph {
	nodesByNS := map[string][]WorkloadNode{}
	for _, ns := range namespaces {
		nodesByNS[ns] = buildWorkloadNodesForNS(ns)
	}

	policies := fetchPolicies(namespaces)
	edges := buildEdges(nodesByNS, policies)

	var allNodes []WorkloadNode
	for _, nsNodes := range nodesByNS {
		allNodes = append(allNodes, nsNodes...)
	}

	return Graph{Nodes: allNodes, Edges: edges}
}

// buildWorkloadNodesForNS fetches pods for a namespace and returns deduplicated WorkloadNodes.
func buildWorkloadNodesForNS(ns string) []WorkloadNode {
	pods := k8s.GetPods(ns)
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

	return nodes
}

// fetchPolicies fetches NetworkPolicies for all requested namespaces.
func fetchPolicies(namespaces []string) []networkingv1.NetworkPolicy {
	var policies []networkingv1.NetworkPolicy
	for _, ns := range namespaces {
		policies = append(policies, k8s.GetPolicies(ns)...)
	}
	return policies
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
func buildEdges(nodesByNS map[string][]WorkloadNode, policies []networkingv1.NetworkPolicy) {
	for _, policy := range policies {
		srcNodes := getSourceNodes(policy, nodesByNS)
		destNodesEgress := getSourceNodes(policy, nodesByNS) // get all nodes the policy allows egress
		statusKeys := getStatusKeys(policy)
		

	}
	return nil
}

// find all workloads that network policies will be applied to
func getSourceNodes(policy networkingv1.NetworkPolicy, nodes map[string][]WorkloadNode) []WorkloadNode {
	ns := policy.Namespace
	nsNodes := nodes[ns]
	// policy matches all pods in ns
	if len(policy.Spec.PodSelector.MatchLabels) == 0 {
		return nsNodes
	}

	return findNodeByLabel(policy.Labels, nsNodes)
}

func getTargetEgressNodes(policy networkingv1.NetworkPolicy, nodes map[string][]WorkloadNode) []PolicyEdge {
	var matches []PolicyEdge
	if len(policy.Spec.Egress) != 0 {
		for _, rule := range policy.Spec.Egress {                                                         
			for _, peer := range rule.To {                                                                
				if peer.PodSelector != nil {                                                              
					ns := policy.Namespace
					matches := findNodeByLabel(peer.PodSelector.MatchLabels, nodes[ns])
					for _, match := range matches {

					}
				}                                                                                         
			}
		}         
	}
	return matches
}


// generic function to find to what workload policy is applied or that is the target workloads
// find what nodes in list have specific label that came from network policy
func findNodeByLabel(labels map[string]string, nodes []WorkloadNode) []WorkloadNode {
	var matches []WorkloadNode
	for _, node := range nodes {
		matched := true
		for labelKey, labelValue := range labels {
			if node.Labels[labelKey] != labelValue {  // checks if label maps has the same keys values inside a node
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

// determineStatusKeys computes status badge keys for a workload node given the full policy set.
func getStatusKeys(node WorkloadNode, policies []networkingv1.NetworkPolicy) []StatusKey {
	// TODO
	return nil
}
