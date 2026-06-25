package store

import (
	"context"
	"graph/internal/mesh"
	"graph/internal/models"
)

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