package utils

// compare to k8s labels and check if match labels will be applied
// src needs to have all labels of destination
func IsLabelMach(src map[string]string, dest map[string]string) bool {
	if len(src) > len(dest) {
		return false
	}

	// verify that dest label has the same key value as the same src
	for key, value := range src {
		if dest[key] != value {
			return  false
		}
	}

	return true
}