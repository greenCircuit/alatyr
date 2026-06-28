package k8s

import (
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"

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

	// single-object fetch by name, for on-demand manifest view
	GetK8sPolicyByName(ns string, name string) (*networkingv1.NetworkPolicy, error)
	GetAuthorizationPoliciesByName(ns string, name string) (*istiosec.AuthorizationPolicy, error)
	GetPeerAuthenticationsByName(ns string, name string) (*istiosec.PeerAuthentication, error)
}
