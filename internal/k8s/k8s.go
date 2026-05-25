package k8s

import (
	"context"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"

	istiosec "istio.io/client-go/pkg/apis/security/v1"
)

type KubernetesClient interface {
	GetPods(ns string) ([]corev1.Pod, error)
	GetCronJobs(ns string) ([]batchv1.CronJob, error)
	GetSvc(ns string) ([]corev1.Service, error)
	GetPolicies(ns string) ([]networkingv1.NetworkPolicy, error)
	GetNsNames() ([]string, error)
	GetNs(ns string) (corev1.Namespace, error)

	// GetAuthorizationPolicies fetches Istio security.istio.io/v1
	// AuthorizationPolicy objects in the namespace. Real client uses
	// istio.io/client-go (versioned clientset).
	GetAuthorizationPolicies(ns string) ([]*istiosec.AuthorizationPolicy, error)
}

type Client struct {
	clientset *kubernetes.Clientset
}

func New(kubeconfigPath string) (*Client, error) {
	config, err := clientcmd.BuildConfigFromFlags("", kubeconfigPath)
	if err != nil {
		return nil, err
	}
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, err
	}
	return &Client{clientset: clientset}, nil
}

func (c *Client) GetPods(ns string) ([]corev1.Pod, error) {
	list, err := c.clientset.CoreV1().Pods(ns).List(context.Background(), metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	return list.Items, nil
}


func (c *Client) GetCronJobs(ns string) ([]batchv1.CronJob, error) {
	list, err := c.clientset.BatchV1().CronJobs(ns).List(context.Background(), metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	return list.Items, nil
}

func (c *Client) GetSvc(ns string) ([]corev1.Service, error) {
	list, err := c.clientset.CoreV1().Services(ns).List(context.Background(), metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	return list.Items, nil
}

func (c *Client) GetPolicies(ns string) ([]networkingv1.NetworkPolicy, error) {
	list, err := c.clientset.NetworkingV1().NetworkPolicies(ns).List(context.Background(), metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	return list.Items, nil
}


func (c *Client) GetNsNames() ([]string, error) {
	list, err := c.clientset.CoreV1().Namespaces().List(context.Background(), metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	var names []string
	for _, ns := range list.Items {
		names = append(names, ns.Name)
	}
	return names, nil
}

// get single ns object so can can convert it to node for object
func (c *Client) GetNs(ns string) (corev1.Namespace, error) {
	obj, err := c.clientset.CoreV1().Namespaces().Get(context.Background(), ns, metav1.GetOptions{})
	if err != nil {
		return corev1.Namespace{}, err
	}
	return *obj, nil
}

// GetAuthorizationPolicies fetches Istio AuthorizationPolicies for a namespace.
// TODO: wire up istio.io/client-go versioned clientset (separate from
// kubernetes.Clientset) and list .SecurityV1().AuthorizationPolicies(ns).
// Returning nil for now means the Istio engine sees no policies in
// real-cluster mode until the engine is wired up.
func (c *Client) GetAuthorizationPolicies(ns string) ([]*istiosec.AuthorizationPolicy, error) {
	return nil, nil
}
