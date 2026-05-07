package graph

import (
	"testing"
)

func TestFindNodeByLabel_ExactMatch(t *testing.T) {
	nodes := []WorkloadNode{
		{ID: "a", Labels: map[string]string{"app": "frontend"}},
		{ID: "b", Labels: map[string]string{"app": "backend"}},
	}
	got := findNodeByLabel(map[string]string{"app": "frontend"}, nodes)
	if len(got) != 1 || got[0].ID != "a" {
		t.Errorf("expected node a, got %v", got)
	}
}

func TestFindNodeByLabel_MultipleMatch(t *testing.T) {
	nodes := []WorkloadNode{
		{ID: "a", Labels: map[string]string{"app": "web", "env": "prod"}},
		{ID: "b", Labels: map[string]string{"app": "web", "env": "staging"}},
	}
	got := findNodeByLabel(map[string]string{"app": "web"}, nodes)
	if len(got) != 2 {
		t.Errorf("expected 2 nodes, got %d", len(got))
	}
}

func TestFindNodeByLabel_NoMatch(t *testing.T) {
	nodes := []WorkloadNode{
		{ID: "a", Labels: map[string]string{"app": "frontend"}},
	}
	got := findNodeByLabel(map[string]string{"app": "backend"}, nodes)
	if len(got) != 0 {
		t.Errorf("expected no matches, got %v", got)
	}
}

func TestFindNodeByLabel_EmptyLabelMap(t *testing.T) {
	nodes := []WorkloadNode{
		{ID: "a", Labels: map[string]string{"app": "frontend"}},
		{ID: "b", Labels: map[string]string{"app": "backend"}},
	}
	got := findNodeByLabel(map[string]string{}, nodes)
	if len(got) != 2 {
		t.Errorf("expected all nodes to match, got %d", len(got))
	}
}

func TestFindNodeByLabel_EmptyNodes(t *testing.T) {
	got := findNodeByLabel(map[string]string{"app": "frontend"}, []WorkloadNode{})
	if len(got) != 0 {
		t.Errorf("expected no matches, got %v", got)
	}
}

func TestFindNodeByLabel_PartialLabelMatch(t *testing.T) {
	nodes := []WorkloadNode{
		{ID: "a", Labels: map[string]string{"app": "web", "env": "prod"}},
	}
	// node has both labels but selector requires both — should match
	got := findNodeByLabel(map[string]string{"app": "web", "env": "prod"}, nodes)
	if len(got) != 1 {
		t.Errorf("expected 1 match, got %d", len(got))
	}
	// wrong value for one key — should not match
	got = findNodeByLabel(map[string]string{"app": "web", "env": "staging"}, nodes)
	if len(got) != 0 {
		t.Errorf("expected no match, got %v", got)
	}
}
