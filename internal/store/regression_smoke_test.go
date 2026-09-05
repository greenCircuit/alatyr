package store

import (
	"log/slog"
	"testing"

	"alatyr/internal/models"

	istioapi "istio.io/api/security/v1beta1"
	istioapitype "istio.io/api/type/v1beta1"
	istiosec "istio.io/client-go/pkg/apis/security/v1"
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	calicov3 "github.com/projectcalico/api/pkg/apis/projectcalico/v3"
)

// fakeClient is a KubernetesClient that returns pre-baked pods + policies
// per namespace. Enough surface to drive Builder.PopulateCache end-to-end
// without touching an apiserver — mirrors what the e2e suite drives through
// kwok. Regressions that show up in e2e but not in per-engine unit tests
// live in the population/stamping pipeline; this fixture exercises exactly
// that pipeline.
type fakeClient struct {
	pods                  map[string][]*corev1.Pod
	networkPolicies       map[string][]*networkingv1.NetworkPolicy
	authorizationPolicies map[string][]*istiosec.AuthorizationPolicy
	namespaces            []string
}

func (c *fakeClient) GetPods(ns string) ([]*corev1.Pod, error)              { return c.pods[ns], nil }
func (c *fakeClient) GetCronJobs(string) ([]*batchv1.CronJob, error)        { return nil, nil }
func (c *fakeClient) GetJobs(string) ([]*batchv1.Job, error)                { return nil, nil }
func (c *fakeClient) GetDeployments(string) ([]*appsv1.Deployment, error)   { return nil, nil }
func (c *fakeClient) GetStatefulSets(string) ([]*appsv1.StatefulSet, error) { return nil, nil }
func (c *fakeClient) GetDaemonSets(string) ([]*appsv1.DaemonSet, error)     { return nil, nil }
func (c *fakeClient) GetPolicies(ns string) ([]*networkingv1.NetworkPolicy, error) {
	return c.networkPolicies[ns], nil
}
func (c *fakeClient) GetNsNames() ([]string, error) { return c.namespaces, nil }
func (c *fakeClient) GetNs(ns string) (*corev1.Namespace, error) {
	return &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name:   ns,
			Labels: map[string]string{"kubernetes.io/metadata.name": ns},
		},
	}, nil
}
func (c *fakeClient) GetAuthorizationPolicies(ns string) ([]*istiosec.AuthorizationPolicy, error) {
	return c.authorizationPolicies[ns], nil
}
func (c *fakeClient) GetPeerAuthentications(string) ([]*istiosec.PeerAuthentication, error) {
	return nil, nil
}
func (c *fakeClient) GetGlobalNetworkPolicies() ([]*calicov3.GlobalNetworkPolicy, error) {
	return nil, nil
}
func (c *fakeClient) GetK8sPolicyByName(string, string) (*networkingv1.NetworkPolicy, error) {
	return nil, nil
}
func (c *fakeClient) GetAuthorizationPoliciesByName(string, string) (*istiosec.AuthorizationPolicy, error) {
	return nil, nil
}
func (c *fakeClient) GetPeerAuthenticationsByName(string, string) (*istiosec.PeerAuthentication, error) {
	return nil, nil
}
func (c *fakeClient) GetGlobalNetworkPolicyByName(string) (*calicov3.GlobalNetworkPolicy, error) {
	return nil, nil
}

func makePod(namespace, name, uid string, labels map[string]string) *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      name,
			UID:       types.UID(uid),
			Labels:    labels,
		},
	}
}

// Mirrors tests/k8s/scenarios.py:allow_fe_to_be — ns_a NetworkPolicy
// selects backend, allows ingress from frontend on TCP/8080. Expected e2e
// keys for backend: {internet-egress} (ingress locked, egress open, no
// cross-ns / lan / internet / inner-ns signals).
//
// This test runs the SAME data through Builder.PopulateCache — the
// pipeline the /api/graph handler serves from since commit fd74c29.
// If it passes but the e2e keeps returning {internet-full}, the delta
// is in the informer/apiserver path, not in the Go population pipeline.
func TestPopulateCache_AllowFeToBe_StampsK8sIngressLock(t *testing.T) {
	namespace := "ns-a"
	backendPod := makePod(namespace, "backend", "backend-uid", map[string]string{"app": "backend"})
	frontendPod := makePod(namespace, "frontend", "frontend-uid", map[string]string{"app": "frontend"})

	networkPolicy := &networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{Namespace: namespace, Name: "allow-fe-be"},
		Spec: networkingv1.NetworkPolicySpec{
			PodSelector: metav1.LabelSelector{MatchLabels: map[string]string{"app": "backend"}},
			PolicyTypes: []networkingv1.PolicyType{networkingv1.PolicyTypeIngress},
			Ingress: []networkingv1.NetworkPolicyIngressRule{{
				From: []networkingv1.NetworkPolicyPeer{{
					PodSelector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "frontend"}},
				}},
			}},
		},
	}

	client := &fakeClient{
		pods:            map[string][]*corev1.Pod{namespace: {backendPod, frontendPod}},
		networkPolicies: map[string][]*networkingv1.NetworkPolicy{namespace: {networkPolicy}},
		namespaces:      []string{namespace},
	}

	builder := NewBuilder(client, slog.Default())
	cache := &models.Cache{}
	if _, err := builder.PopulateCache(cache, []string{namespace}); err != nil {
		t.Fatalf("PopulateCache: %v", err)
	}

	backend := findWorkload(t, cache, namespace, "backend")
	got := backend.StatusesBySource["k8s"]
	assertStatusKeys(t, got, models.StatusInternetEgress)
	assertNoStatusKey(t, got, models.StatusInternetFull)
}

// Mirrors tests/istio/scenarios.py:allow_from_same_ns — ns_a
// AuthorizationPolicy selects backend, ALLOW from ns_a. Expected e2e
// keys for backend: {internet-egress, ns-ingress-access}.
func TestPopulateCache_IstioAllowSameNs_StampsIngressLockAndInnerNs(t *testing.T) {
	namespace := "ns-a"
	backendPod := makePod(namespace, "backend", "backend-uid", map[string]string{"app": "backend"})

	authzPolicy := &istiosec.AuthorizationPolicy{
		ObjectMeta: metav1.ObjectMeta{Namespace: namespace, Name: "allow-from-same-ns-0"},
		Spec: istioapi.AuthorizationPolicy{
			Selector: &istioapitype.WorkloadSelector{MatchLabels: map[string]string{"app": "backend"}},
			Action:   istioapi.AuthorizationPolicy_ALLOW,
			Rules: []*istioapi.Rule{{
				From: []*istioapi.Rule_From{{
					Source: &istioapi.Source{Namespaces: []string{namespace}},
				}},
			}},
		},
	}

	client := &fakeClient{
		pods:                  map[string][]*corev1.Pod{namespace: {backendPod}},
		authorizationPolicies: map[string][]*istiosec.AuthorizationPolicy{namespace: {authzPolicy}},
		namespaces:            []string{namespace},
	}

	builder := NewBuilder(client, slog.Default())
	cache := &models.Cache{}
	if _, err := builder.PopulateCache(cache, []string{namespace}); err != nil {
		t.Fatalf("PopulateCache: %v", err)
	}

	backend := findWorkload(t, cache, namespace, "backend")
	got := backend.StatusesBySource["istio"]
	assertStatusKeys(t, got, models.StatusInternetEgress, models.StatusNamespaceIngress)
	assertNoStatusKey(t, got, models.StatusInternetFull)
}

func findWorkload(t *testing.T, cache *models.Cache, ns, label string) models.WorkloadNode {
	t.Helper()
	nsIndex, ok := cache.NsIndex[ns]
	if !ok {
		t.Fatalf("ns %q missing from cache.NsIndex", ns)
	}
	for _, workload := range nsIndex.Workloads {
		if workload.Type == models.NodeTypeNamespace {
			continue
		}
		if workload.Label == label {
			return workload
		}
	}
	t.Fatalf("workload %q not found in ns %q", label, ns)
	return models.WorkloadNode{}
}

func assertStatusKeys(t *testing.T, got []models.StatusKey, want ...models.StatusKey) {
	t.Helper()
	gotSet := map[models.StatusKey]bool{}
	for _, key := range got {
		gotSet[key] = true
	}
	for _, key := range want {
		if !gotSet[key] {
			t.Errorf("missing status key %q; got %v", key, got)
		}
	}
}

func assertNoStatusKey(t *testing.T, got []models.StatusKey, notWanted models.StatusKey) {
	t.Helper()
	for _, key := range got {
		if key == notWanted {
			t.Errorf("unexpected status key %q in %v", notWanted, got)
		}
	}
}
