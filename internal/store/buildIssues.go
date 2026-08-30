package store

import (
	"context"
	"log/slog"
	"strings"

	"graph/internal/config"
	"graph/internal/logging"
	"graph/internal/mesh"
	"graph/internal/models"
	"graph/internal/policy"
	"graph/internal/utils"
)

// GetIssues runs every issue detector over the cache and returns the combined
// findings. Endpoint-facing aggregator.
func GetIssues(ctx context.Context, data *models.Cache, meshSource mesh.MeshSource) []models.Issue {
	var issues []models.Issue
	policyIssues := PolicyIssues(ctx, data, meshSource)
	dnsIssues := MissingDns(ctx, data)
	issues = append(issues, policyIssues...)
	issues = append(issues, dnsIssues...)
	// Mesh hygiene + transport-blocked issues were computed once during
	// PopulateCache and parked on the cache — no re-walk here.
	issues = append(issues, data.MeshIssues...)
	// Severity stamped once here, not per detector — detectors only decide
	// what fires, the type→severity table decides how loud it is.
	for index := range issues {
		issues[index].Severity = models.SeverityForType(issues[index].Type)
	}
	return issues
}


func MissingDns(ctx context.Context, data *models.Cache) []models.Issue {
	logger := logging.FromCtx(ctx)

	// resolve DNS target from config
	cfg := config.Get()
	dnsNsIdx, ok := data.NsIndex[cfg.DNSNamespace]
	if !ok {
		return nil
	}
	matches := utils.IndexLabelMatch(cfg.DNSLabels, dnsNsIdx.LabelIndex)
	if len(matches) == 0 {
		logger.Warn("missing dns: no workload matched DNS selector",
			slog.String("phase", "missing_dns"),
			slog.String("dns_ns", cfg.DNSNamespace),
		)
		return nil
	}
	dnsNode := matches[0]
	dnsNodeID := dnsNode.ID
	dnsNsID := ""
	if dnsNsIdx.NSNode != nil {
		dnsNsID = dnsNsIdx.NSNode.ID
	}

	// walk every cached workload as src. Per-engine egress verdict via
	// collectToSide — Permitted or NoOpinion skip, everything else flags.
	var issues []models.Issue
	for _, srcNsIdx := range data.NsIndex {
		srcNsID := ""
		if srcNsIdx.NSNode != nil {
			srcNsID = srcNsIdx.NSNode.ID
		}
		for i := range srcNsIdx.Workloads {
			src := &srcNsIdx.Workloads[i]
			if src.Type == models.NodeTypeNamespace || src.ID == dnsNodeID {
				continue
			}
			for engineName, eval := range data.EvaluationResults {
				verdict := collectToSide(eval.NodeRules[srcNsID], eval.NodeRules[src.ID], dnsNsID, dnsNodeID, data.WorkloadByID)
				// no policy governs egress — DNS trivially reachable, port check
				// would false-positive on the empty port set
				if verdict.Reason == ReasonNoOpinion {
					continue
				}
				if verdict.Reason == ReasonPermitted {
					if verdict.AllPorts {
						continue
					}
					// any protocol counts — flagging TCP-only 53 would
					// false-positive on TCP-DNS setups
					dnsPortAllowed := false
					for _, port := range verdict.Ports {
						if port.Port == 53 || (port.EndPort != 0 && port.Port <= 53 && 53 <= port.EndPort) {
							dnsPortAllowed = true
							break
						}
					}
					if dnsPortAllowed {
						continue
					}
					// permitted verdicts carry no culprits — the policies to edit
					// are the ones that allowed the path without port 53
					issues = append(issues, models.Issue{
						Type:           models.NoDNSEgress,
						Message:        "cluster DNS blocked: egress to DNS allowed but not on port 53",
						Engine:         engineName,
						Src:            src,
						Dst:            dnsNode,
						EgressCulprits: dedupContributors(verdict.AllowMatches),
						EgressReason:   verdict.Reason,
					})
					continue
				}
				issues = append(issues, models.Issue{
					Type:           models.NoDNSEgress,
					Message:        "cluster DNS blocked: " + string(verdict.Reason),
					Engine:         engineName,
					Src:            src,
					Dst:            dnsNode,
					EgressCulprits: verdict.Culprits,
					EgressReason:   verdict.Reason,
				})
			}
		}
	}
	return issues
}

// isInternalCidrAggregate reports whether a node is a synthetic CIDR node
// standing for the cluster's own pod or service range. Those aren't real
// endpoints — a ClusterIP is DNAT'd to a backend pod before policy runs — so an
// engine that names pods and namespaces still permits addresses inside the
// range without ever mentioning it.
func isInternalCidrAggregate(node models.WorkloadNode) bool {
	if node.Type != models.NodeTypeCIDR {
		return false
	}
	return node.CidrType == models.CIDRk8sPod || node.CidrType == models.CIDRk8sSvc
}

// subsetAllowCidr returns the CIDR this engine allowed strictly inside the range
// the peer node stands for, or "" when it named none. A /32 host allow under a
// /24 node means the engines disagree about scope, not about intent. Rules whose
// peer isn't a CIDR fall out on the CidrContains parse failure.
//
// The default route is excluded: 0.0.0.0/0 contains every CIDR, so any unrelated
// allow would read as a subset and every WAN block would stop being a conflict.
func subsetAllowCidr(peer models.WorkloadNode, unmatched []models.NodeRule, peerIDOf func(models.NodeRule) string) string {
	if peer.Type != models.NodeTypeCIDR || peer.CidrType == models.CIDRWan {
		return ""
	}
	aggregate := strings.TrimPrefix(peer.ID, models.CIDRIDPrefix)
	for _, nodeRule := range unmatched {
		inner := strings.TrimPrefix(peerIDOf(nodeRule), models.CIDRIDPrefix)
		if inner == aggregate {
			continue
		}
		if policy.CidrContains(aggregate, inner) {
			return inner
		}
	}
	return ""
}

// PolicyIssues flags policy conflicts: an ALLOW rule whose src→dst path is
// still blocked once every engine's rules + locks are intersected — one policy
// permits the edge while another denies or locks it out. Mesh runs in the same
// pair loop: transport-layer blocks emit as MeshConflicts alongside.
func PolicyIssues(ctx context.Context, data *models.Cache, meshSource mesh.MeshSource) []models.Issue {
	var issues []models.Issue
	checkedPolicy := make(map[string]bool) // srcId-dstId already evaluated
	prober := newReachabilityProber(data, nil)

	for _, eval := range data.EvaluationResults {
		for _, nsRules := range eval.AllowByNs {
			for _, rule := range nsRules {
				if rule.Action != models.ActionAllow {
					continue
				}
				checkIndex := rule.SrcID + "-" + rule.DstID
				if checkedPolicy[checkIndex] {
					continue
				}
				checkedPolicy[checkIndex] = true

				// Unresolved endpoints (CIDR dst, unloaded ns) aren't probeable
				// pairs — skip without logging, they're expected, not errors.
				srcNode, srcOk := data.WorkloadByID[rule.SrcID]
				if !srcOk {
					continue
				}
				dstNode, dstOk := data.WorkloadByID[rule.DstID]
				if !dstOk {
					continue
				}

				// Transport layer: this pair has an allow rule, so a mesh deny is
				// a conflict. checkedPolicy already dedups pairs — no second map.
				// Skip ns nodes: membership is per-workload; the zero membership
				// would false-positive a namespace endpoint as "not in mesh".
				if meshSource != nil &&
					srcNode.Type != models.NodeTypeNamespace && dstNode.Type != models.NodeTypeNamespace {
					meshReach := MeshReachabilityUsingNodes(data, meshSource, srcNode, dstNode)
					if meshReach.Verdict == "deny" {
						srcMesh := data.MeshMembership[srcNode.ID]
						dstMesh := data.MeshMembership[dstNode.ID]
						meshIssue := models.Issue{
							Type:          models.MeshConflicts,
							Message:       "mesh blocks a permitted path: " + meshReach.Reason,
							Engine:        meshSource.Name(),
							Src:           &srcNode,
							Dst:           &dstNode,
							SrcMembership: &srcMesh,
							DstMembership: &dstMesh,
						}
						// Source is "pa" (the manifest-kind dispatch key in
						// get-manifest.go), not the mesh engine name — the culprit
						// chip's YAML button needs to hit the PeerAuthentication
						// fetcher, not the AuthorizationPolicy one.
						if meshReach.EffectiveSource != nil {
							meshIssue.IngressCulprits = []models.PolicyRef{{
								Source:    "pa",
								Name:      meshReach.EffectiveSource.Name,
								Namespace: meshReach.EffectiveSource.Namespace,
							}}
						}
						issues = append(issues, meshIssue)
					}
				}

				verdict := prober.probe(ctx, rule.SrcID, srcNode.Namespace, rule.DstID, dstNode.Namespace)
				if verdict.Verdict != "deny" {
					continue
				}

				conflictType := models.PolicyConflicts
				var fineAllowed fineAllows
				// An ns-node endpoint blocks at namespace granularity while the
				// culprit's own pod-level clauses may still permit specific pods.
				// Classify from those fine-grained paths, not the coarse probe.
				switch {
				// Engines target different peer kinds: Calico names cluster ranges,
				// k8s/istio reach them through pod and namespace selectors. A blocker
				// with no rule for the pod/svc range is overlapping scope, not
				// disagreement — its ns allows DO permit addresses inside that range.
				//
				// Ordered ahead of the ns-node case on purpose. A CIDR node carries no
				// namespace, so classifyLayering's candidate join finds zero pairs and
				// classifyConflict reports a conflict it never actually verified.
				case isInternalCidrAggregate(srcNode) || isInternalCidrAggregate(dstNode):
					conflictType = models.IssuesPartial
				case srcNode.Type == models.NodeTypeNamespace || dstNode.Type == models.NodeTypeNamespace:
					conflictType, fineAllowed = classifyLayering(ctx, prober, verdict, srcNode.Namespace, dstNode.Namespace)
				}

				// A conflict is one engine permitting while another blocks. The
				// allow that surfaced this pair may live in a different engine than
				// the blocker (istio allows, k8s denies), so attribute each issue to
				// the engine that actually blocks — reading culprits off engineName
				// would pull them from the permitting engine and show none.
				for blockEngine, engineVerdict := range verdict.Engines {
					if engineVerdict.Status != "deny" {
						continue
					}
					// Partial issues also carry the layering evidence: the
					// policies that permitted the verified pod paths, MINUS this
					// row's own culprits — a baseline policy is both blocker and
					// fine tier, and echoing it as evidence is duplication. What
					// remains is the counterpart tier (the coarse ns-level allow).
					ingressAllowed := dedupContributors(engineVerdict.Ingress.AllowMatches)
					egressAllowed := dedupContributors(engineVerdict.Egress.AllowMatches)
					if conflictType == models.IssuesPartial {
						ingressAllowed = mergePolicyRefs(ingressAllowed,
							excludePolicyRefs(fineAllowed.ingress, engineVerdict.Ingress.Culprits))
						egressAllowed = mergePolicyRefs(egressAllowed,
							excludePolicyRefs(fineAllowed.egress, engineVerdict.Egress.Culprits))
					}
					// Type is per-engine from here: the scope check reads THIS
					// engine's unmatched allows, so a blocker that narrowed the range
					// is classified differently from one that never named it.
					issueType := conflictType
					message := blockEngine + " blocks a permitted path"
					egressPeerOf := func(nodeRule models.NodeRule) string { return nodeRule.DstID }
					ingressPeerOf := func(nodeRule models.NodeRule) string { return nodeRule.SrcID }
					switch {
					case isInternalCidrAggregate(srcNode) || isInternalCidrAggregate(dstNode):
						message = blockEngine + " has no rule naming this cluster range — it targets pods and namespaces, not CIDRs"
					default:
						aggregate := dstNode
						narrow := subsetAllowCidr(dstNode, engineVerdict.Egress.OtherAllowMatches, egressPeerOf)
						if narrow == "" {
							aggregate = srcNode
							narrow = subsetAllowCidr(srcNode, engineVerdict.Ingress.OtherAllowMatches, ingressPeerOf)
						}
						if narrow != "" {
							issueType = models.IssuesCidrScope
							message = blockEngine + " only allows " + narrow + " inside the " +
								strings.TrimPrefix(aggregate.ID, models.CIDRIDPrefix) +
								" this node represents — confirm the narrower mask is deliberate least-privilege, not a typo"
							// The policy granting the WHOLE range lives in a different
							// engine's verdict, so this row would name only the narrow
							// policy and leave the operator guessing what it disagrees
							// with. Pull the counterpart tier in as evidence.
							for otherEngine, otherVerdict := range verdict.Engines {
								if otherEngine == blockEngine {
									continue
								}
								ingressAllowed = mergePolicyRefs(ingressAllowed, dedupContributors(otherVerdict.Ingress.AllowMatches))
								egressAllowed = mergePolicyRefs(egressAllowed, dedupContributors(otherVerdict.Egress.AllowMatches))
							}
						}
					}
					issues = append(issues, models.Issue{
						Type:            issueType,
						Message:         message,
						Engine:          blockEngine,
						Src:             &srcNode,
						Dst:             &dstNode,
						IngressCulprits: engineVerdict.Ingress.Culprits,
						EgressCulprits:  engineVerdict.Egress.Culprits,
						IngressReason:   engineVerdict.Ingress.Reason,
						EgressReason:    engineVerdict.Egress.Reason,
						IngressAllowed:  ingressAllowed,
						EgressAllowed:   egressAllowed,
					})
				}
			}
		}
	}
	return issues
}
