package graph

import (
	"sync"

	"graph/internal/k8s"
	"graph/internal/models"
	"graph/internal/policy/k8spolicy"

	networkingv1 "k8s.io/api/networking/v1"
)

type Builder struct {
	client k8s.KubernetesClient
}

func NewBuilder(client k8s.KubernetesClient) *Builder {
	return &Builder{client: client}
}

// front end will call this function and list of ns are passed as params
func (b *Builder) BuildGraph(namespaces []string) (Graph, error) {
	labelIndexByNS := map[string]map[string][]*models.WorkloadNode{}
	policiesByNS := map[string][]networkingv1.NetworkPolicy{}
	var allNodes []models.WorkloadNode
	var mu sync.Mutex
	var firstErr error
	var wg sync.WaitGroup

	for _, ns := range namespaces {
		wg.Add(1)
		go func(ns string) {
			defer wg.Done()

			nodes, err := b.buildWorkloadNodesForNS(ns)
			if err != nil {
				mu.Lock()
				if firstErr == nil {
					firstErr = err
				}
				mu.Unlock()
				return
			}
			nsPolicies, err := b.client.GetPolicies(ns)
			if err != nil {
				mu.Lock()
				if firstErr == nil {
					firstErr = err
				}
				mu.Unlock()
				return
			}

			for position := range nodes {
				matchPolicies := getNodePolicies(nodes[position], nsPolicies)
				nodes[position].Statuses = k8spolicy.BuildStatusKeys(matchPolicies)
			}

			// build index outside the lock — read-only on local nodes slice
			// should not modify nodes after this
			labelIndex := buildWorkloadIndex(nodes)

			mu.Lock()
			labelIndexByNS[ns] = labelIndex
			policiesByNS[ns] = nsPolicies
			allNodes = append(allNodes, nodes...)
			mu.Unlock()
		}(ns)
	}

	wg.Wait()

	if firstErr != nil {
		return Graph{}, firstErr
	}

	var allPolicies []networkingv1.NetworkPolicy
	for _, ns := range namespaces {
		allPolicies = append(allPolicies, policiesByNS[ns]...)
	}

	tuples := k8spolicy.BuildAllowTuples(labelIndexByNS, allPolicies)
	edges := foldTuplesToEdges(tuples, allNodes)

	return Graph{Nodes: allNodes, Edges: edges}, nil
}
