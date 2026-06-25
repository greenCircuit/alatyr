package graph

import (
	"graph/internal/models"
	"graph/internal/utils"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
)

// AssembleNsIndex builds a per-namespace NSIndex from already-fetched k8s
// objects. Pure — no k8s client. Inputs come from store.fetchNsIndex.
// Output: workloads + label index + namespace node. Skips Pods in
// Succeeded/Failed phase and Pods owned by a Job.
func AssembleNsIndex(pods []*corev1.Pod, cronJobs []*batchv1.CronJob, nsObj *corev1.Namespace) models.NSIndex {
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
		uid := OwnerUID(pod)
		if seen[uid] {
			continue
		}
		seen[uid] = true

		nodes = append(nodes, models.WorkloadNode{
			ID:        uid,
			Label:     WorkloadLabel(pod),
			Namespace: pod.Namespace,
			Type:      models.NodeTypeDeployment,
			Labels:    pod.Labels,
		})
	}

	nsNode := models.WorkloadNode{
		ID:        "ns-" + nsObj.Name,
		Label:     nsObj.Name,
		Namespace: nsObj.Name, // k8s Namespace objects are cluster-scoped; .Namespace is empty. Use .Name so tests + UI can address it as a ns.
		Type:      models.NodeTypeNamespace,
		Labels:    nsObj.Labels,
	}
	nodes = append(nodes, nsNode)

	return models.NSIndex{
		Workloads:  nodes,
		LabelIndex: BuildWorkloadIndex(nodes),
		NSNode:     &nsNode,
	}
}

// OwnerUID returns the workload-level UID for a pod (controller UID when
// owned, pod UID otherwise). Same workload's pods share this id.
func OwnerUID(pod *corev1.Pod) string {
	if len(pod.OwnerReferences) > 0 {
		return string(pod.OwnerReferences[0].UID)
	}
	return string(pod.UID)
}

// WorkloadLabel picks a display label for a pod: app label > app.kubernetes.io/name
// > owner name > pod name.
func WorkloadLabel(pod *corev1.Pod) string {
	var component string
	if comp, ok := pod.Labels["app.kubernetes.io/component"]; ok {
		component = comp
	}

	if name, ok := pod.Labels["app"]; ok {
		if component != "" && component != name {
			return name + "-" + component
		} else {
			return name
		}
	}
	if name, ok := pod.Labels["app.kubernetes.io/name"]; ok {
		if component != "" && component != name {
			return name + "-" + component
		} else {
			return name
		}
	}
	if len(pod.OwnerReferences) > 0 {
		return pod.OwnerReferences[0].Name
	}
	return pod.Name
}

// BuildWorkloadIndex builds a flat label index for fast selector matching.
// Called once per namespace during NSIndex assembly; engines receive the
// pre-built index and never rebuild.
// Pointers reference the caller's slice — do not append to nodes after this returns.
func BuildWorkloadIndex(nodes []models.WorkloadNode) map[string][]*models.WorkloadNode {
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
