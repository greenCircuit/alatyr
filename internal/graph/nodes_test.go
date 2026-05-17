package graph

import "testing"

// buildWorkloadIndex

func TestBuildWorkloadIndex_SingleLabel(t *testing.T) {
	nodes := []WorkloadNode{
		{ID: "a", Labels: map[string]string{"app": "web"}},
	}
	idx := buildWorkloadIndex(nodes)
	bucket := idx[makeLabelIndexKey("app", "web")]
	if len(bucket) != 1 || bucket[0].ID != "a" {
		t.Errorf("expected node a in bucket, got %v", bucket)
	}
}

func TestBuildWorkloadIndex_MultiLabel(t *testing.T) {
	nodes := []WorkloadNode{
		{ID: "a", Labels: map[string]string{"app": "web", "env": "prod"}},
	}
	idx := buildWorkloadIndex(nodes)
	if len(idx[makeLabelIndexKey("app", "web")]) != 1 {
		t.Error("expected node in app=web bucket")
	}
	if len(idx[makeLabelIndexKey("env", "prod")]) != 1 {
		t.Error("expected node in env=prod bucket")
	}
}

func TestBuildWorkloadIndex_SharedBucket(t *testing.T) {
	nodes := []WorkloadNode{
		{ID: "a", Labels: map[string]string{"app": "api"}},
		{ID: "b", Labels: map[string]string{"app": "api"}},
	}
	idx := buildWorkloadIndex(nodes)
	bucket := idx[makeLabelIndexKey("app", "api")]
	if len(bucket) != 2 {
		t.Errorf("expected 2 nodes in shared bucket, got %d", len(bucket))
	}
}

func TestBuildWorkloadIndex_EmptyNodes(t *testing.T) {
	idx := buildWorkloadIndex(nil)
	if len(idx) != 0 {
		t.Errorf("expected empty index, got %d entries", len(idx))
	}
}

// indexLabelMatch

func TestIndexLabelMatch_SingleLabel(t *testing.T) {
	nodes := []WorkloadNode{
		{ID: "a", Labels: map[string]string{"app": "web"}},
		{ID: "b", Labels: map[string]string{"app": "api"}},
	}
	idx := buildWorkloadIndex(nodes)
	result := indexLabelMatch(map[string]string{"app": "web"}, idx)
	if len(result) != 1 || result[0].ID != "a" {
		t.Errorf("expected node a, got %v", result)
	}
}

func TestIndexLabelMatch_MultiLabelIntersection(t *testing.T) {
	nodes := []WorkloadNode{
		{ID: "a", Labels: map[string]string{"app": "api", "tier": "backend"}},
		{ID: "b", Labels: map[string]string{"app": "api", "tier": "frontend"}},
	}
	idx := buildWorkloadIndex(nodes)
	result := indexLabelMatch(map[string]string{"app": "api", "tier": "backend"}, idx)
	if len(result) != 1 || result[0].ID != "a" {
		t.Errorf("expected only node a after intersection, got %v", result)
	}
}

func TestIndexLabelMatch_ImpossibleCombo(t *testing.T) {
	nodes := []WorkloadNode{
		{ID: "a", Labels: map[string]string{"app": "web", "tier": "frontend"}},
		{ID: "b", Labels: map[string]string{"app": "api", "tier": "backend"}},
	}
	idx := buildWorkloadIndex(nodes)
	// no node has app=web AND tier=backend
	result := indexLabelMatch(map[string]string{"app": "web", "tier": "backend"}, idx)
	if len(result) != 0 {
		t.Errorf("expected nil for impossible label combo, got %v", result)
	}
}

func TestIndexLabelMatch_NoMatch(t *testing.T) {
	nodes := []WorkloadNode{
		{ID: "a", Labels: map[string]string{"app": "web"}},
	}
	idx := buildWorkloadIndex(nodes)
	result := indexLabelMatch(map[string]string{"app": "api"}, idx)
	if len(result) != 0 {
		t.Errorf("expected no matches, got %v", result)
	}
}

func TestIndexLabelMatch_MissingBucket(t *testing.T) {
	nodes := []WorkloadNode{
		{ID: "a", Labels: map[string]string{"app": "web"}},
	}
	idx := buildWorkloadIndex(nodes)
	result := indexLabelMatch(map[string]string{"env": "prod"}, idx)
	if result != nil {
		t.Errorf("missing bucket should return nil, got %v", result)
	}
}

func TestIndexLabelMatch_EmptySelector(t *testing.T) {
	nodes := []WorkloadNode{
		{ID: "a", Labels: map[string]string{"app": "web"}},
	}
	idx := buildWorkloadIndex(nodes)
	result := indexLabelMatch(map[string]string{}, idx)
	if result != nil {
		t.Errorf("empty selector should return nil, got %v", result)
	}
}
