package graph

import (
	networkingv1 "k8s.io/api/networking/v1"
	"graph/internal/k8s"
)

type Builder struct {
	client *k8s.Client
}

func NewBuilder(client *k8s.Client) *Builder {
	return &Builder{client: client}
}

// front end will call this function and list of ns are passed as params
func (b *Builder) BuildGraph(namespaces []string) (Graph, error) {
	// gather all workloads
	nodesByNS := map[string][]WorkloadNode{}
	for _, ns := range namespaces {
		nodes, err := b.buildWorkloadNodesForNS(ns)
		if err != nil {
			return Graph{}, err
		}
		nodesByNS[ns] = nodes
	}

	// get all k8s policies
	policies, err := b.fetchPolicies(namespaces)
	if err != nil {
		return Graph{}, err
	}

	edges := buildEdges(nodesByNS, policies)

	var allNodes []WorkloadNode
	for _, nsNodes := range nodesByNS {
		allNodes = append(allNodes, nsNodes...)
	}

	return Graph{Nodes: allNodes, Edges: edges}, nil
}

func (b *Builder) fetchPolicies(namespaces []string) ([]networkingv1.NetworkPolicy, error) {
	var policies []networkingv1.NetworkPolicy
	for _, ns := range namespaces {
		nsPolicies, err := b.client.GetPolicies(ns)
		if err != nil {
			return nil, err
		}
		policies = append(policies, nsPolicies...)
	}
	return policies, nil
}




// getStatusKeys computes status badge keys for a workload node given the full policy set.
func getStatusKeys(node WorkloadNode, policies []networkingv1.NetworkPolicy) []StatusKey {
	// TODO
	return nil
}
