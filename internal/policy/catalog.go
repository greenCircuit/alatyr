package policy

import "alatyr/internal/models"

// AllStatusKeys returns the full catalog of status keys any policy engine
// may emit. Shared vocabulary across all engines (k8s NetworkPolicy, Istio
// AuthorizationPolicy, etc). Static — no client or graph build required.
// Used by /api/cluster-state to render filter chips before the graph loads.
func AllStatusKeys() []models.StatusKey {
	return []models.StatusKey{
		models.StatusInternetIngress,
		models.StatusInternetEgress,
		models.StatusInternetFull,
		models.StatusLanIngress,
		models.StatusLanEgress,
		models.StatusLanFull,
		models.StatusApiServerEgress,
		models.StatusIsolated,
		models.StatusCrossNamespace,
		models.StatusNamespaceEgress,
		models.StatusNamespaceIngress,
		models.StatusNamespaceFull,
		models.StatusL7Applied,
	}
}
