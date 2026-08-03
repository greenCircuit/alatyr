package store

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"graph/internal/graph"
	"graph/internal/k8s"
	"graph/internal/mesh"
	meshistio "graph/internal/mesh/istio"
	"graph/internal/models"
	"graph/internal/policy"
	"graph/internal/policy/calico"
	"graph/internal/policy/istio"
	"graph/internal/policy/k8spolicy"
)

// Builder owns the k8s client and the registered engine list + mesh source.
// Constructed once at server start; methods take only per-request inputs
// (cache, namespaces) so call sites never thread the client through.
// meshSource is a single provider — Istio is the only supported mesh today
// and multi-mesh in one cluster is a construction that doesn't exist in prod.
// May be nil if no mesh provider is configured.
type Builder struct {
	client      k8s.KubernetesClient
	sources     []policy.PolicySource
	meshSource  mesh.MeshSource
	log         *slog.Logger
}

func NewBuilder(client k8s.KubernetesClient, logger *slog.Logger) *Builder {
	if logger == nil {
		logger = slog.Default()
	}
	return &Builder{
		client:     client,
		sources:    defaultSources(client, logger),
		meshSource: defaultMeshSource(client, logger),
		log:        logger,
	}
}

func defaultSources(client k8s.KubernetesClient, logger *slog.Logger) []policy.PolicySource {
	return []policy.PolicySource{
		k8spolicy.New(client, logger),
		istio.New(client, logger),
		calico.New(client, logger),
	}
}

func defaultMeshSource(client k8s.KubernetesClient, logger *slog.Logger) mesh.MeshSource {
	return meshistio.New(client, logger)
}

// MeshSource returns the single registered mesh provider. Nil when none
// configured — callers must guard.
func (b *Builder) MeshSource() mesh.MeshSource {
	return b.meshSource
}

// MeshSources exposes the registered mesh source (as a slice for the api-layer
// helpers that still iterate). Returns an empty slice when no mesh provider
// is configured so callers can range safely.
func (b *Builder) MeshSources() []mesh.MeshSource {
	if b.meshSource == nil {
		return nil
	}
	return []mesh.MeshSource{b.meshSource}
}

// EngineNames returns the registered engine identifiers. Used by the
// cluster-state endpoint to populate the UI source filter.
func (b *Builder) EngineNames() []string {
	out := make([]string, 0, len(b.sources))
	for _, src := range b.sources {
		out = append(out, src.Name())
	}
	return out
}

// PopulateCache fetches NSIndex for every requested namespace in parallel
// then runs every registered engine over the full selected-ns index set,
// storing results in cache. Replaces existing entries for the touched
// namespaces and engines; entries for namespaces outside the request are
// left untouched.
func (b *Builder) PopulateCache(cache *models.Cache, namespaces []string) error {
	started := time.Now()
	if cache.NsIndex == nil {
		cache.NsIndex = map[string]models.NSIndex{}
	}
	if cache.EvaluationResults == nil {
		cache.EvaluationResults = map[string]models.EvaluationResult{}
	}

	var mu sync.Mutex
	var firstErr error
	var wg sync.WaitGroup

	for _, ns := range namespaces {
		wg.Add(1)
		go func(ns string) {
			defer wg.Done()
			nsStart := time.Now()
			nsIndex, err := b.fetchNsIndex(ns)
			if err != nil {
				b.log.Warn("ns index fetch failed",
					slog.String("phase", "fetch_ns_index"),
					slog.String("ns", ns),
					slog.String("error", err.Error()),
				)
				mu.Lock()
				if firstErr == nil {
					firstErr = err
				}
				mu.Unlock()
				return
			}
			b.log.Debug("ns index fetched",
				slog.String("phase", "fetch_ns_index"),
				slog.String("ns", ns),
				slog.Int("workload_count", len(nsIndex.Workloads)),
				slog.Int64("duration_ms", time.Since(nsStart).Milliseconds()),
			)
			mu.Lock()
			cache.NsIndex[ns] = nsIndex
			mu.Unlock()
		}(ns)
	}
	wg.Wait()
	if firstErr != nil {
		return firstErr
	}
	// flatten ns index to flat structure id: node
	cache.RebuildWorkloadIndex()

	indexByNS := map[string]models.NSIndex{}
	for _, ns := range namespaces {
		indexByNS[ns] = cache.NsIndex[ns]
	}

	for _, source := range b.sources {
		engineStart := time.Now()
		b.log.Debug("engine evaluate start",
			slog.String("phase", "engine_evaluate"),
			slog.String("engine", source.Name()),
			slog.Int("ns_count", len(namespaces)),
		)
		result, err := source.Evaluate(context.Background(), namespaces, indexByNS)
		if err != nil {
			b.log.Error("engine evaluate failed",
				slog.String("phase", "engine_evaluate"),
				slog.String("engine", source.Name()),
				slog.String("error", err.Error()),
			)
			return fmt.Errorf("engine %s: %w", source.Name(), err)
		}
		allowRuleCount, denyRuleCount := 0, 0
		for _, rules := range result.AllowByNs {
			for _, rule := range rules {
				if rule.Action == models.ActionDeny {
					denyRuleCount++
				} else {
					allowRuleCount++
				}
			}
		}
		for _, rules := range result.DenyByNs {
			denyRuleCount += len(rules)
		}
		b.log.Debug("engine evaluate done",
			slog.String("phase", "engine_evaluate"),
			slog.String("engine", source.Name()),
			slog.Int("allow_rule_count", allowRuleCount),
			slog.Int("deny_rule_count", denyRuleCount),
			slog.Int64("duration_ms", time.Since(engineStart).Milliseconds()),
		)
		cache.EvaluationResults[source.Name()] = result
	}
	// Engines just produced CIDR peer nodes; fold them into WorkloadByID so
	// node-info, neighbors, and layering resolve external endpoints.
	cache.RebuildWorkloadIndex()

	if b.meshSource != nil {
		meshStart := time.Now()
		b.log.Debug("mesh populate start",
			slog.String("phase", "mesh_populate"),
			slog.String("provider", b.meshSource.Name()),
			slog.Int("ns_count", len(namespaces)),
		)
		result, err := b.meshSource.BuildMeshMembership(cache.NsIndex)
		if err != nil {
			b.log.Error("mesh populate failed",
				slog.String("phase", "mesh_populate"),
				slog.String("provider", b.meshSource.Name()),
				slog.String("error", err.Error()),
			)
			return fmt.Errorf("mesh %s: %w", b.meshSource.Name(), err)
		}
		cache.MeshMembership = result.Memberships
		cache.MeshMetrics = result.Metrics
		cache.MeshIssues = result.Issues

		// Transport-layer hygiene: per enrolled workload, check whether other
		// engines' rules restrict ports without allowing HBONE (15008). Runs
		// after engine evaluation and mesh membership are both populated so
		// GetNodeData + ambient membership are ready.
		for nodeID, membership := range cache.MeshMembership {
			if !membership.InMesh {
				continue
			}
			workload, ok := cache.WorkloadByID[nodeID]
			if !ok {
				continue
			}
			policies := GetNodeData(cache, nodeID, workload.Namespace)
			cache.MeshIssues = append(cache.MeshIssues,
				meshistio.ValidateExternalRules(workload, cache, &membership, policies)...)
		}

		b.log.Debug("mesh populate done",
			slog.String("phase", "mesh_populate"),
			slog.String("provider", b.meshSource.Name()),
			slog.Int("node_count", len(result.Memberships)),
			slog.Int("issue_count", len(cache.MeshIssues)),
			slog.Any("metrics", result.Metrics),
			slog.Int64("duration_ms", time.Since(meshStart).Milliseconds()),
		)
	}

	b.log.Info("populate cache done",
		slog.String("phase", "populate_cache_done"),
		slog.Int("ns_count", len(namespaces)),
		slog.Int("engine_count", len(b.sources)),
		slog.Int64("total_duration_ms", time.Since(started).Milliseconds()),
	)
	return nil
}

func (b *Builder) fetchNsIndex(ns string) (models.NSIndex, error) {
	pods, err := b.client.GetPods(ns)
	if err != nil {
		return models.NSIndex{}, err
	}
	cronJobs, err := b.client.GetCronJobs(ns)
	if err != nil {
		return models.NSIndex{}, err
	}
	nsObj, err := b.client.GetNs(ns)
	if err != nil {
		return models.NSIndex{}, err
	}
	return graph.AssembleNsIndex(pods, cronJobs, nsObj), nil
}

func GetByNodeInNs(data *models.Cache, nodeId string, nodeNs string) (models.WorkloadNode, error) {
	_, ok := data.NsIndex[nodeNs]
	if !ok {
		return models.WorkloadNode{}, fmt.Errorf("couldn't find requested ns %s", nodeNs)
	}
	for _, node := range data.NsIndex[nodeNs].Workloads {
		if node.ID == nodeId {
			return node, nil
		}
	}

	return models.WorkloadNode{}, fmt.Errorf("couldn't find node: %s in ns: %s", nodeId, nodeNs)

}

func GetWorkloadMesh(data *models.Cache, nodeId string) models.MeshMembership {
	return data.MeshMembership[nodeId]
}


