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

// CanReach denies when dst requires STRICT mTLS and src cannot speak mTLS
// (not enrolled or PA DISABLE). port=0 means workload-level Verdict.
// TODO(perf): re-resolves both sides — callers that already hold MtlsState
// pay 2 redundant fetches. Add CanReachFromStates variant.
func (s *source) CanReach(ctx context.Context, srcWorkload, dstWorkload models.WorkloadNode, srcNsLabels, dstNsLabels map[string]string, port uint32) models.MeshVerdict {
	srcMtls, srcErr := s.ResolveMtls(ctx, srcWorkload, srcNsLabels)
	dstMtls, dstErr := s.ResolveMtls(ctx, dstWorkload, dstNsLabels)
	if srcErr != nil || dstErr != nil || srcMtls == nil || dstMtls == nil {
		return models.MeshVerdict{Verdict: "unknown", Reason: "could not resolve PAs"}
	}

	dstEffective := effectiveMode(dstMtls, port)
	srcEffective := effectiveMode(srcMtls, 0)

	if dstEffective == models.MeshStrict && srcEffective == models.MeshDisable {
		v := models.MeshVerdict{
			Verdict: "deny",
			Reason:  "dst requires STRICT mTLS; src cannot speak mTLS (not enrolled or PA DISABLE)",
		}
		if dstMtls.EffectiveSource.Name != "" {
			ref := dstMtls.EffectiveSource
			v.EffectiveSource = &ref
		}
		return v
	}
	return models.MeshVerdict{Verdict: "allow", Reason: "mesh permits"}
}

// ValidateExternalRule flags ambient workloads whose other-engine ingress
// rules restrict ports without allowing the ztunnel HBONE port.
func ValidateExternalRules(membership *models.MeshMembership, nodePolices map[string]models.NodeInfo) []string {
	var errors []string
	if !membership.InMesh || membership.Mode != "ambient" {
		return errors
	}
	// all mismanaged policies will be stored here and will be deleted if correct one are found
	seenInErr := make(map[string]models.NodeRule)
	seenOutErr := make(map[string]models.NodeRule)

	for engine, policyEngine := range nodePolices {
		for _, rule := range policyEngine.Rules {
			ruleKey := fmt.Sprintf("%s|%s|%s|%s", engine, rule.DstID, rule.Contributor.Name, rule.Contributor.Namespace)                      
			// need to check only if ports are defined
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
							seenInErr[ruleKey] = rule
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
							seenOutErr[ruleKey] = rule
						}
					}
				}

			}
		}

	// found all errors at this point need to build msgs
	for _, rule := range seenInErr {
		errMsg := fmt.Sprintf("Missing ingress rule for policy engine: %s, name: %s, ns: %s need add port %d to ingress rule for istio ambient to work", rule.Contributor.Source, rule.Contributor.Name, rule.Contributor.Namespace, ZtunnelHBONEPort)
		errors = append(errors, errMsg)
	}

	for _, rule := range seenOutErr {
		errMsg := fmt.Sprintf("Missing egress rule for policy engine: %s, name: %s, ns: %s need add port %d to egress rule for istio ambient to work", rule.Contributor.Source, rule.Contributor.Name, rule.Contributor.Namespace, ZtunnelHBONEPort)
		errors = append(errors, errMsg)
	}

	return errors
}
