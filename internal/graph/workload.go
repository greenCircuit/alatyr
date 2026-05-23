package graph

import (
	"context"
	"sync"

	"graph/internal/k8s"
	"graph/internal/models"
	"graph/internal/policy/k8spolicy"
)

type Builder struct {
	client k8s.KubernetesClient
}

func NewBuilder(client k8s.KubernetesClient) *Builder {
	return &Builder{client: client}
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

	result := k8spolicy.New(b.client).Evaluate(context.Background(), namespaces, indexByNS)
	edges := foldTuplesToEdges(result.Allow, allNodes)

	return Graph{Nodes: allNodes, Edges: edges}, nil
}
