package store

import (
	"context"
	"log/slog"
	"strings"

	"alatyr/internal/config"
	"alatyr/internal/logging"
	"alatyr/internal/mesh"
	"alatyr/internal/models"
	"alatyr/internal/policy"
	"alatyr/internal/utils"
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
	// dns workload node
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
	var issues []models.Issue

	// walk every cached workload as src. Per-engine egress verdict via
	// collectToSide — Permitted or NoOpinion skip, everything else flags.
	// walk ns and their nodes in ns
	for _, srcNsIdx := range data.NsIndex {
		srcNsID := ""
		if srcNsIdx.NSNode != nil {
			srcNsID = srcNsIdx.NSNode.ID
		}
		for _, node := range srcNsIdx.Workloads {
			src := &node
			if node.Type == models.NodeTypeNamespace || node.ID == dnsNodeID {
				continue
			}

			for engineName, eval := range data.EvaluationResults {
				// blanket 53 on ns or workload clears only this engine — engines
				// intersect, so another engine can still block DNS
				if allowsFullPortAccess(eval.NodeRules[srcNsID], 53) || allowsFullPortAccess(eval.NodeRules[src.ID], 53) {
					continue
				}

				verdict := collectToSide(eval.NodeRules[srcNsID], eval.NodeRules[src.ID], dnsNsID, dnsNodeID, data.WorkloadByID)

				if verdict.Reason == ReasonNoOpinion {
					continue
				}
				if verdict.Reason == ReasonPermitted {
					if verdict.AllPorts {
						continue
					}

					// checking if ports match
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

				// default mgs if all checks failed
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

// blanket egress allow (no peer named) carrying targetPort, within one engine's rules
func allowsFullPortAccess(rules models.NodeRules, targetPort int) bool {
	for _, rule := range rules.Egress.Allow {
		// coverage, not empty DstID — deny-all/unenforced markers are empty too
		if rule.Coverage != models.CoverageAllowAll {
			continue
		}
		for _, port := range rule.Ports {
			if port.Port == targetPort || (port.EndPort != 0 && port.Port <= targetPort && targetPort <= port.EndPort) {
				return true
			}
		}
	}
	return false
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
// do inline in one for look so don't do for loop for every issue type
func PolicyIssues(ctx context.Context, data *models.Cache, meshSource mesh.MeshSource) []models.Issue {
	var issues []models.Issue
	checkedPolicy := make(map[string]bool) // srcId-dstId already evaluated
	prober := newReachabilityProber(ctx, data, nil)

	for _, eval := range data.EvaluationResults {
		for _, nsRules := range eval.AllowByNs {
			for _, rule := range nsRules {
				if rule.Action != models.ActionAllow {
					continue
				}

				//checking only unique pairs
				checkIndex := rule.SrcID + "-" + rule.DstID
				if checkedPolicy[checkIndex] {
					continue
				}
				checkedPolicy[checkIndex] = true

				// Unresolved endpoints 
				srcNode, srcOk := data.WorkloadByID[rule.SrcID]
				if !srcOk {
					continue
				}
				dstNode, dstOk := data.WorkloadByID[rule.DstID]
				if !dstOk {
					continue
				}

				// mtls mismatch
				// Skip ns nodes: membership is per-workload; the zero membership would false-positive a namespace endpoint as "not in mesh".
				if meshSource != nil && srcNode.Type != models.NodeTypeNamespace && dstNode.Type != models.NodeTypeNamespace {
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

				// checking only deny routes now
				verdict := prober.probe(rule.SrcID, srcNode.Namespace, rule.DstID, dstNode.Namespace)
				if verdict.Verdict != "deny" {
					continue
				}

				// check what policy denied
				for blockEngine, engineVerdict := range verdict.Engines {
					if engineVerdict.Status != "deny" {
						continue
					}

					// two tiers composing, not a conflict. per engine — one engine
					// layering says nothing about the next, so continue not break
					if isLayered, find := isLayeredPolicy(prober, blockEngine, srcNode, dstNode); isLayered {
						issues = append(issues, models.Issue{
							Type:            models.IssuesPolicyLayer,
							Message:         find.message,
							Engine:          blockEngine,
							Src:             &srcNode,
							Dst:             &dstNode,
							IngressCulprits: engineVerdict.Ingress.Culprits,
							EgressCulprits:  engineVerdict.Egress.Culprits,
							IngressReason:   engineVerdict.Ingress.Reason,
							EgressReason:    engineVerdict.Egress.Reason,
							IngressAllowed:  find.ingressAllowed,
							EgressAllowed:   find.egressAllowed,
						})
						continue
					}

					ingressAllowed := dedupContributors(engineVerdict.Ingress.AllowMatches)
					egressAllowed := dedupContributors(engineVerdict.Egress.AllowMatches)

					issueType := models.PolicyConflicts
					message := blockEngine + " blocks a permitted path"
					egressPeerOf := func(nodeRule models.NodeRule) string { return nodeRule.DstID }
					ingressPeerOf := func(nodeRule models.NodeRule) string { return nodeRule.SrcID }

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
							" this node represents, confirm the narrower mask is deliberate least-privilege, not a typo"

						for otherEngine, otherVerdict := range verdict.Engines {
							if otherEngine == blockEngine {
								continue
							}
							ingressAllowed = mergePolicyRefs(ingressAllowed, dedupContributors(otherVerdict.Ingress.AllowMatches))
							egressAllowed = mergePolicyRefs(egressAllowed, dedupContributors(otherVerdict.Egress.AllowMatches))
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
