package graph

import (
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
)

func (b *Builder) buildWorkloadNodesForNS(ns string) ([]WorkloadNode, error) {
	pods, err := b.client.GetPods(ns)
	if err != nil {
		return nil, err
	}

	svc, err := b.client.GetSvc(ns)
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

	// add services that have no matching pod workload node
	var svcNodes []WorkloadNode
	for _, s := range svc {
		res := findNodeByLabel(s.Spec.Selector, nodes)
		if len(res) == 0 {
			svcNodes = append(svcNodes, WorkloadNode{
				ID:        string(s.UID),
				Label:     s.Name,
				Namespace: s.Namespace,
				Type:      NodeTypeService,
				Labels:    s.Labels,
			})
		}
	}
	nodes = append(nodes, svcNodes...)

	// add namespace node
	nsObj, err := b.client.GetNs(ns)
	if err != nil {
		return nil, err
	}

	nsNode := WorkloadNode{
		ID:        "ns-" + nsObj.Name,
		Label:     nsObj.Name,
		Namespace: nsObj.Namespace,
		Type:      NodeTypeNamespace,
		Labels:    nsObj.Labels,
	}

	nodes = append(nodes, nsNode)

	return nodes, nil
}

// generate all nodes for ns
func getNsNodes(namespaces []corev1.Namespace) []WorkloadNode {
	var nsNodes []WorkloadNode
	for _, ns := range namespaces {
		nsNodes = append(nsNodes, WorkloadNode{
			ID:        string(ns.UID),
			Label:     ns.Name,
			Namespace: ns.Namespace,
			Type:      NodeTypeNamespace,
			Labels:    ns.Labels,
		})
	}
	return nsNodes
}

// getSourceNodes finds all workload nodes the policy applies to via podSelector.
// An empty podSelector (catch-all) is collapsed to the namespace node to avoid N² edges.
func getSourceNodes(policy networkingv1.NetworkPolicy, nodes map[string][]WorkloadNode) []WorkloadNode {
	nsNodes := nodes[policy.Namespace]
	if isCatchAll(policy.Spec.PodSelector.MatchLabels, len(policy.Spec.PodSelector.MatchExpressions)) {
		if nsNode := findNSNode(nsNodes); nsNode != nil {
			return []WorkloadNode{*nsNode}
		}
	}
	return findNodeByLabel(policy.Spec.PodSelector.MatchLabels, nsNodes)
}

func ownerUID(pod corev1.Pod) string {
	if len(pod.OwnerReferences) > 0 {
		return string(pod.OwnerReferences[0].UID)
	}
	return string(pod.UID)
}

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
