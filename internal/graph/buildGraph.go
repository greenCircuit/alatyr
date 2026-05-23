package graph

import (
	"context"
	"sync"

	"graph/internal/k8s"
	"graph/internal/models"
	"graph/internal/policy"
	"graph/internal/policy/k8spolicy"
)

type Builder struct {
	client  k8s.KubernetesClient
	sources []policy.PolicySource
}

func NewBuilder(client k8s.KubernetesClient) *Builder {
	return &Builder{
		client: client,
		sources: []policy.PolicySource{
			k8spolicy.New(client),
			// istio.New(client),  // future
		},
	}
}

func (b *Builder) BuildGraph(namespaces []string) (Graph, error) {
	indexByNS := map[string]models.NSIndex{}
	var allNodes []models.WorkloadNode
	var mu sync.Mutex
	var firstErr error
	var wg sync.WaitGroup

	for _, ns := range namespaces {
		wg.Add(1)
		go func(ns string) {
			defer wg.Done()

			nsIndex, err := b.buildNsIndex(ns)
			if err != nil {
				mu.Lock()
				if firstErr == nil {
					firstErr = err
				}
				mu.Unlock()
				return
			}

			mu.Lock()
			indexByNS[ns] = nsIndex
			allNodes = append(allNodes, nsIndex.Workloads...)
			mu.Unlock()
		}(ns)
	}

	wg.Wait()

	if firstErr != nil {
		return Graph{}, firstErr
	}

	// Run every registered policy engine. Each engine returns its own Allow
	// rules + per-workload PolicyStatus. Rules concatenate (each engine's
	// edges render independently); PolicyStatus is collected per-engine so
	// updateStatusKeys can intersect them later.
	var allRules []policy.Rule
	statusBySourcePerNode := map[string]map[string]models.PolicyStatus{}

	for _, source := range b.sources {
		result := source.Evaluate(context.Background(), namespaces, indexByNS)
		allRules = append(allRules, result.Allow...)
		for nodeID, status := range result.PolicyStatuses {
			if statusBySourcePerNode[nodeID] == nil {
				statusBySourcePerNode[nodeID] = map[string]models.PolicyStatus{}
			}
			statusBySourcePerNode[nodeID][source.Name()] = status
		}
	}

	allNodes = updateStatusKeys(allNodes, statusBySourcePerNode)
	edges := renderEdges(allRules, allNodes)

	return Graph{Nodes: allNodes, Edges: edges}, nil
}
