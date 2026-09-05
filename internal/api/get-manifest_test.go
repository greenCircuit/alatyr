package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"alatyr/internal/k8s"

	"github.com/labstack/echo/v4"
	calicov3 "github.com/projectcalico/api/pkg/apis/projectcalico/v3"
	istiosec "istio.io/client-go/pkg/apis/security/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// fakeManifestClient satisfies k8s.KubernetesClient via the embedded interface
// (unused methods are never called by getPolicyManifest). Only the k8s
// single-object getter is seeded.
type fakeManifestClient struct {
	k8s.KubernetesClient
	netpol       *networkingv1.NetworkPolicy
	globalPolicy *calicov3.GlobalNetworkPolicy
}

func (f *fakeManifestClient) GetK8sPolicyByName(ns, name string) (*networkingv1.NetworkPolicy, error) {
	return f.netpol, nil
}
func (f *fakeManifestClient) GetAuthorizationPoliciesByName(ns, name string) (*istiosec.AuthorizationPolicy, error) {
	return nil, nil
}
func (f *fakeManifestClient) GetPeerAuthenticationsByName(ns, name string) (*istiosec.PeerAuthentication, error) {
	return nil, nil
}
func (f *fakeManifestClient) GetGlobalNetworkPolicyByName(name string) (*calicov3.GlobalNetworkPolicy, error) {
	return f.globalPolicy, nil
}

func doManifestRequest(t *testing.T, client *fakeManifestClient, query string) *httptest.ResponseRecorder {
	t.Helper()
	server := New(client, nil, nil)
	echoServer := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/api/manifest?"+query, nil)
	rec := httptest.NewRecorder()
	if err := server.getPolicyManifest(echoServer.NewContext(req, rec)); err != nil {
		t.Fatalf("handler returned error: %v", err)
	}
	return rec
}

func TestGetPolicyManifest_K8sCleanHappyPath(t *testing.T) {
	netpol := &networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:          "deny-all",
			Namespace:     "shop",
			ManagedFields: []metav1.ManagedFieldsEntry{{Manager: "kubectl"}},
			Annotations: map[string]string{
				"kubectl.kubernetes.io/last-applied-configuration": "{noise}",
				"keep-me": "yes",
			},
		},
	}

	rec := doManifestRequest(t, &fakeManifestClient{netpol: netpol}, "kind=k8s&namespace=shop&name=deny-all")

	if rec.Code != http.StatusOK {
		t.Fatalf("status: want 200, got %d (%s)", rec.Code, rec.Body.String())
	}
	var got PolicyManifest
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Kind != "NetworkPolicy" {
		t.Errorf("kind: want NetworkPolicy, got %q", got.Kind)
	}
	// GVK stamped before marshal so the YAML reads like kubectl get -o yaml.
	if !strings.Contains(got.YAML, "kind: NetworkPolicy") {
		t.Errorf("yaml missing stamped GVK:\n%s", got.YAML)
	}
	// Server-side noise stripped; unrelated annotation preserved.
	if strings.Contains(got.YAML, "managedFields") {
		t.Error("managedFields not stripped from manifest")
	}
	if strings.Contains(got.YAML, "last-applied-configuration") {
		t.Error("last-applied annotation not stripped")
	}
	if !strings.Contains(got.YAML, "keep-me") {
		t.Error("unrelated annotation wrongly stripped")
	}
}

func TestGetPolicyManifest_MissingParam(t *testing.T) {
	// namespace omitted on a namespaced kind → 400, no client call.
	rec := doManifestRequest(t, &fakeManifestClient{}, "kind=k8s&name=deny-all")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status: want 400 for missing namespace, got %d", rec.Code)
	}
	// name omitted → 400 even for a cluster-scoped kind.
	rec = doManifestRequest(t, &fakeManifestClient{}, "kind=calico")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status: want 400 for missing name, got %d", rec.Code)
	}
}

func TestGetPolicyManifest_UnknownKind(t *testing.T) {
	rec := doManifestRequest(t, &fakeManifestClient{}, "kind=bogus&namespace=shop&name=x")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status: want 400 for unknown kind, got %d", rec.Code)
	}
}

// Cluster-scoped kind: no namespace param, GVK stamped, empty namespace echoed.
func TestGetPolicyManifest_CalicoClusterScoped(t *testing.T) {
	globalPolicy := &calicov3.GlobalNetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:          "egress-default-deny",
			ManagedFields: []metav1.ManagedFieldsEntry{{Manager: "kubectl"}},
		},
	}

	rec := doManifestRequest(t, &fakeManifestClient{globalPolicy: globalPolicy}, "kind=calico&name=egress-default-deny")

	if rec.Code != http.StatusOK {
		t.Fatalf("status: want 200, got %d (%s)", rec.Code, rec.Body.String())
	}
	var got PolicyManifest
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Kind != "GlobalNetworkPolicy" || got.Namespace != "" {
		t.Errorf("kind/ns: want GlobalNetworkPolicy + empty ns, got %q / %q", got.Kind, got.Namespace)
	}
	if !strings.Contains(got.YAML, "kind: GlobalNetworkPolicy") {
		t.Errorf("yaml missing stamped GVK:\n%s", got.YAML)
	}
	if strings.Contains(got.YAML, "managedFields") {
		t.Error("managedFields not stripped from manifest")
	}
}

// Calico absent from the cluster → nil object, surfaced as 400 not a nil panic.
func TestGetPolicyManifest_CalicoNotInstalled(t *testing.T) {
	rec := doManifestRequest(t, &fakeManifestClient{}, "kind=calico&name=whatever")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status: want 400 when calico CRDs absent, got %d", rec.Code)
	}
}
