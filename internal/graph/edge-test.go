package graph

import (
	"testing"

	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var (
	nodeFrontend = WorkloadNode{ID: "frontend-uid", Labels: map[string]string{"app": "frontend"}, Namespace: "default"}
	nodeBackend  = WorkloadNode{ID: "backend-uid", Labels: map[string]string{"app": "backend"}, Namespace: "default"}
)

func testNodesByNS() map[string][]WorkloadNode {
	return map[string][]WorkloadNode{
		"default": {nodeFrontend, nodeBackend},
	}
}

func policyWithEgress(srcLabels, dstLabels map[string]string) networkingv1.NetworkPolicy {
	return networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{Namespace: "default"},
		Spec: networkingv1.NetworkPolicySpec{
			PodSelector: metav1.LabelSelector{MatchLabels: srcLabels},
			Egress: []networkingv1.NetworkPolicyEgressRule{
				{To: []networkingv1.NetworkPolicyPeer{
					{PodSelector: &metav1.LabelSelector{MatchLabels: dstLabels}},
				}},
			},
		},
	}
}

func policyWithIngress(dstLabels, srcLabels map[string]string) networkingv1.NetworkPolicy {
	return networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{Namespace: "default"},
		Spec: networkingv1.NetworkPolicySpec{
			PodSelector: metav1.LabelSelector{MatchLabels: dstLabels},
			Ingress: []networkingv1.NetworkPolicyIngressRule{
				{From: []networkingv1.NetworkPolicyPeer{
					{PodSelector: &metav1.LabelSelector{MatchLabels: srcLabels}},
				}},
			},
		},
	}
}

// buildEdges

func TestBuildEdges_EgressDirection(t *testing.T) {
	policy := policyWithEgress(
		map[string]string{"app": "frontend"},
		map[string]string{"app": "backend"},
	)
	edges := buildEdges(testNodesByNS(), []networkingv1.NetworkPolicy{policy})

	if len(edges) != 1 {
		t.Fatalf("expected 1 edge, got %d", len(edges))
	}
	e := edges[0]
	if e.Source != nodeFrontend.ID {
		t.Errorf("egress source: want %s, got %s", nodeFrontend.ID, e.Source)
	}
	if e.Target != nodeBackend.ID {
		t.Errorf("egress target: want %s, got %s", nodeBackend.ID, e.Target)
	}
	if e.Direction != DirectionEgress {
		t.Errorf("expected egress direction, got %s", e.Direction)
	}
}

func TestBuildEdges_IngressDirection(t *testing.T) {
	// policy selects backend (receiver), ingress from frontend (sender)
	policy := policyWithIngress(
		map[string]string{"app": "backend"},
		map[string]string{"app": "frontend"},
	)
	edges := buildEdges(testNodesByNS(), []networkingv1.NetworkPolicy{policy})

	if len(edges) != 1 {
		t.Fatalf("expected 1 edge, got %d", len(edges))
	}
	e := edges[0]
	if e.Source != nodeFrontend.ID {
		t.Errorf("ingress source: want %s (sender), got %s", nodeFrontend.ID, e.Source)
	}
	if e.Target != nodeBackend.ID {
		t.Errorf("ingress target: want %s (receiver), got %s", nodeBackend.ID, e.Target)
	}
	if e.Direction != DirectionIngress {
		t.Errorf("expected ingress direction, got %s", e.Direction)
	}
}

func TestBuildEdges_NoPolicies(t *testing.T) {
	edges := buildEdges(testNodesByNS(), nil)
	if len(edges) != 0 {
		t.Errorf("expected no edges, got %d", len(edges))
	}
}

func TestBuildEdges_NoMatchingNodes(t *testing.T) {
	policy := policyWithEgress(
		map[string]string{"app": "unknown"},
		map[string]string{"app": "backend"},
	)
	edges := buildEdges(testNodesByNS(), []networkingv1.NetworkPolicy{policy})
	if len(edges) != 0 {
		t.Errorf("expected no edges when source selector matches nothing, got %d", len(edges))
	}
}

// getTargetEgressNodes

func TestGetTargetEgressNodes_Match(t *testing.T) {
	policy := policyWithEgress(
		map[string]string{"app": "frontend"},
		map[string]string{"app": "backend"},
	)
	edges := getTargetEgressNodes(policy, testNodesByNS())
	if len(edges) != 1 {
		t.Fatalf("expected 1 egress edge, got %d", len(edges))
	}
	if edges[0].Target != nodeBackend.ID {
		t.Errorf("want target %s, got %s", nodeBackend.ID, edges[0].Target)
	}
	if edges[0].Direction != DirectionEgress {
		t.Errorf("expected egress direction, got %s", edges[0].Direction)
	}
}

func TestGetTargetEgressNodes_NilPodSelector(t *testing.T) {
	policy := networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{Namespace: "default"},
		Spec: networkingv1.NetworkPolicySpec{
			Egress: []networkingv1.NetworkPolicyEgressRule{
				{To: []networkingv1.NetworkPolicyPeer{{PodSelector: nil}}},
			},
		},
	}
	edges := getTargetEgressNodes(policy, testNodesByNS())
	if len(edges) != 0 {
		t.Errorf("nil PodSelector should be skipped, got %d edges", len(edges))
	}
}

func TestGetTargetEgressNodes_NoRules(t *testing.T) {
	policy := networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{Namespace: "default"},
	}
	edges := getTargetEgressNodes(policy, testNodesByNS())
	if len(edges) != 0 {
		t.Errorf("expected no edges, got %d", len(edges))
	}
}

// getTargetIngressNodes

func TestGetTargetIngressNodes_Match(t *testing.T) {
	policy := policyWithIngress(
		map[string]string{"app": "backend"},
		map[string]string{"app": "frontend"},
	)
	edges := getTargetIngressNodes(policy, testNodesByNS())
	if len(edges) != 1 {
		t.Fatalf("expected 1 ingress edge, got %d", len(edges))
	}
	if edges[0].Target != nodeFrontend.ID {
		t.Errorf("want target %s, got %s", nodeFrontend.ID, edges[0].Target)
	}
	if edges[0].Direction != DirectionIngress {
		t.Errorf("expected ingress direction, got %s", edges[0].Direction)
	}
}

func TestGetTargetIngressNodes_NilPodSelector(t *testing.T) {
	policy := networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{Namespace: "default"},
		Spec: networkingv1.NetworkPolicySpec{
			Ingress: []networkingv1.NetworkPolicyIngressRule{
				{From: []networkingv1.NetworkPolicyPeer{{PodSelector: nil}}},
			},
		},
	}
	edges := getTargetIngressNodes(policy, testNodesByNS())
	if len(edges) != 0 {
		t.Errorf("nil PodSelector should be skipped, got %d edges", len(edges))
	}
}

func TestGetTargetIngressNodes_NoRules(t *testing.T) {
	policy := networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{Namespace: "default"},
	}
	edges := getTargetIngressNodes(policy, testNodesByNS())
	if len(edges) != 0 {
		t.Errorf("expected no edges, got %d", len(edges))
	}
}
