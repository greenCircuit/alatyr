package graph


import (
	"graph/internal/utils"

	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
)

func (b *Builder) buildWorkloadNodesForNS(ns string) ([]WorkloadNode, error) {
	pods, err := b.client.GetPods(ns)
	if err != nil {
		return nil, err
	}


	cronJobs, err := b.client.GetCronJobs(ns)
	if err != nil {
		return nil, err
	}

	seen := map[string]bool{}
	var nodes []WorkloadNode

	for _, cj := range cronJobs {
		uid := string(cj.UID)
		if seen[uid] {
			continue
		}
		seen[uid] = true
		nodes = append(nodes, WorkloadNode{
			ID:        uid,
			Label:     cj.Name,
			Namespace: cj.Namespace,
			Type:      NodeTypeCronJob,
			Labels:    cj.Spec.JobTemplate.Spec.Template.Labels,
		})
	}

	for _, pod := range pods {
		if pod.Status.Phase == corev1.PodSucceeded || pod.Status.Phase == corev1.PodFailed {
			continue
		}
		if len(pod.OwnerReferences) > 0 && pod.OwnerReferences[0].Kind == "Job" {
			continue
		}
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

// getSourceNodes finds all workload nodes the policy applies to via podSelector.
// An empty podSelector (catch-all) is collapsed to the namespace node to avoid N² edges.
func getSourceNodes(policy networkingv1.NetworkPolicy, nodes map[string]map[string][]*WorkloadNode) []*WorkloadNode {
	nsNodes := nodes[policy.Namespace]
	if isCatchAll(policy.Spec.PodSelector.MatchLabels, len(policy.Spec.PodSelector.MatchExpressions)) {
		if nsNode := findNSNode(nsNodes); nsNode != nil {
			return []*WorkloadNode{nsNode}
		}
	}
	return indexLabelMatch(policy.Spec.PodSelector.MatchLabels, nsNodes)
}

// find all policies for a single node, need so can show badges on node
func getNodePolicies(node WorkloadNode, policies []networkingv1.NetworkPolicy) []networkingv1.NetworkPolicy {
	var matches []networkingv1.NetworkPolicy
	for _, policy := range policies {
		sel := policy.Spec.PodSelector
		catchAll := isCatchAll(sel.MatchLabels, len(sel.MatchExpressions))
		if node.Type == NodeTypeNamespace {
			if catchAll {
				matches = append(matches, policy)
			}
			continue
		}
		if utils.IsLabelMach(sel.MatchLabels, node.Labels) {
			matches = append(matches, policy)
		}
	}
	return matches
}



func ownerUID(pod corev1.Pod) string {
	if len(pod.OwnerReferences) > 0 {
		return string(pod.OwnerReferences[0].UID)
	}
	return string(pod.UID)
}

func workloadLabel(pod corev1.Pod) string {
	if name, ok := pod.Labels["app"]; ok {
		return name
	}
	if name, ok := pod.Labels["app.kubernetes.io/name"]; ok {
		return name
	}
	if len(pod.OwnerReferences) > 0 {
		return pod.OwnerReferences[0].Name
	}
	return pod.Name
}

func makeLabelIndexKey(key string, value string) string {
	return key + "=" + value
}

// buildWorkloadIndex builds a flat label index for fast selector matching.
// Pointers reference the caller's slice — do not append to nodes after this returns.
func buildWorkloadIndex(nodes []WorkloadNode) map[string][]*WorkloadNode {
	index := make(map[string][]*WorkloadNode, len(nodes))
	for pos := range nodes {
		node := &nodes[pos]
		for key, value := range node.Labels {
			entry := makeLabelIndexKey(key, value)
			index[entry] = append(index[entry], node)
		}
	}
	return index
}