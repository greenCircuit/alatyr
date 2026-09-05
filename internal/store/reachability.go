package store

import (
	"context"
	"fmt"

	"alatyr/internal/mesh"
	"alatyr/internal/models"
	"alatyr/internal/policy"
)

// cidrNodeCoversPeer reports whether a rule whose peer endpoint is the synthetic
// CIDR node cidrID actually speaks for the queried peer. Used on both sides: an
// allow only permits a peer its CIDR contains, and an ipBlock except carve-out
// (Coverage=CoverageExcept) only bites when the peer falls inside the except range.
//
// Two cases, split by peer kind:
//   - peer is a CIDR node (external/WAN): real subnet containment. An internal
//     /16 does NOT contain 0.0.0.0/0, so allowing the pod CIDR must not count as
//     allowing the internet — and an internal except must not block the WAN node.
//   - peer is an internal workload: its runtime IP lives in the cluster pod/svc
//     space, so any range covering the cluster CIDR speaks for it.
func cidrNodeCoversPeer(cidrID, peerID string, idIndex map[string]models.WorkloadNode) bool {
	cidrNode, ok := idIndex[cidrID]
	if !ok || cidrNode.Type != models.NodeTypeCIDR {
		return false
	}
	if peerNode, ok := idIndex[peerID]; ok && peerNode.Type == models.NodeTypeCIDR {
		return policy.CidrContains(cidrNode.Label, peerNode.Label)
	}
	return cidrNode.CidrType != models.CIDRLan
}

// policyIdentityKey — the policy an operator would edit. RuleIndex excluded on
// purpose: a policy's allow stanza and its catch-all deny are separate rules of
// the SAME policy.
func policyIdentityKey(ref models.PolicyRef) string {
	return ref.Source + "|" + ref.Namespace + "|" + ref.Name
}

// policiesAllowingPeerSpecifically collects the policies that allow the queried
// peer by NAMING it — its node ID, its namespace node, or a peer CIDR that
// contains it. Blanket allow-all does not count.
//
// This is what makes ordered first-match engines resolve correctly. Calico
// buckets a policy by peer: `allow nets 10.42.0.0/16 … then deny` emits a narrow
// allow for the pod-CIDR bucket AND the policy's catch-all deny for the
// 0.0.0.0/0 bucket. Both are real; which one governs depends on the peer. When a
// policy named the queried peer, its own catch-all deny is the fallback for
// OTHER peers and must not block this one. Istio can't produce that shape (ALLOW
// and DENY are separate objects), so its deny-overrides behavior is untouched.
func policiesAllowingPeerSpecifically(
	allowRules []models.Rule,
	peerOf func(models.Rule) string,
	peerID, peerNsID string,
	idIndex map[string]models.WorkloadNode,
) map[string]bool {
	allowing := map[string]bool{}
	for _, rule := range allowRules {
		peer := peerOf(rule)
		if peer == peerID || peer == peerNsID || cidrNodeCoversPeer(peer, peerID, idIndex) {
			allowing[policyIdentityKey(rule.Contributor)] = true
		}
	}
	return allowing
}

// collectToSide sorts the src's egress rules (its pod bucket + ns-node bucket)
// into the DirectionVerdict slices by whether each rule targets the dst peer,
// then stamps the reason. Egress rules identify the peer by DstID. Rules are
// converted to NodeRule at append time so the payload carries resolved
// endpoint labels/kinds instead of raw node IDs.
func collectToSide(nsNodeRules models.NodeRules, srcNodeRules models.NodeRules, dstNsNodeId string, dstNodeId string, idIndex map[string]models.WorkloadNode) DirectionVerdict {
	var verdict DirectionVerdict
	var allowAllPorts bool
	ports := []models.Port{}
	// dedup key: port+endPort+protocol — TCP/53 and UDP/53 stay distinct,
	// Name is display-only so named vs unnamed same port don't duplicate
	seenPorts := map[string]bool{}
	// Built across BOTH buckets before any verdict, so a narrow allow on the pod
	// bucket outranks the same policy's catch-all deny regardless of which bucket
	// each landed in.
	egressPeerOf := func(rule models.Rule) string { return rule.DstID }
	allowingPolicies := policiesAllowingPeerSpecifically(
		append(append([]models.Rule{}, nsNodeRules.Egress.Allow...), srcNodeRules.Egress.Allow...),
		egressPeerOf, dstNodeId, dstNsNodeId, idIndex)
	collect := func(nodeRules models.NodeRules) {
		for _, rule := range nodeRules.Egress.Allow {
			switch {
			case rule.Coverage == models.CoverageDenyAll:
				verdict.DenyAllMatches = append(verdict.DenyAllMatches, toNodeRule(rule, idIndex))
			// Coverage check, not DstID == "" — unenforced/deny-all markers also
			// carry empty DstID, and only allow-all means "matches any peer".
			// cidrCoveringInternal catches ipBlock peers whose CIDR contains the
			// cluster pod/svc CIDR (e.g. 0.0.0.0/0): DstID never equals the
			// workload ID but the rule semantically permits any internal pod.
			case rule.DstID == dstNodeId || rule.DstID == dstNsNodeId || rule.Coverage == models.CoverageAllowAll || cidrNodeCoversPeer(rule.DstID, dstNodeId, idIndex):
				verdict.AllowMatches = append(verdict.AllowMatches, toNodeRule(rule, idIndex))
				if rule.AllPorts {
					allowAllPorts = true
				}
				for _, port := range rule.Ports {
					key := fmt.Sprintf("%d/%d/%s", port.Port, port.EndPort, port.Protocol)
					if seenPorts[key] {
						continue
					}
					seenPorts[key] = true
					ports = append(ports, port)
				}
			case rule.Coverage == models.CoverageUnenforced:
				// policy names egress but doesn't constrain it — no opinion, drop
			default:
				verdict.OtherAllowMatches = append(verdict.OtherAllowMatches, toNodeRule(rule, idIndex))
			}
		}
		for _, rule := range nodeRules.Egress.Deny {
			// A catch-all deny is the policy's fallback for peers it didn't name.
			// Same policy already allowed this peer specifically → that bucket wins.
			if rule.Coverage == models.CoverageDenyAll && allowingPolicies[policyIdentityKey(rule.Contributor)] {
				continue
			}
			// Except carve-outs (Coverage=CoverageExcept) land here with a CIDR
			// DstID — cidrNodeCoversPeer bites only when the except CIDR actually
			// contains the queried dst. They suppress the covering allow but are
			// not deny objects, so they get their own bucket.
			if rule.DstID == dstNodeId || rule.DstID == dstNsNodeId || rule.Coverage == models.CoverageDenyAll || cidrNodeCoversPeer(rule.DstID, dstNodeId, idIndex) {
				if rule.Coverage == models.CoverageExcept {
					verdict.CarveOutMatches = append(verdict.CarveOutMatches, toNodeRule(rule, idIndex))
					continue
				}
				verdict.DenyMatches = append(verdict.DenyMatches, toNodeRule(rule, idIndex))
			}
		}
	}

	collect(nsNodeRules)
	collect(srcNodeRules)
	verdict.Reason, verdict.Culprits = decideDirectionVerdict(verdict)
	verdict.AllPorts = allowAllPorts
	verdict.Ports = ports
	return verdict
}

// collectFromSide sorts the dst's ingress rules (its pod bucket + ns-node
// bucket) by whether each rule admits the src peer, then stamps the reason.
// Ingress rules identify the peer by SrcID.
func collectFromSide(nsNodeRules models.NodeRules, dstNodeRules models.NodeRules, srcNsNodeId string, srcNodeId string, idIndex map[string]models.WorkloadNode) DirectionVerdict {
	var verdict DirectionVerdict
	var allowAllPorts bool
	ports := []models.Port{}
	// dedup key: port+endPort+protocol — TCP/53 and UDP/53 stay distinct,
	// Name is display-only so named vs unnamed same port don't duplicate
	seenPorts := map[string]bool{}
	ingressPeerOf := func(rule models.Rule) string { return rule.SrcID }
	allowingPolicies := policiesAllowingPeerSpecifically(
		append(append([]models.Rule{}, nsNodeRules.Ingress.Allow...), dstNodeRules.Ingress.Allow...),
		ingressPeerOf, srcNodeId, srcNsNodeId, idIndex)
	collect := func(nodeRules models.NodeRules) {
		for _, rule := range nodeRules.Ingress.Allow {
			switch {
			case rule.Coverage == models.CoverageDenyAll:
				verdict.DenyAllMatches = append(verdict.DenyAllMatches, toNodeRule(rule, idIndex))
			// Coverage check, not SrcID == "" — unenforced/deny-all markers also
			// carry empty SrcID, and only allow-all means "matches any peer".
			// cidrCoveringInternal mirrors the egress path: ingress from an
			// ipBlock CIDR that covers the pod/svc CIDR admits any internal src.
			case rule.SrcID == srcNodeId || rule.SrcID == srcNsNodeId || rule.Coverage == models.CoverageAllowAll || cidrNodeCoversPeer(rule.SrcID, srcNodeId, idIndex):
				verdict.AllowMatches = append(verdict.AllowMatches, toNodeRule(rule, idIndex))
				if rule.AllPorts {
					allowAllPorts = true
				}
				for _, port := range rule.Ports {
					key := fmt.Sprintf("%d/%d/%s", port.Port, port.EndPort, port.Protocol)
					if seenPorts[key] {
						continue
					}
					seenPorts[key] = true
					ports = append(ports, port)
				}
			case rule.Coverage == models.CoverageUnenforced:
				// policy names ingress but doesn't constrain it — no opinion, drop
			default:
				verdict.OtherAllowMatches = append(verdict.OtherAllowMatches, toNodeRule(rule, idIndex))
			}
		}
		for _, rule := range nodeRules.Ingress.Deny {
			if rule.Coverage == models.CoverageDenyAll && allowingPolicies[policyIdentityKey(rule.Contributor)] {
				continue
			}
			if rule.SrcID == srcNodeId || rule.SrcID == srcNsNodeId || rule.Coverage == models.CoverageDenyAll || cidrNodeCoversPeer(rule.SrcID, srcNodeId, idIndex) {
				if rule.Coverage == models.CoverageExcept {
					verdict.CarveOutMatches = append(verdict.CarveOutMatches, toNodeRule(rule, idIndex))
					continue
				}
				verdict.DenyMatches = append(verdict.DenyMatches, toNodeRule(rule, idIndex))
			}
		}
	}

	collect(nsNodeRules)
	collect(dstNodeRules)
	verdict.AllPorts = allowAllPorts
	verdict.Ports = ports
	verdict.Reason, verdict.Culprits = decideDirectionVerdict(verdict)
	return verdict
}

// decideDirectionVerdict derives the reason for one direction from which slices
// are populated. Order is precedence, most-specific first: explicit deny wins,
// then an except carve-out (still blocks, but names no deny object), then an
// allow to this peer permits, then "allows elsewhere" (locked but not for us),
// then a bare deny-all marker (genuine default-deny), else nothing governs.
// Carve-outs must outrank AllowMatches — the 0.0.0.0/0 allow they punch a hole
// in also matches the peer, and letting it win would report an excluded range
// as reachable.
func decideDirectionVerdict(verdict DirectionVerdict) (DirectionReason, []models.PolicyRef) {
	if len(verdict.DenyMatches) != 0 {
		return ReasonExplicitDeny, dedupContributors(verdict.DenyMatches)
	}
	if len(verdict.CarveOutMatches) != 0 {
		return ReasonCarvedOut, dedupContributors(verdict.CarveOutMatches)
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
	result := PoliciesCanReach(ctx, prober.data, srcID, srcNs, dstID, dstNs)
	prober.memo[memoKey] = result
	return result
}

// IsNodesReachable evaluates src→dst across every policy engine
func PoliciesCanReach(ctx context.Context, data *models.Cache, srcNodeId, srcNodeNs, destNodeId, destNodeNs string) ReachabilityResult {
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
		// governs it; every block reason (explicit-deny, carved-out,
		// locked-no-match, default-deny) does not. Both sides must permit.
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

	// Verdict here is the policy-layer answer. Mesh (transport) verdicts are
	// separate — callers combine via MeshReachabilityUsingNodes.
	if blockedBy != "" {
		result.Verdict = "deny"
		result.Reason = "blocked by: " + blockedBy
	} else {
		result.Reason = "all engines permit"
	}
	return result
}

// IsEndpointsReachable is the endpoint-facing answer: policy-layer verdict
// fused with the mesh transport verdict. Mesh runs only for workload
// endpoints — membership is per-workload, and the zero membership would
// false-positive a namespace node as "not in mesh".
func IsEndpointsReachable(ctx context.Context, data *models.Cache, meshSource mesh.MeshSource, srcNodeId, srcNodeNs, destNodeId, destNodeNs string) ReachabilityResult {
	result := PoliciesCanReach(ctx, data, srcNodeId, srcNodeNs, destNodeId, destNodeNs)
	if meshSource == nil {
		return result
	}
	srcNode, srcOk := data.WorkloadByID[srcNodeId]
	dstNode, dstOk := data.WorkloadByID[destNodeId]
	if !srcOk || !dstOk || srcNode.Type == models.NodeTypeNamespace || dstNode.Type == models.NodeTypeNamespace {
		return result
	}
	meshReach := MeshReachabilityUsingNodes(data, meshSource, srcNode, dstNode)
	result.Mesh = map[string]models.MeshVerdict{meshSource.Name(): meshReach}
	if meshReach.Verdict == "deny" {
		result.Verdict = "deny"
		result.Reason = "blocked by mesh: " + meshReach.Reason
	}
	return result
}

// MeshReachabilityUsingNodes resolves both memberships from cache and returns
// the mesh source's transport-layer verdict. Missing cache entries decode as
// the zero MeshMembership (InMesh=false) — exactly the plaintext-src input
// CanReach needs. port=0 = any port (workload-level verdict).
func MeshReachabilityUsingNodes(data *models.Cache, meshSource mesh.MeshSource, srcNode models.WorkloadNode, dstNode models.WorkloadNode) models.MeshVerdict {
	srcMesh := data.MeshMembership[srcNode.ID]
	dstMesh := data.MeshMembership[dstNode.ID]
	return meshSource.CanReach(srcMesh, dstMesh, 0)
}
