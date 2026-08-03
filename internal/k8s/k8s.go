package k8s

import (
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"

	calicov3 "github.com/projectcalico/api/pkg/apis/projectcalico/v3"
	istiosec "istio.io/client-go/pkg/apis/security/v1"
)

type KubernetesClient interface {
	GetPods(ns string) ([]*corev1.Pod, error)
	GetCronJobs(ns string) ([]*batchv1.CronJob, error)
	GetPolicies(ns string) ([]*networkingv1.NetworkPolicy, error)
	GetNsNames() ([]string, error)
	GetNs(ns string) (*corev1.Namespace, error)
	GetAuthorizationPolicies(ns string) ([]*istiosec.AuthorizationPolicy, error)
	GetPeerAuthentications(ns string) ([]*istiosec.PeerAuthentication, error)

	// GlobalNetworkPolicy is cluster-scoped — no namespace arg. Returns
	// (nil, nil) when the projectcalico.org/v3 API isn't served.
	GetGlobalNetworkPolicies() ([]*calicov3.GlobalNetworkPolicy, error)

	// single-object fetch by name, for on-demand manifest view
	GetK8sPolicyByName(ns string, name string) (*networkingv1.NetworkPolicy, error)
	GetAuthorizationPoliciesByName(ns string, name string) (*istiosec.AuthorizationPolicy, error)
	GetPeerAuthenticationsByName(ns string, name string) (*istiosec.PeerAuthentication, error)
	// Cluster-scoped, so name only. (nil, nil) when Calico isn't installed.
	GetGlobalNetworkPolicyByName(name string) (*calicov3.GlobalNetworkPolicy, error)
}
