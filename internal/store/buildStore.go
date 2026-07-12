package store

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"graph/internal/graph"
	"graph/internal/k8s"
	"graph/internal/logging"
	"graph/internal/mesh"
	meshistio "graph/internal/mesh/istio"
	"graph/internal/models"
	"graph/internal/policy"
	"graph/internal/policy/istio"
	"graph/internal/policy/k8spolicy"
)

// Builder owns the k8s client and the registered engine + mesh source lists.
// Constructed once at server start; methods take only per-request inputs
// (cache, namespaces) so call sites never thread the client through.
type Builder struct {
	client       k8s.KubernetesClient
	sources      []policy.PolicySource
	meshSources  []mesh.MeshSource
	log          *slog.Logger
}

func NewBuilder(client k8s.KubernetesClient, logger *slog.Logger) *Builder {
	if logger == nil {
		logger = slog.Default()
	}
	return &Builder{
		client:      client,
		sources:     defaultSources(client, logger),
		meshSources: defaultMeshSources(client, logger),
		log:         logger,
	}
}

func defaultSources(client k8s.KubernetesClient, logger *slog.Logger) []policy.PolicySource {
	return []policy.PolicySource{
		k8spolicy.New(client, logger),
		istio.New(client, logger),
	}
}

func defaultMeshSources(client k8s.KubernetesClient, logger *slog.Logger) []mesh.MeshSource {
	return []mesh.MeshSource{
		meshistio.New(client, logger),
	}
}

// MeshSources exposes the registered mesh sources so the API layer can call
// ResolveMtls on demand from the detail / reachability endpoints. Returned
// slice mirrors registration order.
func (b *Builder) MeshSources() []mesh.MeshSource {
	return b.meshSources
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
		b.log.Debug("engine evaluate done",
			slog.String("phase", "engine_evaluate"),
			slog.String("engine", source.Name()),
			slog.Int("allow_ns_count", len(result.AllowByNs)),
			slog.Int("deny_ns_count", len(result.DenyByNs)),
			slog.Int64("duration_ms", time.Since(engineStart).Milliseconds()),
		)
		cache.EvaluationResults[source.Name()] = result
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

// GetWorkloadMesh resolves per-source mesh state for a single node on demand.
// For workload nodes: membership (cheap label check) + mTLS (one k8s
// round-trip). For namespace nodes: membership only — PA resolution is
// per-pod and doesn't roll up to a single ns-level verdict. Sources whose
// ResolveMtls errors surface membership only. Returns nil for unknown nodes.
func GetWorkloadMesh(ctx context.Context, data *models.Cache, sources []mesh.MeshSource, nodeId, ns string) map[string]*models.MeshMembership {
	logger := logging.FromCtx(ctx)
	nsIndex, ok := data.NsIndex[ns]
	if !ok {
		return nil
	}
	var workload *models.WorkloadNode
	for i := range nsIndex.Workloads {
		w := &nsIndex.Workloads[i]
		if w.ID == nodeId {
			workload = w
			break
		}
	}
	if workload == nil {
		return nil
	}
	var nsLabels map[string]string
	if nsIndex.NSNode != nil {
		nsLabels = nsIndex.NSNode.Labels
	}
	isNamespace := workload.Type == models.NodeTypeNamespace

	out := map[string]*models.MeshMembership{}
	for _, src := range sources {
		membership := src.Membership(*workload, nsLabels)
		if membership == nil {
			continue
		}
		if membership.InMesh && !isNamespace {
			if mtls, err := src.ResolveMtls(ctx, *workload, nsLabels); err == nil {
				membership.Mtls = mtls
			} else {
				logger.Warn("mesh mtls resolve failed",
					slog.String("phase", "get_workload_mesh"),
					slog.String("source", src.Name()),
					slog.String("node_id", nodeId),
					slog.String("ns", ns),
					slog.String("error", err.Error()),
				)
			}
		}
		out[src.Name()] = membership
	}
	return out
}


