package graph

import (
	"graph/internal/models"
	"graph/internal/utils"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

// AssembleNsIndex builds a per-namespace NSIndex from already-fetched k8s
// objects. Pure — no k8s client. Inputs come from store.fetchNsIndex.
// Output: workloads + label index + namespace node. Emits only workloads that
// can currently carry traffic. Skips Succeeded/Failed Pods and Pods whose
// controller already has a node.
func AssembleNsIndex(nsResources NsResources) models.NSIndex {
	seen := map[string]bool{}
	var nodes []models.WorkloadNode

	for _, cj := range nsResources.CronJobs {
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

	for _, obj := range nsResources.StatefulSets{
		uid := string(obj.UID)
		if seen[uid] {
			continue
		}
		seen[uid] = true
		nodes = append(nodes, models.WorkloadNode{
			ID:        uid,
			Label:     obj.Name,
			Namespace: obj.Namespace,
			Type:      models.NodeTypeStatefullSet,
			Labels:    obj.Spec.Template.Labels,
		})
	}

	for _, obj := range nsResources.DaemonSets{
		uid := string(obj.UID)
		if seen[uid] {
			continue
		}
		seen[uid] = true
		nodes = append(nodes, models.WorkloadNode{
			ID:        uid,
			Label:     obj.Name,
			Namespace: obj.Namespace,
			Type:      models.NodeTypeDaemonset,
			Labels:    obj.Spec.Template.Labels,
		})
	}
	for _, obj := range nsResources.Deployments{
		uid := string(obj.UID)
		if seen[uid] {
			continue
		}
		seen[uid] = true
		nodes = append(nodes, models.WorkloadNode{
			ID:        uid,
			Label:     obj.Name,
			Namespace: obj.Namespace,
			Type:      models.NodeTypeDeployment,
			Labels:    obj.Spec.Template.Labels,
		})
	}


	jobsByUID := make(map[types.UID]*batchv1.Job, len(nsResources.Jobs))
	for _, job := range nsResources.Jobs {
		jobsByUID[job.UID] = job
	}

	for _, pod := range nsResources.Pods {
		// phase not readiness — crashloop pod stays Running, still holds an IP
		if pod.Status.Phase == corev1.PodSucceeded || pod.Status.Phase == corev1.PodFailed {
			continue
		}

		nodeType := models.NodeTypePod
		controller := metav1.GetControllerOf(pod)
		if controller != nil {
			switch controller.Kind {
			case "ReplicaSet", "StatefulSet", "DaemonSet", "CronJob":
				continue // long-running controller already emitted above
			case "Job":
				// CronJob-owned Job — CronJob node has same template labels.
				// Cache miss emits anyway: duplicate node beats a hole.
				if job, ok := jobsByUID[controller.UID]; ok && metav1.GetControllerOf(job) != nil {
					continue
				}
				nodeType = models.NodeTypeJob
			}
			// other kinds (operator CR, static pod) fall through, grouped by controller UID
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
			Type:      nodeType,
			Labels:    pod.Labels,
		})
	}

	nsNode := models.WorkloadNode{
		ID:        "ns-" + nsResources.Namespace.Name,
		Label:     nsResources.Namespace.Name,
		Namespace: nsResources.Namespace.Namespace, // k8s Namespace objects are cluster-scoped; .Namespace is empty. Use .Name so tests + UI can address it as a ns.
		Type:      models.NodeTypeNamespace,
		Labels:    nsResources.Namespace.Labels,
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
// Controller ref, not OwnerReferences[0] — non-controller refs can sit at index 0.
func OwnerUID(pod *corev1.Pod) string {
	if controller := metav1.GetControllerOf(pod); controller != nil {
		return string(controller.UID)
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
	if controller := metav1.GetControllerOf(pod); controller != nil {
		return controller.Name
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
