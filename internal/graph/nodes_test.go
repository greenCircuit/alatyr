package graph

import (
	"testing"

	"graph/internal/models"
	"graph/internal/utils"
)

// BuildWorkloadIndex

func TestBuildWorkloadIndex_SingleLabel(t *testing.T) {
	nodes := []models.WorkloadNode{
		{ID: "a", Labels: map[string]string{"app": "web"}},
	}
	idx := BuildWorkloadIndex(nodes)
	bucket := idx[utils.MakeLabelIndexKey("app", "web")]
	if len(bucket) != 1 || bucket[0].ID != "a" {
		t.Errorf("expected node a in bucket, got %v", bucket)
	}
}

func TestBuildWorkloadIndex_MultiLabel(t *testing.T) {
	nodes := []models.WorkloadNode{
		{ID: "a", Labels: map[string]string{"app": "web", "env": "prod"}},
	}
	idx := BuildWorkloadIndex(nodes)
	if len(idx[utils.MakeLabelIndexKey("app", "web")]) != 1 {
		t.Error("expected node in app=web bucket")
	}
	if len(idx[utils.MakeLabelIndexKey("env", "prod")]) != 1 {
		t.Error("expected node in env=prod bucket")
	}
}

func TestBuildWorkloadIndex_SharedBucket(t *testing.T) {
	nodes := []models.WorkloadNode{
		{ID: "a", Labels: map[string]string{"app": "api"}},
		{ID: "b", Labels: map[string]string{"app": "api"}},
	}
	idx := BuildWorkloadIndex(nodes)
	bucket := idx[utils.MakeLabelIndexKey("app", "api")]
	if len(bucket) != 2 {
		t.Errorf("expected 2 nodes in shared bucket, got %d", len(bucket))
	}
}

func TestBuildWorkloadIndex_EmptyNodes(t *testing.T) {
	idx := BuildWorkloadIndex(nil)
	if len(idx) != 0 {
		t.Errorf("expected empty index, got %d entries", len(idx))
	}
}
