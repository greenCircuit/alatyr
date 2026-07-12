package store

import (
	"context"
	"graph/internal/mesh"
	"graph/internal/models"
)

// collectToSide sorts the src's egress rules (its pod bucket + ns-node bucket)
// into the DirectionVerdict slices by whether each rule targets the dst peer,
// then stamps the reason. Egress rules identify the peer by DstID. Rules are
// converted to NodeRule at append time so the payload carries resolved
// endpoint labels/kinds instead of raw node IDs.
func collectToSide(nsNodeRules models.NodeRules, srcNodeRules models.NodeRules, dstNsNodeId string, dstNodeId string, idIndex map[string]models.WorkloadNode) DirectionVerdict {
	var verdict DirectionVerdict

	collect := func(nodeRules models.NodeRules) {
		for _, rule := range nodeRules.Egress.Allow {
			switch {
			case rule.DstID == dstNodeId || rule.DstID == dstNsNodeId:
				verdict.AllowMatches = append(verdict.AllowMatches, toNodeRule(rule, idIndex))
			case rule.Coverage == models.CoverageDenyAll:
				verdict.DenyAllMatches = append(verdict.DenyAllMatches, toNodeRule(rule, idIndex))
			case rule.Coverage == models.CoverageUnenforced:
				// policy names egress but doesn't constrain it — no opinion, drop
			default:
				verdict.OtherAllowMatches = append(verdict.OtherAllowMatches, toNodeRule(rule, idIndex))
			}
		}
		for _, rule := range nodeRules.Egress.Deny {
			if rule.DstID == dstNodeId || rule.DstID == dstNsNodeId  || rule.Coverage == models.CoverageDenyAll {
				verdict.DenyMatches = append(verdict.DenyMatches, toNodeRule(rule, idIndex))
			}
		}
	}

	collect(nsNodeRules)
	collect(srcNodeRules)
	verdict.Reason, verdict.Culprits = decideDirectionVerdict(verdict)
	return verdict
}

// collectFromSide sorts the dst's ingress rules (its pod bucket + ns-node
// bucket) by whether each rule admits the src peer, then stamps the reason.
// Ingress rules identify the peer by SrcID.
func collectFromSide(nsNodeRules models.NodeRules, dstNodeRules models.NodeRules, srcNsNodeId string, srcNodeId string, idIndex map[string]models.WorkloadNode) DirectionVerdict {
	var verdict DirectionVerdict

	collect := func(nodeRules models.NodeRules) {
		for _, rule := range nodeRules.Ingress.Allow {

			switch {
				case rule.SrcID == srcNodeId || rule.SrcID == srcNsNodeId:
					verdict.AllowMatches = append(verdict.AllowMatches, toNodeRule(rule, idIndex))
				case rule.Coverage == models.CoverageDenyAll:
					verdict.DenyAllMatches = append(verdict.DenyAllMatches, toNodeRule(rule, idIndex))
				case rule.Coverage == models.CoverageUnenforced:
					// policy names ingress but doesn't constrain it — no opinion, drop
				default:
					verdict.OtherAllowMatches = append(verdict.OtherAllowMatches, toNodeRule(rule, idIndex))
			}
		}
		for _, rule := range nodeRules.Ingress.Deny {
			if rule.SrcID == srcNodeId || rule.SrcID == srcNsNodeId || rule.Coverage == models.CoverageDenyAll {
				verdict.DenyMatches = append(verdict.DenyMatches, toNodeRule(rule, idIndex))
			}
		}

	}

	collect(nsNodeRules)
	collect(dstNodeRules)
	verdict.Reason, verdict.Culprits = decideDirectionVerdict(verdict)
	return verdict
}

// decideDirectionVerdict derives the reason for one direction from which slices
// are populated. Order is precedence, most-specific first: explicit deny wins,
// then an allow to this peer permits, then "allows elsewhere" (locked but not
// for us), then a bare deny-all marker (genuine default-deny), else nothing
// governs.
func decideDirectionVerdict(verdict DirectionVerdict) (DirectionReason, []models.PolicyRef) {
	if len(verdict.DenyMatches) != 0 {
		return ReasonExplicitDeny, dedupContributors(verdict.DenyMatches)
	}
	if len(verdict.AllowMatches) != 0 {
		return ReasonPermitted, nil
	}
	if len(verdict.OtherAllowMatches) != 0 {
		return ReasonLockedNoMatch, dedupContributors(verdict.OtherAllowMatches)
	}
	if len(verdict.DenyAllMatches) != 0 {
		return ReasonDefaultDeny, dedupContributors(verdict.DenyAllMatches)
	}
	return ReasonNoOpinion, nil
}

// dedupContributors distills a rule slice to the unique policies that produced
// it — the netpols an operator would edit. Rules repeat a Contributor per
// port/peer, so dedup by PolicyRef (comparable: all string/int fields).
func dedupContributors(rules []models.NodeRule) []models.PolicyRef {
	// Dedup by policy identity, not full PolicyRef — a netpol emits one rule per
	// egress/ingress stanza (distinct RuleIndex), but the operator edits the
	// policy, not a rule line. Keying on RuleIndex would show the same policy twice.
	type policyIdentity struct{ source, name, namespace string }
	seen := map[policyIdentity]bool{}
	var culprits []models.PolicyRef
	for _, rule := range rules {
		policy := rule.Contributor
		identity := policyIdentity{policy.Source, policy.Name, policy.Namespace}
		if !seen[identity] {
			seen[identity] = true
			culprits = append(culprits, policy)
		}
	}
	return culprits
}

// reachabilityProber memoizes fused per-pair verdicts so the discovery sweep
// and layering classification never probe the same pair twice in one request.
// IsNodesReachable is deterministic over the request cache, so the ordered
// pair is a full key.
type reachabilityProber struct {
	data        *models.Cache
	meshSources []mesh.MeshSource
	memo        map[string]ReachabilityResult
}

func newReachabilityProber(data *models.Cache, meshSources []mesh.MeshSource) *reachabilityProber {
	return &reachabilityProber{
		data:        data,
		meshSources: meshSources,
		memo:        make(map[string]ReachabilityResult),
	}
}

func (prober *reachabilityProber) probe(ctx context.Context, srcID, srcNs, dstID, dstNs string) ReachabilityResult {
	memoKey := srcID + "->" + dstID
	if cached, ok := prober.memo[memoKey]; ok {
		return cached
	}
	result := IsNodesReachable(ctx, prober.data, prober.meshSources, srcID, srcNs, dstID, dstNs)
	prober.memo[memoKey] = result
	return result
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
	idIndex := data.WorkloadByID

	result := ReachabilityResult{
		Verdict: "allow",
		Engines: map[string]EngineVerdict{},
	}
	blockedBy := ""
	for engineName, evaluation := range data.EvaluationResults {
		egressEval := collectToSide(evaluation.NodeRules[srcNsId], evaluation.NodeRules[srcNodeId], dstNsId, destNodeId, idIndex)
		ingressEval := collectFromSide(evaluation.NodeRules[dstNsId], evaluation.NodeRules[destNodeId], srcNsId, srcNodeId, idIndex)

		engineVerdict := EngineVerdict{
			Egress:      egressEval,
			Ingress:     ingressEval,
			SrcPolicies: evaluation.NodePolicies[srcNodeId],
			DstPolicies: evaluation.NodePolicies[destNodeId],
		}

		// A direction permits when it explicitly allows this peer or no policy
		// governs it; the three block reasons (explicit-deny, locked-no-match,
		// default-deny) do not. Both sides must permit for traffic to flow.
		egressOK := false
		if egressEval.Reason == ReasonPermitted || egressEval.Reason == ReasonNoOpinion {
			egressOK = true
		}
		ingressOK := false
		if ingressEval.Reason == ReasonPermitted || ingressEval.Reason == ReasonNoOpinion {
			ingressOK = true
		}

		switch {
			case egressEval.Reason == ReasonNoOpinion && ingressEval.Reason == ReasonNoOpinion:
				engineVerdict.Status = "not enforced"
			case egressOK && ingressOK:
				engineVerdict.Status = "allow"
			default:
				engineVerdict.Status = "deny"
				if blockedBy == "" {
					blockedBy = engineName
				} else {
					blockedBy = blockedBy + ", " + engineName
				}
		}
		result.Engines[engineName] = engineVerdict
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