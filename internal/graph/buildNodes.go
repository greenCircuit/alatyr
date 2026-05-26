package graph


import (
	"graph/internal/models"
	"graph/internal/utils"

	corev1 "k8s.io/api/core/v1"
)

func (b *Builder) buildNsIndex(ns string) (models.NSIndex, error) {
	var nsIndex models.NSIndex
	pods, err := b.client.GetPods(ns)
	if err != nil {
		return nsIndex, err
	}


	cronJobs, err := b.client.GetCronJobs(ns)
	if err != nil {
		return nsIndex, err
	}

	seen := map[string]bool{}
	var nodes []models.WorkloadNode

	for _, cj := range cronJobs {
		uid := string(cj.UID)
		if seen[uid] {
			continue
		}
		seen[uid] = true
		nodes = append(nodes, models.WorkloadNode{
			ID:        uid,
			Label:     cj.Name,
			Namespace: cj.Namespace,
			Type:      models.NodeTypeCronJob,
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

		nodes = append(nodes, models.WorkloadNode{
			ID:        uid,
			Label:     workloadLabel(pod),
			Namespace: pod.Namespace,
			Type:      models.NodeTypeDeployment,
			Labels:    pod.Labels,
		})
	}

	// add namespace node
	nsObj, err := b.client.GetNs(ns)
	if err != nil {
		return nsIndex, err
	}

	nsNode := models.WorkloadNode{
		ID:        "ns-" + nsObj.Name,
		Label:     nsObj.Name,
		Namespace: nsObj.Namespace,
		Type:      models.NodeTypeNamespace,
		Labels:    nsObj.Labels,
	}

	nodes = append(nodes, nsNode)

	nodeIndex := buildWorkloadIndex(nodes)

	nsIndex.Workloads = nodes
	nsIndex.LabelIndex = nodeIndex
	nsIndex.NSNode = &nsNode

	return nsIndex, nil
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

// buildWorkloadIndex builds a flat label index for fast selector matching.
// Called once per namespace during graph construction; engines receive the
// pre-built index and never rebuild.
// Pointers reference the caller's slice — do not append to nodes after this returns.
func buildWorkloadIndex(nodes []models.WorkloadNode) map[string][]*models.WorkloadNode {
	index := make(map[string][]*models.WorkloadNode, len(nodes))
	for position := range nodes {
		node := &nodes[position]
		for key, value := range node.Labels {
			entry := utils.MakeLabelIndexKey(key, value)
			index[entry] = append(index[entry], node)
		}
	}
	return index
}
