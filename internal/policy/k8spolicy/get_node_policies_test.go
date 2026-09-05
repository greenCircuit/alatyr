package k8spolicy

import (
	"testing"

	"alatyr/internal/models"

	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ── getNodePolicies tests ─────────────────────────────────────────────────────

func TestGetNodePolicies_MatchesByLabel(t *testing.T) {
	node := models.WorkloadNode{Labels: map[string]string{"app": "foo"}}
	matchingPolicy := networkingv1.NetworkPolicy{Spec: networkingv1.NetworkPolicySpec{
		PodSelector: metav1.LabelSelector{MatchLabels: map[string]string{"app": "foo"}},
	}}
	nonMatchingPolicy := networkingv1.NetworkPolicy{Spec: networkingv1.NetworkPolicySpec{
		PodSelector: metav1.LabelSelector{MatchLabels: map[string]string{"app": "bar"}},
	}}
	got := getNodePolicies(node, []*networkingv1.NetworkPolicy{&matchingPolicy, &nonMatchingPolicy})
	if len(got) != 1 {
		t.Errorf("expected 1 matching policy, got %d", len(got))
	}
}

// Empty podSelector matches all nodes.
func TestGetNodePolicies_EmptyPodSelector_MatchesAll(t *testing.T) {
	node := models.WorkloadNode{Labels: map[string]string{"app": "foo"}}
	networkPolicy := networkingv1.NetworkPolicy{Spec: networkingv1.NetworkPolicySpec{
		PodSelector: metav1.LabelSelector{},
	}}
	got := getNodePolicies(node, []*networkingv1.NetworkPolicy{&networkPolicy})
	if len(got) != 1 {
		t.Errorf("expected 1 policy for empty podSelector, got %d", len(got))
	}
}

// Namespace node only receives catch-all policies (podSelector: {}), not pod-specific ones.
func TestGetNodePolicies_NamespaceNode_OnlyCatchAll(t *testing.T) {
	nsNode := models.WorkloadNode{Type: models.NodeTypeNamespace, Labels: map[string]string{"kubernetes.io/metadata.name": "ns-a"}}
	catchAllPolicy := networkingv1.NetworkPolicy{Spec: networkingv1.NetworkPolicySpec{
		PodSelector: metav1.LabelSelector{},
	}}
	specificPolicy := networkingv1.NetworkPolicy{Spec: networkingv1.NetworkPolicySpec{
		PodSelector: metav1.LabelSelector{MatchLabels: map[string]string{"app": "foo"}},
	}}
	got := getNodePolicies(nsNode, []*networkingv1.NetworkPolicy{&catchAllPolicy, &specificPolicy})
	if len(got) != 1 {
		t.Errorf("expected only catch-all policy for namespace node, got %d", len(got))
	}
}

// No label overlap → no match.
func TestGetNodePolicies_NoMatch(t *testing.T) {
	node := models.WorkloadNode{Labels: map[string]string{"app": "foo"}}
	networkPolicy := networkingv1.NetworkPolicy{Spec: networkingv1.NetworkPolicySpec{
		PodSelector: metav1.LabelSelector{MatchLabels: map[string]string{"app": "bar"}},
	}}
	got := getNodePolicies(node, []*networkingv1.NetworkPolicy{&networkPolicy})
	if len(got) != 0 {
		t.Errorf("expected 0 policies, got %d", len(got))
	}
}
