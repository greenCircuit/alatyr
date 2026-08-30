package k8s

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"strings"

	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	k8syaml "k8s.io/apimachinery/pkg/util/yaml"

	calicov3 "github.com/projectcalico/api/pkg/apis/projectcalico/v3"
	istiosec "istio.io/client-go/pkg/apis/security/v1"
)

type DemoClient struct {
	namespaces            map[string]*corev1.Namespace
	pods                  map[string][]*corev1.Pod
	deployments           map[string][]*appsv1.Deployment
	statefulSets          map[string][]*appsv1.StatefulSet
	daemonSets            map[string][]*appsv1.DaemonSet
	jobs                  map[string][]*batchv1.Job
	policies              map[string][]*networkingv1.NetworkPolicy
	authorizationPolicies map[string][]*istiosec.AuthorizationPolicy
	peerAuthentications   map[string][]*istiosec.PeerAuthentication
	globalNetworkPolicies []*calicov3.GlobalNetworkPolicy // cluster-scoped, no ns key
}

func NewDemoClient(dataFS fs.FS, dir string) (*DemoClient, error) {
	c := &DemoClient{
		namespaces:            map[string]*corev1.Namespace{},
		pods:                  map[string][]*corev1.Pod{},
		deployments:           map[string][]*appsv1.Deployment{},
		statefulSets:          map[string][]*appsv1.StatefulSet{},
		daemonSets:            map[string][]*appsv1.DaemonSet{},
		jobs:                  map[string][]*batchv1.Job{},
		policies:              map[string][]*networkingv1.NetworkPolicy{},
		authorizationPolicies: map[string][]*istiosec.AuthorizationPolicy{},
		peerAuthentications:   map[string][]*istiosec.PeerAuthentication{},
	}

	// Walk the tree so per-engine subdirs (test-data/k8sEngine,
	// test-data/istioEngine, ...) are all picked up.
	walkErr := fs.WalkDir(dataFS, dir, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		name := entry.Name()
		if entry.IsDir() || (!strings.HasSuffix(name, ".yaml") && !strings.HasSuffix(name, ".yml")) {
			return nil
		}
		data, readErr := fs.ReadFile(dataFS, path)
		if readErr != nil {
			return fmt.Errorf("reading %s: %w", path, readErr)
		}
		if parseErr := c.parseFile(data); parseErr != nil {
			return fmt.Errorf("parsing %s: %w", path, parseErr)
		}
		return nil
	})
	if walkErr != nil {
		return nil, fmt.Errorf("walking demo data tree: %w", walkErr)
	}
	return c, nil
}

func (c *DemoClient) parseFile(data []byte) error {
	for _, doc := range splitYAMLDocs(data) {
		jsonBytes, err := k8syaml.ToJSON(doc)
		if err != nil {
			continue
		}

		var tm metav1.TypeMeta
		if err := json.Unmarshal(jsonBytes, &tm); err != nil || tm.Kind == "" {
			continue
		}

		switch tm.Kind {
		case "Namespace":
			ns := &corev1.Namespace{}
			if err := json.Unmarshal(jsonBytes, ns); err != nil {
				return err
			}
			c.namespaces[ns.Name] = ns

		case "Deployment":
			deployment := &appsv1.Deployment{}
			if err := json.Unmarshal(jsonBytes, deployment); err != nil {
				return err
			}
			// fixtures rarely carry a UID; node IDs key on it, empty collides
			if deployment.UID == "" {
				deployment.UID = types.UID("demo-deployment-" + deployment.Namespace + "-" + deployment.Name)
			}
			c.deployments[deployment.Namespace] = append(c.deployments[deployment.Namespace], deployment)

		case "StatefulSet":
			statefulSet := &appsv1.StatefulSet{}
			if err := json.Unmarshal(jsonBytes, statefulSet); err != nil {
				return err
			}
			if statefulSet.UID == "" {
				statefulSet.UID = types.UID("demo-statefulset-" + statefulSet.Namespace + "-" + statefulSet.Name)
			}
			c.statefulSets[statefulSet.Namespace] = append(c.statefulSets[statefulSet.Namespace], statefulSet)

		case "DaemonSet":
			daemonSet := &appsv1.DaemonSet{}
			if err := json.Unmarshal(jsonBytes, daemonSet); err != nil {
				return err
			}
			if daemonSet.UID == "" {
				daemonSet.UID = types.UID("demo-daemonset-" + daemonSet.Namespace + "-" + daemonSet.Name)
			}
			c.daemonSets[daemonSet.Namespace] = append(c.daemonSets[daemonSet.Namespace], daemonSet)

		case "Job":
			job := &batchv1.Job{}
			if err := json.Unmarshal(jsonBytes, job); err != nil {
				return err
			}
			if job.UID == "" {
				job.UID = types.UID("demo-job-" + job.Namespace + "-" + job.Name)
			}
			c.jobs[job.Namespace] = append(c.jobs[job.Namespace], job)

		case "Pod":
			var pod *corev1.Pod
			if err := json.Unmarshal(jsonBytes, &pod); err != nil {
				return err
			}
			c.pods[pod.Namespace] = append(c.pods[pod.Namespace], pod)

		case "NetworkPolicy":
			var np *networkingv1.NetworkPolicy
			if err := json.Unmarshal(jsonBytes, &np); err != nil {
				return err
			}
			c.policies[np.Namespace] = append(c.policies[np.Namespace], np)

		case "AuthorizationPolicy":
			authzPolicy := &istiosec.AuthorizationPolicy{}
			if err := json.Unmarshal(jsonBytes, authzPolicy); err != nil {
				return err
			}
			c.authorizationPolicies[authzPolicy.Namespace] = append(c.authorizationPolicies[authzPolicy.Namespace], authzPolicy)

		case "PeerAuthentication":
			peerAuth := &istiosec.PeerAuthentication{}
			if err := json.Unmarshal(jsonBytes, peerAuth); err != nil {
				return err
			}
			c.peerAuthentications[peerAuth.Namespace] = append(c.peerAuthentications[peerAuth.Namespace], peerAuth)

		case "GlobalNetworkPolicy":
			gnp := &calicov3.GlobalNetworkPolicy{}
			if err := json.Unmarshal(jsonBytes, gnp); err != nil {
				return err
			}
			c.globalNetworkPolicies = append(c.globalNetworkPolicies, gnp)
		}
	}
	return nil
}

// splitYAMLDocs splits a multi-document YAML file on --- separators.
func splitYAMLDocs(data []byte) [][]byte {
	content := string(data)
	if strings.HasPrefix(content, "---") {
		content = "\n" + content
	}
	var docs [][]byte
	for _, part := range strings.Split(content, "\n---") {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			docs = append(docs, []byte(trimmed))
		}
	}
	return docs
}

func (c *DemoClient) GetPods(ns string) ([]*corev1.Pod, error) {
	return c.pods[ns], nil
}

func (c *DemoClient) GetCronJobs(ns string) ([]*batchv1.CronJob, error) {
	return nil, nil
}

func (c *DemoClient) GetJobs(ns string) ([]*batchv1.Job, error) {
	return c.jobs[ns], nil
}

func (c *DemoClient) GetDeployments(ns string) ([]*appsv1.Deployment, error) {
	return c.deployments[ns], nil
}

func (c *DemoClient) GetStatefulSets(ns string) ([]*appsv1.StatefulSet, error) {
	return c.statefulSets[ns], nil
}

func (c *DemoClient) GetDaemonSets(ns string) ([]*appsv1.DaemonSet, error) {
	return c.daemonSets[ns], nil
}

func (c *DemoClient) GetPolicies(ns string) ([]*networkingv1.NetworkPolicy, error) {
	return c.policies[ns], nil
}

func (c *DemoClient) GetNsNames() ([]string, error) {
	names := make([]string, 0, len(c.namespaces))
	for name := range c.namespaces {
		names = append(names, name)
	}
	return names, nil
}

func (c *DemoClient) GetNs(ns string) (*corev1.Namespace, error) {
	obj, ok := c.namespaces[ns]
	if !ok {
		return nil, fmt.Errorf("namespace %q not found in demo data", ns)
	}
	return obj, nil
}

func (c *DemoClient) GetAuthorizationPolicies(ns string) ([]*istiosec.AuthorizationPolicy, error) {
	return c.authorizationPolicies[ns], nil
}

func (c *DemoClient) GetPeerAuthentications(ns string) ([]*istiosec.PeerAuthentication, error) {
	return c.peerAuthentications[ns], nil
}

func (c *DemoClient) GetGlobalNetworkPolicies() ([]*calicov3.GlobalNetworkPolicy, error) {
	return c.globalNetworkPolicies, nil
}

func (c *DemoClient) GetGlobalNetworkPolicyByName(name string) (*calicov3.GlobalNetworkPolicy, error) {
	return findByName(c.globalNetworkPolicies, name)
}

func (c *DemoClient) GetK8sPolicyByName(ns string, name string) (*networkingv1.NetworkPolicy, error) {
	return findByName(c.policies[ns], name)
}

func (c *DemoClient) GetAuthorizationPoliciesByName(ns string, name string) (*istiosec.AuthorizationPolicy, error) {
	return findByName(c.authorizationPolicies[ns], name)
}

func (c *DemoClient) GetPeerAuthenticationsByName(ns string, name string) (*istiosec.PeerAuthentication, error) {
	return findByName(c.peerAuthentications[ns], name)
}

// findByName returns the item whose metadata name matches, or a not-found error.
func findByName[T interface{ GetName() string }](items []T, name string) (T, error) {
	for _, item := range items {
		if item.GetName() == name {
			return item, nil
		}
	}
	var zero T
	return zero, fmt.Errorf("not found: %q", name)
}
