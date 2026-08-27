package k8s

import (
	"fmt"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	batchlisters "k8s.io/client-go/listers/batch/v1"
	corelisters "k8s.io/client-go/listers/core/v1"
	netlisters "k8s.io/client-go/listers/networking/v1"
	"k8s.io/client-go/tools/clientcmd"

	istiosec "istio.io/client-go/pkg/apis/security/v1"
	istioclient "istio.io/client-go/pkg/clientset/versioned"
	istioinformers "istio.io/client-go/pkg/informers/externalversions"
	istiolisters "istio.io/client-go/pkg/listers/security/v1"

	calicov3 "github.com/projectcalico/api/pkg/apis/projectcalico/v3"
	calicoclient "github.com/projectcalico/api/pkg/client/clientset_generated/clientset"
	calicoinformers "github.com/projectcalico/api/pkg/client/informers_generated/externalversions"
	calicolisters "github.com/projectcalico/api/pkg/client/listers_generated/projectcalico/v3"
)

// InformerClient backs the KubernetesClient surface with shared informer
// caches instead of per-request List calls. One cluster-scoped LIST + WATCH
// per GVK at startup; every Get* call after that is an in-memory indexer
// lookup. apLister/paLister stay nil when the Istio security CRDs aren't
// installed — methods return (nil, nil) in that case, matching the existing
// IsNoMatch tolerance.
type InformerClient struct {
	coreFactory   informers.SharedInformerFactory
	istioFactory  istioinformers.SharedInformerFactory
	calicoFactory calicoinformers.SharedInformerFactory

	podLister corelisters.PodLister
	cjLister  batchlisters.CronJobLister
	npLister  netlisters.NetworkPolicyLister
	nsLister  corelisters.NamespaceLister

	apLister istiolisters.AuthorizationPolicyLister // nil if security.istio.io/v1 absent
	paLister istiolisters.PeerAuthenticationLister  // nil if security.istio.io/v1 absent

	gnpLister calicolisters.GlobalNetworkPolicyLister // nil if projectcalico.org/v3 absent

	// syncedResources is the closed set of GVK strings whose informer cache
	// completed sync during NewInformerClient. Populated once at startup;
	// exposed via SyncedResources so the metrics recorder can stamp
	// alatyr_informer_cache_synced without importing client-go internals.
	syncedResources []string
}

// NewInformerClient builds clientsets, starts shared informer factories,
// blocks until every registered informer's cache has synced, then returns
// a client wired to the listers. Caller owns stopCh; closing it stops all
// informers. Istio CRDs are probed via discovery — when absent, the istio
// factory is skipped entirely (WaitForCacheSync on a missing GVK hangs).
func NewInformerClient(kubeconfigPath string, stopCh <-chan struct{}) (*InformerClient, error) {
	config, err := clientcmd.BuildConfigFromFlags("", kubeconfigPath)
	if err != nil {
		return nil, err
	}
	coreCS, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, err
	}
	istioCS, err := istioclient.NewForConfig(config)
	if err != nil {
		return nil, err
	}
	calicoCS, err := calicoclient.NewForConfig(config)
	if err != nil {
		return nil, err
	}

	coreFactory := informers.NewSharedInformerFactory(coreCS, 0)
	podInf := coreFactory.Core().V1().Pods()
	cjInf := coreFactory.Batch().V1().CronJobs()
	npInf := coreFactory.Networking().V1().NetworkPolicies()
	nsInf := coreFactory.Core().V1().Namespaces()

	c := &InformerClient{
		coreFactory: coreFactory,
		podLister:   podInf.Lister(),
		cjLister:    cjInf.Lister(),
		npLister:    npInf.Lister(),
		nsLister:    nsInf.Lister(),
	}

	istioPresent, err := istioSecurityCRDPresent(coreCS)
	if err != nil {
		return nil, fmt.Errorf("probe istio security CRDs: %w", err)
	}
	if istioPresent {
		istioFactory := istioinformers.NewSharedInformerFactory(istioCS, 0)
		apInf := istioFactory.Security().V1().AuthorizationPolicies()
		paInf := istioFactory.Security().V1().PeerAuthentications()
		c.istioFactory = istioFactory
		c.apLister = apInf.Lister()
		c.paLister = paInf.Lister()
	}

	calicoPresent, err := calicoCRDPresent(coreCS)
	if err != nil {
		return nil, fmt.Errorf("probe calico CRDs: %w", err)
	}
	if calicoPresent {
		calicoFactory := calicoinformers.NewSharedInformerFactory(calicoCS, 0)
		gnpInf := calicoFactory.Projectcalico().V3().GlobalNetworkPolicies()
		c.calicoFactory = calicoFactory
		c.gnpLister = gnpInf.Lister()
	}

	coreFactory.Start(stopCh)
	if c.istioFactory != nil {
		c.istioFactory.Start(stopCh)
	}
	if c.calicoFactory != nil {
		c.calicoFactory.Start(stopCh)
	}

	for typ, ok := range coreFactory.WaitForCacheSync(stopCh) {
		if !ok {
			return nil, fmt.Errorf("core informer failed to sync: %v", typ)
		}
		c.syncedResources = append(c.syncedResources, typ.String())
	}
	if c.istioFactory != nil {
		for typ, ok := range c.istioFactory.WaitForCacheSync(stopCh) {
			if !ok {
				return nil, fmt.Errorf("istio informer failed to sync: %v", typ)
			}
			c.syncedResources = append(c.syncedResources, typ.String())
		}
	}
	if c.calicoFactory != nil {
		for typ, ok := range c.calicoFactory.WaitForCacheSync(stopCh) {
			if !ok {
				return nil, fmt.Errorf("calico informer failed to sync: %v", typ)
			}
			c.syncedResources = append(c.syncedResources, typ.String())
		}
	}

	return c, nil
}

// SyncedResources returns the informer GVK strings that completed sync at
// startup. NewInformerClient returns an error if any informer fails to sync,
// so a non-nil client always has every listed resource marked synced —
// callers still emit per-resource gauges so a future health probe can flip
// individual entries to 0.
func (c *InformerClient) SyncedResources() []string {
	out := make([]string, len(c.syncedResources))
	copy(out, c.syncedResources)
	return out
}

// calicoCRDPresent probes discovery for projectcalico.org/v3 (the aggregated
// Calico API the clientset targets). Absent on clusters without Calico's API
// server — skip the factory so WaitForCacheSync doesn't block on a missing GVK.
func calicoCRDPresent(cs *kubernetes.Clientset) (bool, error) {
	_, err := cs.Discovery().ServerResourcesForGroupVersion("projectcalico.org/v3")
	if err == nil {
		return true, nil
	}
	if errors.IsNotFound(err) {
		return false, nil
	}
	return false, err
}

// istioSecurityCRDPresent probes discovery for security.istio.io/v1. WaitForCacheSync
// blocks forever on a missing GVK; checking up front lets the rest of the app
// run on clusters without Istio installed.
func istioSecurityCRDPresent(cs *kubernetes.Clientset) (bool, error) {
	_, err := cs.Discovery().ServerResourcesForGroupVersion("security.istio.io/v1")
	if err == nil {
		return true, nil
	}
	if errors.IsNotFound(err) {
		return false, nil
	}
	return false, err
}

func (c *InformerClient) GetPods(ns string) ([]*corev1.Pod, error) {
	return c.podLister.Pods(ns).List(labels.Everything())
}

func (c *InformerClient) GetCronJobs(ns string) ([]*batchv1.CronJob, error) {
	return c.cjLister.CronJobs(ns).List(labels.Everything())
}

func (c *InformerClient) GetPolicies(ns string) ([]*networkingv1.NetworkPolicy, error) {
	return c.npLister.NetworkPolicies(ns).List(labels.Everything())
}

func (c *InformerClient) GetK8sPolicyByName(ns string, name string) (*networkingv1.NetworkPolicy, error) {
	return c.npLister.NetworkPolicies(ns).Get(name)
}

func (c *InformerClient) GetNs(ns string) (*corev1.Namespace, error) {
	return c.nsLister.Get(ns)
}

func (c *InformerClient) GetNsNames() ([]string, error) {
	items, err := c.nsLister.List(labels.Everything())
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(items))
	for _, n := range items {
		names = append(names, n.Name)
	}
	return names, nil
}

// GetAuthorizationPolicies returns (nil, nil) when security.istio.io/v1 is not
// installed — same shape callers already tolerate via IsNoMatch on the live
// client.
func (c *InformerClient) GetAuthorizationPolicies(ns string) ([]*istiosec.AuthorizationPolicy, error) {
	if c.apLister == nil {
		return nil, nil
	}
	return c.apLister.AuthorizationPolicies(ns).List(labels.Everything())
}

func (c *InformerClient) GetPeerAuthentications(ns string) ([]*istiosec.PeerAuthentication, error) {
	if c.paLister == nil {
		return nil, nil
	}
	return c.paLister.PeerAuthentications(ns).List(labels.Everything())
}


func (c *InformerClient) GetAuthorizationPoliciesByName(ns string, name string) (*istiosec.AuthorizationPolicy, error) {
	if c.apLister == nil {
		return nil, nil
	}
	return c.apLister.AuthorizationPolicies(ns).Get(name)
}

func (c *InformerClient) GetPeerAuthenticationsByName(ns string, name string) (*istiosec.PeerAuthentication, error) {
	if c.paLister == nil {
		return nil, nil
	}
	return c.paLister.PeerAuthentications(ns).Get(name)
}

// GetGlobalNetworkPolicies returns (nil, nil) when projectcalico.org/v3 isn't
// served — clusters without Calico. Cluster-scoped: no namespace filter.
func (c *InformerClient) GetGlobalNetworkPolicies() ([]*calicov3.GlobalNetworkPolicy, error) {
	if c.gnpLister == nil {
		return nil, nil
	}
	return c.gnpLister.List(labels.Everything())
}

func (c *InformerClient) GetGlobalNetworkPolicyByName(name string) (*calicov3.GlobalNetworkPolicy, error) {
	if c.gnpLister == nil {
		return nil, nil
	}
	return c.gnpLister.Get(name)
}