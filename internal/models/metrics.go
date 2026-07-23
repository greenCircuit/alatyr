package models

type ClusterMetrics struct {
	NsTotal       int32       `json:"nsTotal"`
	WorkloadTotal int32       `json:"workloadTotal"`
	MeshMetrics   MeshMetrics `json:"meshMetrics"`
}
