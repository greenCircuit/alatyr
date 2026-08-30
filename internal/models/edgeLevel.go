package models

// EdgeLevel mirrors PolicyEdge.level.
type EdgeLevel string

const (
	EdgeLevelWorkload  EdgeLevel = "workload"
	EdgeLevelNamespace EdgeLevel = "namespace"
)
