package k8s

import (
	"testing"

	istioapi "istio.io/api/security/v1beta1"
	istiosec "istio.io/client-go/pkg/apis/security/v1"
	istiofake "istio.io/client-go/pkg/clientset/versioned/fake"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Regression guard against the live-mode stub bug: GetAuthorizationPolicies
// used to `return nil, nil` and was caught only by hitting a real cluster.
// This test wires a fake istio clientset, seeds an AuthorizationPolicy in a
// namespace, and asserts the method actually returns it.
func TestGetAuthorizationPolicies_FetchesFromIstioClientset(t *testing.T) {
	seed := &istiosec.AuthorizationPolicy{
		ObjectMeta: metav1.ObjectMeta{Name: "grafana-access", Namespace: "observability"},
		Spec: istioapi.AuthorizationPolicy{
			Action: istioapi.AuthorizationPolicy_ALLOW,
		},
	}
	fakeClientset := istiofake.NewSimpleClientset(seed)
	client := &Client{istioClientset: fakeClientset}

	out, err := client.GetAuthorizationPolicies("observability")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(out) != 1 || out[0].Name != "grafana-access" {
		t.Fatalf("got %+v, want one policy named grafana-access", out)
	}

	// Empty namespace returns no policies (not nil clientset).
	empty, err := client.GetAuthorizationPolicies("other-ns")
	if err != nil {
		t.Fatalf("unexpected error querying empty ns: %v", err)
	}
	if len(empty) != 0 {
		t.Errorf("empty namespace returned %d policies, want 0", len(empty))
	}
}
