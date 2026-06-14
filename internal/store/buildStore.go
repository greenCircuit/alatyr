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
		Port:         rule.Port,
		L7Match:      rule.L7Match,
		Action:       rule.Action,
		Contributors: rule.Contributors,
		DstID:        rule.DstID,
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

// GetNodeData returns per-engine NodeInfo for the workload. Inbound rules
// (DstID == nodeId) are deliberately excluded for now; will be added when
// click-on-node UI needs them.
func GetNodeData(data *models.Cache, nodeId string, ns string) map[string]models.NodeInfo {
	idIndex := buildWorkloadIDIndex(data)
	out := map[string]models.NodeInfo{}
	for engineName, engineEvaluate := range data.EvaluationResults {
		var policyRules []models.NodeRule
		for _, rule := range engineEvaluate.AllowByNs[ns] {
			if rule.SrcID == nodeId {
				policyRules = append(policyRules, toNodeRule(rule, idIndex))
			}
		}
		for _, rule := range engineEvaluate.DenyByNs[ns] {
			if rule.SrcID == nodeId {
				policyRules = append(policyRules, toNodeRule(rule, idIndex))
			}
		}
		policies := engineEvaluate.NodePolicies[nodeId]
		if len(policyRules) == 0 && len(policies) == 0 {
			continue
		}
		out[engineName] = models.NodeInfo{Rules: policyRules, Policies: policies}
	}
	return out
}

// IsNodesReachable evaluates src→dst across every policy engine + every mesh
// source in one pass. Both pod-ID and ns-node-ID participate as matchers so
// pod↔pod, pod↔ns, ns↔pod, and ns↔ns peer rules are caught without separate
// calls. Locks come from PolicyStatuses; deny matches subtract per-direction.
// Mesh pass runs after the engine loop and adds transport-layer (mTLS)
// reachability — block when dst requires STRICT but src cannot speak mTLS.
func IsNodesReachable(ctx context.Context, data *models.Cache, meshSources []mesh.MeshSource, srcNodeId, srcNodeNs, destNodeId, destNodeNs string) ReachabilityResult {
	// NsIndex entries are missing for external nodes (ns=="") and any ns not
	// fetched yet; NSNode is *WorkloadNode so the zero NSIndex has a nil pointer.
	// Tolerate both — empty matcher just never matches.
	srcNsId := ""
	if idx, ok := data.NsIndex[srcNodeNs]; ok && idx.NSNode != nil {
		srcNsId = idx.NSNode.ID
	}
	dstNsId := ""
	if idx, ok := data.NsIndex[destNodeNs]; ok && idx.NSNode != nil {
		dstNsId = idx.NSNode.ID
	}
	idIndex := buildWorkloadIDIndex(data)

	result := ReachabilityResult{
		Verdict: "allow",
		Engines: map[string]EngineVerdict{},
	}
	blockedBy := ""

	for engineName, eval := range data.EvaluationResults {
		ev := EngineVerdict{
			Egress:      DirectionVerdict{Locked: eval.PolicyStatuses[srcNodeId].EgressLocked},
			Ingress:     DirectionVerdict{Locked: eval.PolicyStatuses[destNodeId].IngressLocked},
			SrcPolicies: eval.NodePolicies[srcNodeId],
			DstPolicies: eval.NodePolicies[destNodeId],
		}

		// EGRESS deny — policy lives in src ns
		for _, rule := range eval.DenyByNs[srcNodeNs] {
			if (rule.SrcID == srcNodeId || rule.SrcID == srcNsId) &&
				(rule.DstID == destNodeId || rule.DstID == dstNsId) &&
				rule.Direction == models.DirectionEgress {
				ev.Egress.DenyMatches = append(ev.Egress.DenyMatches, toNodeRule(rule, idIndex))
			}
		}
		// EGRESS allow
		for _, rule := range eval.AllowByNs[srcNodeNs] {
			if (rule.SrcID == srcNodeId || rule.SrcID == srcNsId) &&
				(rule.DstID == destNodeId || rule.DstID == dstNsId) &&
				rule.Direction == models.DirectionEgress {
				ev.Egress.AllowMatches = append(ev.Egress.AllowMatches, toNodeRule(rule, idIndex))
			}
		}
		// INGRESS deny — policy lives in dst ns
		for _, rule := range eval.DenyByNs[destNodeNs] {
			if (rule.SrcID == srcNodeId || rule.SrcID == srcNsId) &&
				(rule.DstID == destNodeId || rule.DstID == dstNsId) &&
				rule.Direction == models.DirectionIngress {
				ev.Ingress.DenyMatches = append(ev.Ingress.DenyMatches, toNodeRule(rule, idIndex))
			}
		}
		// INGRESS allow
		for _, rule := range eval.AllowByNs[destNodeNs] {
			if (rule.SrcID == srcNodeId || rule.SrcID == srcNsId) &&
				(rule.DstID == destNodeId || rule.DstID == dstNsId) &&
				rule.Direction == models.DirectionIngress {
				ev.Ingress.AllowMatches = append(ev.Ingress.AllowMatches, toNodeRule(rule, idIndex))
			}
		}

		egressOK := len(ev.Egress.DenyMatches) == 0 && (!ev.Egress.Locked || len(ev.Egress.AllowMatches) > 0)
		ingressOK := len(ev.Ingress.DenyMatches) == 0 && (!ev.Ingress.Locked || len(ev.Ingress.AllowMatches) > 0)
		ev.Egress.Reason = directionReason(ev.Egress, egressOK)
		ev.Ingress.Reason = directionReason(ev.Ingress, ingressOK)

		total := len(ev.Egress.AllowMatches) + len(ev.Egress.DenyMatches) +
			len(ev.Ingress.AllowMatches) + len(ev.Ingress.DenyMatches)
		switch {
		case !ev.Egress.Locked && !ev.Ingress.Locked && total == 0:
			ev.Status = "not enforced"
		case egressOK && ingressOK:
			ev.Status = "allow"
		default:
			ev.Status = "deny"
			if blockedBy == "" {
				blockedBy = engineName
			} else {
				blockedBy = blockedBy + ", " + engineName
			}
		}
		result.Engines[engineName] = ev
	}

	// Mesh pass — transport-layer check (mTLS) + per-side state for the UI.
	// Skip if either endpoint is not a known workload (e.g. external nodes)
	// — mesh has no opinion on off-cluster peers.
	if len(meshSources) > 0 {
		srcWorkload, srcOk := idIndex[srcNodeId]
		dstWorkload, dstOk := idIndex[destNodeId]
		if srcOk && dstOk {
			srcNsLabels := nsLabels(data, srcNodeNs)
			dstNsLabels := nsLabels(data, destNodeNs)
			result.Mesh = map[string]models.MeshVerdict{}
			result.SrcMesh = map[string]*models.MeshMembership{}
			result.DstMesh = map[string]*models.MeshMembership{}
			for _, src := range meshSources {
				// TODO(perf): N+1 ResolveMtls — outer ResolveMtls(src/dst) below
				// plus CanReach internally re-resolves both. 4 calls per click
				// when 2 would do. Fix by adding CanReachFromStates(srcState,
				// dstState, port) and resolving once here.
				// Per-side membership + mTLS, so the UI can show why mesh denied.
				if m := src.Membership(srcWorkload, srcNsLabels); m != nil {
					if m.InMesh {
						if mtls, err := src.ResolveMtls(ctx, srcWorkload, srcNsLabels); err == nil {
							m.Mtls = mtls
						}
					}
					result.SrcMesh[src.Name()] = m
				}
				if m := src.Membership(dstWorkload, dstNsLabels); m != nil {
					if m.InMesh {
						if mtls, err := src.ResolveMtls(ctx, dstWorkload, dstNsLabels); err == nil {
							m.Mtls = mtls
						}
					}
					result.DstMesh[src.Name()] = m
				}

				v := src.CanReach(ctx, srcWorkload, dstWorkload, srcNsLabels, dstNsLabels, 0)
				result.Mesh[src.Name()] = v
				if v.Verdict == "deny" {
					if blockedBy == "" {
						blockedBy = "mesh:" + src.Name()
					} else {
						blockedBy = blockedBy + ", mesh:" + src.Name()
					}
				}
			}
		}
	}

	if blockedBy != "" {
		result.Verdict = "deny"
		result.Reason = "blocked by: " + blockedBy
	} else {
		result.Reason = "all engines + mesh permit"
	}
	return result
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



