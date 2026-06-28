package store

import (
	"context"
	"fmt"
	"sync"

	"graph/internal/graph"
	"graph/internal/k8s"
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
}

func NewBuilder(client k8s.KubernetesClient) *Builder {
	return &Builder{
		client:      client,
		sources:     defaultSources(client),
		meshSources: defaultMeshSources(client),
	}
}

func defaultSources(client k8s.KubernetesClient) []policy.PolicySource {
	return []policy.PolicySource{
		k8spolicy.New(client),
		istio.New(client),
	}
}

func defaultMeshSources(client k8s.KubernetesClient) []mesh.MeshSource {
	return []mesh.MeshSource{
		meshistio.New(client),
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
			nsIndex, err := b.fetchNsIndex(ns)
			if err != nil {
				mu.Lock()
				if firstErr == nil {
					firstErr = err
				}
				mu.Unlock()
				return
			}
			mu.Lock()
			cache.NsIndex[ns] = nsIndex
			mu.Unlock()
		}(ns)
	}
	wg.Wait()
	if firstErr != nil {
		return firstErr
	}

	indexByNS := map[string]models.NSIndex{}
	for _, ns := range namespaces {
		indexByNS[ns] = cache.NsIndex[ns]
	}

	for _, source := range b.sources {
		result, err := source.Evaluate(context.Background(), namespaces, indexByNS)
		if err != nil {
			return fmt.Errorf("engine %s: %w", source.Name(), err)
		}
		cache.EvaluationResults[source.Name()] = result
	}

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


// buildWorkloadIDIndex collects every cached workload into a flat ID → node
// lookup so cross-namespace rule destinations can be resolved to labels.
func buildWorkloadIDIndex(data *models.Cache) map[string]models.WorkloadNode {
	index := map[string]models.WorkloadNode{}
	for _, nsIndex := range data.NsIndex {
		for _, workload := range nsIndex.Workloads {
			index[workload.ID] = workload
		}
	}
	return index
}

func toNodeRule(rule models.Rule, idIndex map[string]models.WorkloadNode) models.NodeRule {
	view := models.NodeRule{
		Direction:    rule.Direction,
		Ports:        rule.Ports,
		L7Match:      rule.L7Match,
		Action:       rule.Action,
		Contributor:  rule.Contributor,
		DstID:        rule.DstID,
		DstSelector:  rule.DstSelector,
		SrcSelector:  rule.SrcSelector,
	}
	if dst, ok := idIndex[rule.DstID]; ok {
		view.DstLabel = dst.Label
		view.DstNamespace = dst.Namespace
	}
	return view
}

// GetWorkloadMesh resolves per-source mesh state for a single node on demand.
// For workload nodes: membership (cheap label check) + mTLS (one k8s
// round-trip). For namespace nodes: membership only — PA resolution is
// per-pod and doesn't roll up to a single ns-level verdict. Sources whose
// ResolveMtls errors surface membership only. Returns nil for unknown nodes.
func GetWorkloadMesh(ctx context.Context, data *models.Cache, sources []mesh.MeshSource, nodeId, ns string) map[string]*models.MeshMembership {
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
			}
		}
		out[src.Name()] = membership
	}
	return out
}


// nsLabels returns the ns object's k8s labels from cached NSIndex, or nil
// when the ns isn't in cache (e.g. external).
func nsLabels(data *models.Cache, ns string) map[string]string {
	idx, ok := data.NsIndex[ns]
	if !ok || idx.NSNode == nil {
		return nil
	}
	return idx.NSNode.Labels
}

// directionReason produces the per-direction human-readable explanation
// surfaced to operators.
func directionReason(d DirectionVerdict, ok bool) string {
	switch {
	case len(d.DenyMatches) > 0:
		return "explicit deny rule matched"
	case d.Locked && len(d.AllowMatches) == 0:
		return "default-deny: locked, no allow rule matches"
	case !d.Locked && len(d.AllowMatches) == 0:
		return "no policy opinion"
	case ok:
		return "permitted by allow rule"
	default:
		return ""
	}
}



