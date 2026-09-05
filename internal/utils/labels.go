package utils

import "alatyr/internal/models"

// compare to k8s labels and check if match labels will be applied
// src needs to have all labels of destination
func IsLabelMach(src map[string]string, dest map[string]string) bool {
	if len(src) > len(dest) {
		return false
	}

	// verify that dest label has the same key value as the same src
	for key, value := range src {
		if dest[key] != value {
			return false
		}
	}

	return true
}

func LabelsMatch(selector map[string]string, labels map[string]string) bool {
	for key, value := range selector {
		if labels[key] != value {
			return false
		}
	}
	return true
}

func IndexLabelMatch(selectors map[string]string, labelIndex map[string][]*models.WorkloadNode) []*models.WorkloadNode {
	if len(selectors) == 0 {
		return nil
	}
	matches := map[*models.WorkloadNode]bool{}
	first := true

	for key, value := range selectors {
		mapKey := MakeLabelIndexKey(key, value)
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

func MakeLabelIndexKey(key, value string) string {
	return key + "=" + value
}

// search ns index and return matching nodes
func NSIndexByLabel() {

}
