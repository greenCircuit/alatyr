package models

// EvaluationResult is the per-engine output produced by PolicySource.Evaluate.
// Allow + Deny rules render as edges; PolicyStatuses feeds the status-key
// intersection layer for node badges.
type EvaluationResult struct {
	AllowByNs      map[string][]Rule
	DenyByNs       map[string][]Rule
	// saving node polices
	PolicyStatuses map[string]PolicyStatus // workloadID → per-engine PolicyStatus
	NodePolicies   map[string][]PolicyRef  // workloadID → policies that select it

}
