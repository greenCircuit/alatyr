package istio

import (
	"context"
	"fmt"

	"graph/internal/models"
)

// Membership: workload-level dataplane-mode label overrides ns label.
// Waypoint binding TODO (see ADR 0003).
func (s *source) Membership(workload models.WorkloadNode, nsLabels map[string]string) *models.MeshMembership {
	inMesh := inAmbientMesh(workload.Labels, nsLabels)
	m := &models.MeshMembership{InMesh: inMesh}
	if inMesh {
		m.Provider = SourceName
		m.Mode = "ambient"
	}
	if workload.Namespace == RootNamespace || workload.Namespace == IngressNamespace {
		m.Provider = SourceName
		m.Mode = "ambient"
	}

	return m
}

// ResolveMtls walks PA precedence (workload → ns → mesh). Skips the fetch
// and returns MeshDisable when the workload is not enrolled.
func (s *source) ResolveMtls(ctx context.Context, workload models.WorkloadNode, nsLabels map[string]string) (*models.MtlsState, error) {
	if !inAmbientMesh(workload.Labels, nsLabels) {
		return &models.MtlsState{Verdict: models.MeshDisable}, nil
	}

	nsPAs, err := s.client.GetPeerAuthentications(workload.Namespace)
	if err != nil {
		return nil, err
	}
	rootPAs, err := s.client.GetPeerAuthentications(RootNamespace)
	if err != nil {
		return nil, err
	}
	return resolveMtls(workload, nsPAs, rootPAs), nil
}


// CanReach denies when dst requires STRICT mTLS on the queried port and src
// cannot speak mTLS. portLevelMtls override wins over the workload verdict;
// port=0 means no port specified, verdict alone decides.
func (s *source) CanReach(srcMesh models.MeshMembership, dstMesh models.MeshMembership, port uint32) models.MeshVerdict {
	// Mtls is nil until BuildMeshMembership stamps it — treat as not-strict
	// rather than deref.
	dstMode := models.MeshUnset
	if dstMesh.InMesh && dstMesh.Mtls != nil {
		dstMode = dstMesh.Mtls.Verdict
		if port != 0 {
			if override, ok := dstMesh.Mtls.PortOverrides[port]; ok {
				dstMode = override
			}
		}
	}
	dstStrict := dstMode == models.MeshStrict
	if !dstStrict {
		return models.MeshVerdict{Verdict: "allow", Reason: "mesh permits"}
	}
	if !srcMesh.InMesh {
		return models.MeshVerdict{Verdict: "deny", Reason: "dst requires STRICT mTLS but src is not in mesh"}
	}
	if srcMesh.Mtls != nil && srcMesh.Mtls.Verdict == models.MeshDisable {
		return models.MeshVerdict{Verdict: "deny", Reason: "dst requires STRICT mTLS but src mTLS is disabled"}
	}
	return models.MeshVerdict{Verdict: "allow", Reason: "mesh permits"}
}

// hboneRuleEntry keeps enough of a flagged rule to reconstruct the Issue after
// the walk finishes (engine attribution + contributor policy + direction).
type hboneRuleEntry struct {
	engine string
	rule   models.NodeRule
}

// hboneCoverage tracks whether an engine's ruleset already opens HBONE
// globally (allow-all peers) per direction. When true for a direction, no
// per-policy flag is needed on that side.
type hboneCoverage struct {
	ingress bool
	egress  bool
}

func (c hboneCoverage) covers(direction models.Direction) bool {
	if direction == models.DirectionIngress {
		return c.ingress
	}
	return c.egress
}

// detectGlobalHBONEOpen scans an engine's rule set for allow-any-peer allow
// rules that admit port 15008 (or leave ports unrestricted). One such rule
// per direction means HBONE is globally reachable within the engine, so
// per-policy flags on that side would be false positives.
// Allow-any-peer surfaces as an empty peer id on NodeRule (empty DstID for
// egress, empty SrcID for ingress) — matches how CoverageAllowAll rules land
// after the Rule → NodeRule conversion.
func detectGlobalHBONEOpen(rules []models.NodeRule) hboneCoverage {
	var coverage hboneCoverage
	for _, rule := range rules {
		if !ruleGloballyAdmitsHBONE(rule) {
			continue
		}
		if rule.Direction == models.DirectionIngress {
			coverage.ingress = true
		}
		if rule.Direction == models.DirectionEgress {
			coverage.egress = true
		}
	}
	return coverage
}

// ruleGloballyAdmitsHBONE returns true when the rule is an allow-all-peer
// stanza that admits HBONE — either lists 15008 or leaves ports empty
// (k8s NetworkPolicy semantics: empty ports = every port).
func ruleGloballyAdmitsHBONE(rule models.NodeRule) bool {
	if rule.Action != models.ActionAllow {
		return false
	}
	peer := rule.DstID
	if rule.Direction == models.DirectionIngress {
		peer = rule.SrcID
	}
	if peer != "" {
		return false
	}
	if len(rule.Ports) == 0 {
		return true
	}
	for _, port := range rule.Ports {
		if port.Port == ZtunnelHBONEPort {
			return true
		}
	}
	return false
}

// ValidateExternalRules flags ambient workloads whose other-engine allow rules
// restrict ports without including the ztunnel HBONE port. Returns one Issue
// per offending (policy, direction) — dedup key: engine|dst|policy name|ns.
func ValidateExternalRules(workload models.WorkloadNode, cache *models.Cache, membership *models.MeshMembership, nodePolicies map[string]models.NodeInfo) []models.Issue {
	if membership == nil || !membership.InMesh || membership.Mode != "ambient" {
		return nil
	}
	// all mismanaged policies will be stored here and will be deleted if correct one are found
	seenInErr := make(map[string]hboneRuleEntry)
	seenOutErr := make(map[string]hboneRuleEntry)

	for engine, policyEngine := range nodePolicies {
		// k8s NetworkPolicies union within an engine, so an allow-all-peer
		// stanza opening HBONE covers every other stanza in the same
		// direction. Skip per-policy flagging when that engine-wide open
		// exists — otherwise a restricted-peer stanza (e.g. app=ledger-db
		// on 5432) gets false-flagged even though a sibling stanza already
		// opens 15008 to any peer.
		globalHBONE := detectGlobalHBONEOpen(policyEngine.Rules)
		for _, rule := range policyEngine.Rules {
			if globalHBONE.covers(rule.Direction) {
				continue
			}
			// HBONE (15008) only applies when the other end is a mesh peer.
			// External CIDR peers (0.0.0.0/0) and non-ambient workloads never
			// tunnel, so restricting their ports is correct — don't flag them.
			// k8s ingress rules swap: SrcID holds the peer, DstID the
			// protected workload itself (see k8spolicy/buildRules.go).
			peerID, peerNamespace := rule.DstID, rule.DstNamespace
			if rule.Direction == models.DirectionIngress {
				peerID, peerNamespace = rule.SrcID, rule.SrcNamespace
			}
			peerWorkload, peerNsLabels := cache.Workload(peerID, peerNamespace)
			if peerWorkload == nil || !participatesInMesh(*peerWorkload, peerNsLabels) {
				continue
			}
			ruleKey := fmt.Sprintf("%s|%s|%s|%s", engine, peerID, rule.Contributor.Name, rule.Contributor.Namespace)
			if rule.Direction == models.DirectionIngress && rule.Action == models.ActionAllow {
				missingPort := true
				for _, port := range rule.Ports {
					_, ok := seenInErr[ruleKey]
					if port.Port == ZtunnelHBONEPort || len(rule.Ports) == 0 {
						missingPort = false
						if ok {
							delete(seenInErr, ruleKey)
						}
					}
					if missingPort && !ok {
						seenInErr[ruleKey] = hboneRuleEntry{engine: engine, rule: rule}
					}
				}
			}

			if rule.Direction == models.DirectionEgress && rule.Action == models.ActionAllow {
				missingPort := true
				for _, port := range rule.Ports {
					_, ok := seenOutErr[ruleKey]
					if port.Port == ZtunnelHBONEPort || len(rule.Ports) == 0 {
						missingPort = false
						if ok {
							delete(seenOutErr, ruleKey)
						}
					}

					if missingPort && !ok {
						seenOutErr[ruleKey] = hboneRuleEntry{engine: engine, rule: rule}
					}
				}
			}
		}
	}

	var issues []models.Issue
	for _, entry := range seenInErr {
		issues = append(issues, hboneIssue(workload, entry, models.DirectionIngress))
	}
	for _, entry := range seenOutErr {
		issues = append(issues, hboneIssue(workload, entry, models.DirectionEgress))
	}
	return issues
}

// hboneIssue materializes one MeshTransportBlocked Issue from a flagged rule.
// Node = the ambient workload the policy applies to; culprits stamp the
// direction so the UI's CulpritActions row lights up the correct side.
func hboneIssue(workload models.WorkloadNode, entry hboneRuleEntry, direction models.Direction) models.Issue {
	sideLabel := "ingress"
	if direction == models.DirectionEgress {
		sideLabel = "egress"
	}
	issue := models.Issue{
		Type:    models.MeshTransportBlocked,
		Message: fmt.Sprintf("Policy does not allow ztunnel HBONE port %d; ambient %s traffic is blocked", ZtunnelHBONEPort, sideLabel),
		Engine: entry.engine,
		Node:   &workload,
	}
	culprit := []models.PolicyRef{entry.rule.Contributor}
	if direction == models.DirectionIngress {
		issue.IngressCulprits = culprit
	} else {
		issue.EgressCulprits = culprit
	}
	return issue
}
