package api

import (
	"fmt"
	"net/http"

	"github.com/labstack/echo/v4"
	 metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/yaml"
)

// PolicyManifest is the on-the-wire envelope for one policy object.
// YAML carries the cleaned cluster object, fetched on demand.
type PolicyManifest struct {
	Kind      string `json:"kind"`
	Namespace string `json:"namespace"`
	Name      string `json:"name"`
	YAML      string `json:"yaml"`
}

// Kinds whose objects live outside a namespace, so the namespace query param is
// absent rather than missing. Keyed by the frontend kind (PolicySource name).
var clusterScopedKinds = map[string]bool{"calico": true}

// get yaml for polices from api server
func (s *Server) getPolicyManifest(c echo.Context) error {
	ns := c.QueryParam("namespace")
	name := c.QueryParam("name")
	kind := c.QueryParam("kind")

	// Cluster-scoped kinds carry no namespace — only name+kind are required.
	if name == "" || kind == "" || (ns == "" && !clusterScopedKinds[kind]) {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "missing params"})
	}

	obj, kindName, err := s.fetchPolicyObject(kind, ns, name)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
	}

	manifestYAML, err := toCleanYAML(obj)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}

	return c.JSON(http.StatusOK, PolicyManifest{
		Kind:      kindName,
		Namespace: ns,
		Name:      name,
		YAML:      manifestYAML,
	})
}

// fetchPolicyObject dispatches on kind to the right single-object getter and
// stamps the GroupVersionKind (listers return objects with empty TypeMeta).
func (s *Server) fetchPolicyObject(kind, ns, name string) (runtime.Object, string, error) {
	switch kind {
	case "k8s":
		networkPolicy, err := s.client.GetK8sPolicyByName(ns, name)
		if err != nil {
			return nil, "", err
		}
		networkPolicy.GetObjectKind().SetGroupVersionKind(schema.GroupVersionKind{
			Group: "networking.k8s.io", Version: "v1", Kind: "NetworkPolicy",
		})
		return networkPolicy, "NetworkPolicy", nil
	case "istio":
		authzPolicy, err := s.client.GetAuthorizationPoliciesByName(ns, name)
		if err != nil {
			return nil, "", err
		}
		if authzPolicy == nil {
			return nil, "", fmt.Errorf("istio security CRDs not installed")
		}
		authzPolicy.GetObjectKind().SetGroupVersionKind(schema.GroupVersionKind{
			Group: "security.istio.io", Version: "v1", Kind: "AuthorizationPolicy",
		})
		return authzPolicy, "AuthorizationPolicy", nil
	case "pa":
		peerAuth, err := s.client.GetPeerAuthenticationsByName(ns, name)
		if err != nil {
			return nil, "", err
		}
		if peerAuth == nil {
			return nil, "", fmt.Errorf("istio security CRDs not installed")
		}
		peerAuth.GetObjectKind().SetGroupVersionKind(schema.GroupVersionKind{
			Group: "security.istio.io", Version: "v1", Kind: "PeerAuthentication",
		})
		return peerAuth, "PeerAuthentication", nil
	case "calico":
		globalPolicy, err := s.client.GetGlobalNetworkPolicyByName(name)
		if err != nil {
			return nil, "", err
		}
		if globalPolicy == nil {
			return nil, "", fmt.Errorf("calico CRDs not installed")
		}
		globalPolicy.GetObjectKind().SetGroupVersionKind(schema.GroupVersionKind{
			Group: "projectcalico.org", Version: "v3", Kind: "GlobalNetworkPolicy",
		})
		return globalPolicy, "GlobalNetworkPolicy", nil
	default:
		return nil, "", fmt.Errorf("unknown kind %q", kind)
	}
}

// toCleanYAML strips server-side noise so output matches kubectl get -o yaml.
func toCleanYAML(obj runtime.Object) (string, error) {
	if meta, ok := obj.(metav1.Object); ok {
		meta.SetManagedFields(nil)
		annotations := meta.GetAnnotations()
		delete(annotations, "kubectl.kubernetes.io/last-applied-configuration") 
		meta.SetAnnotations(annotations)
		// meta.SetResourceVersion("")
		// meta.SetUID("")
		// meta.SetGeneration(0)
		// meta.SetCreationTimestamp(metav1.Time{})
	}
	data, err := yaml.Marshal(obj)
	if err != nil {
		return "", err
	}
	return string(data), nil
}
