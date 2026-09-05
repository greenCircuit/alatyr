package graph

import (
	"testing"

	"alatyr/internal/models"
	"alatyr/internal/utils"
)

// utils.IndexLabelMatch — exercised through the graph-built workload index
// so tests stay close to the BuildWorkloadIndex producer.

func TestIndexLabelMatch_SingleLabel(t *testing.T) {
	nodes := []models.WorkloadNode{
		{ID: "a", Labels: map[string]string{"app": "web"}},
		{ID: "b", Labels: map[string]string{"app": "api"}},
	}
	idx := BuildWorkloadIndex(nodes)
	result := utils.IndexLabelMatch(map[string]string{"app": "web"}, idx)
	if len(result) != 1 || result[0].ID != "a" {
		t.Errorf("expected node a, got %v", result)
	}
}

func TestIndexLabelMatch_MultiLabelIntersection(t *testing.T) {
	nodes := []models.WorkloadNode{
		{ID: "a", Labels: map[string]string{"app": "api", "tier": "backend"}},
		{ID: "b", Labels: map[string]string{"app": "api", "tier": "frontend"}},
	}
	idx := BuildWorkloadIndex(nodes)
	result := utils.IndexLabelMatch(map[string]string{"app": "api", "tier": "backend"}, idx)
	if len(result) != 1 || result[0].ID != "a" {
		t.Errorf("expected only node a after intersection, got %v", result)
	}
}

func TestIndexLabelMatch_ImpossibleCombo(t *testing.T) {
	nodes := []models.WorkloadNode{
		{ID: "a", Labels: map[string]string{"app": "web", "tier": "frontend"}},
		{ID: "b", Labels: map[string]string{"app": "api", "tier": "backend"}},
	}
	idx := BuildWorkloadIndex(nodes)
	result := utils.IndexLabelMatch(map[string]string{"app": "web", "tier": "backend"}, idx)
	if len(result) != 0 {
		t.Errorf("expected nil for impossible label combo, got %v", result)
	}
}

func TestIndexLabelMatch_NoMatch(t *testing.T) {
	nodes := []models.WorkloadNode{
		{ID: "a", Labels: map[string]string{"app": "web"}},
	}
	idx := BuildWorkloadIndex(nodes)
	result := utils.IndexLabelMatch(map[string]string{"app": "api"}, idx)
	if len(result) != 0 {
		t.Errorf("expected no matches, got %v", result)
	}
}

func TestIndexLabelMatch_MissingBucket(t *testing.T) {
	nodes := []models.WorkloadNode{
		{ID: "a", Labels: map[string]string{"app": "web"}},
	}
	idx := BuildWorkloadIndex(nodes)
	result := utils.IndexLabelMatch(map[string]string{"env": "prod"}, idx)
	if result != nil {
		t.Errorf("missing bucket should return nil, got %v", result)
	}
}

func TestIndexLabelMatch_EmptySelector(t *testing.T) {
	nodes := []models.WorkloadNode{
		{ID: "a", Labels: map[string]string{"app": "web"}},
	}
	idx := BuildWorkloadIndex(nodes)
	result := utils.IndexLabelMatch(map[string]string{}, idx)
	if result != nil {
		t.Errorf("empty selector should return nil, got %v", result)
	}
}
