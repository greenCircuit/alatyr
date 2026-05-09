package k8s

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"strings"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	k8syaml "k8s.io/apimachinery/pkg/util/yaml"
)

type DemoClient struct {
	namespaces map[string]corev1.Namespace
	pods       map[string][]corev1.Pod
	services   map[string][]corev1.Service
	policies   map[string][]networkingv1.NetworkPolicy
}

func NewDemoClient(dataFS fs.FS, dir string) (*DemoClient, error) {
	c := &DemoClient{
		namespaces: map[string]corev1.Namespace{},
		pods:       map[string][]corev1.Pod{},
		services:   map[string][]corev1.Service{},
		policies:   map[string][]networkingv1.NetworkPolicy{},
	}

	entries, err := fs.ReadDir(dataFS, dir)
	if err != nil {
		return nil, fmt.Errorf("reading demo data dir: %w", err)
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".yaml") {
			continue
		}
		data, err := fs.ReadFile(dataFS, dir+"/"+entry.Name())
		if err != nil {
			return nil, fmt.Errorf("reading %s: %w", entry.Name(), err)
		}
		if err := c.parseFile(data); err != nil {
			return nil, fmt.Errorf("parsing %s: %w", entry.Name(), err)
		}
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
			var ns corev1.Namespace
			if err := json.Unmarshal(jsonBytes, &ns); err != nil {
				return err
			}
			c.namespaces[ns.Name] = ns

		case "Deployment":
			var d appsv1.Deployment
			if err := json.Unmarshal(jsonBytes, &d); err != nil {
				return err
			}
			c.pods[d.Namespace] = append(c.pods[d.Namespace], deploymentToPod(d))

		case "NetworkPolicy":
			var np networkingv1.NetworkPolicy
			if err := json.Unmarshal(jsonBytes, &np); err != nil {
				return err
			}
			c.policies[np.Namespace] = append(c.policies[np.Namespace], np)
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

// deploymentToPod synthesizes a representative pod from a Deployment.
// Uses a deterministic UID so that multiple pods from the same Deployment
// collapse into a single workload node via ownerUID deduplication.
func deploymentToPod(d appsv1.Deployment) corev1.Pod {
	return corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      d.Name + "-demo-pod",
			Namespace: d.Namespace,
			Labels:    d.Spec.Template.Labels,
			OwnerReferences: []metav1.OwnerReference{{
				Kind: "ReplicaSet",
				Name: d.Name + "-demo-rs",
				UID:  types.UID("demo-rs-" + d.Namespace + "-" + d.Name),
			}},
		},
	}
}

func (c *DemoClient) GetPods(ns string) ([]corev1.Pod, error) {
	return c.pods[ns], nil
}

func (c *DemoClient) GetSvc(ns string) ([]corev1.Service, error) {
	return c.services[ns], nil
}

func (c *DemoClient) GetPolicies(ns string) ([]networkingv1.NetworkPolicy, error) {
	return c.policies[ns], nil
}

func (c *DemoClient) GetNsNames() ([]string, error) {
	names := make([]string, 0, len(c.namespaces))
	for name := range c.namespaces {
		names = append(names, name)
	}
	return names, nil
}

func (c *DemoClient) GetNs(ns string) (corev1.Namespace, error) {
	obj, ok := c.namespaces[ns]
	if !ok {
		return corev1.Namespace{}, fmt.Errorf("namespace %q not found in demo data", ns)
	}
	return obj, nil
}
