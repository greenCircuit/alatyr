package models

// Cache holds per-namespace state reused across graph builds.
// NsIndex skips the k8s pod/cronjob fetch on cache hit; EvaluationResults
// skips per-engine policy evaluation. Populated by store, read by graph.
type Cache struct {
	NsIndex           map[string]NSIndex
	EvaluationResults map[string]EvaluationResult // keyed by PolicySource.Name()
}
