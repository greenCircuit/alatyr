package store

import (
	"context"
	"log/slog"

	"graph/internal/config"
	"graph/internal/logging"
	"graph/internal/mesh"
	"graph/internal/models"
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
	return  issues
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
				if srcNode.Type == models.NodeTypeNamespace || dstNode.Type == models.NodeTypeNamespace {
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
					issues = append(issues, models.Issue{
						Type:            conflictType,
						Message:         blockEngine + " blocks a permitted path",
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
