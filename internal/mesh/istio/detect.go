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
	for engineName, policyEngine := range nodePolices {
		// id of nodes dest if they are missing port for istio ingress
		missingPortIn := make(map[string]bool)
		missingPortOut := make(map[string]bool)
		// id of nodes dest if they have access to istio ingress
		workingIngressDst := make(map[string]bool)
		workingEgressDst := make(map[string]bool)
		for _, rule := range policyEngine.Rules {
			// checking allow ingress
			if rule.Direction == models.DirectionIngress && rule.Action == models.ActionAllow {
				// no port defined, 0 is default value
				if rule.Port.Port == 0 || rule.Port.Port == ZtunnelHBONEPort {
					workingIngressDst[rule.DstID] = true
					// remove from missing list because with port 0, all ports are open
					_, ok := missingPortIn[rule.DstID]
					if ok {
						delete(missingPortIn, rule.DstID)
					} else {
						workingIngressDst[rule.DstID] = true
					}
					// port defined	that is not allow
				} else {
					_, ok := workingIngressDst[rule.DstID]
					// destination is not marked as able to reach
					if !ok {
						if rule.Port.Port != ZtunnelHBONEPort {
							missingPortIn[rule.DstID] = true
						}
					}

				}
			}
			if rule.Direction == models.DirectionEgress && rule.Action == models.ActionAllow {
				// no port defined, 0 is default value
				if rule.Port.Port == 0 || rule.Port.Port == ZtunnelHBONEPort {
					workingEgressDst[rule.DstID] = true
					// remove from missing list because with port 0, all ports are open
					_, ok := missingPortOut[rule.DstID]
					if ok {
						delete(missingPortOut, rule.DstID)
					} else {
						workingEgressDst[rule.DstID] = true
					}
					// port defined	that is not allow
				} else {
					_, ok := workingEgressDst[rule.DstID]
					// destination is not marked as able to reach
					if !ok {
						if rule.Port.Port != ZtunnelHBONEPort {
							missingPortOut[rule.DstID] = true
						}
					}

				}
			}
		}
		// finished for loop all policies for an engine
		// going over maps of all remaining misconfigured to find what rule associated with this id and policy engine
		for ruleId := range missingPortIn{
			for _, rule := range policyEngine.Rules {
				if rule.DstID == ruleId {
					for _, policyRef :=range rule.Contributors {
						errMsg := fmt.Sprintf("Missing ingress rule for policy engine: %s, name: %s, ns: %s need add port %d to ingress rule for istio ambient to work", engineName, policyRef.Name, policyRef.Namespace, ZtunnelHBONEPort)
						errors = append(errors, errMsg)
					}
				}
			}
		}

		for ruleId := range missingPortOut{
			for _, rule := range policyEngine.Rules {
				if rule.DstID == ruleId {
					for _, policyRef :=range rule.Contributors {
						errMsg := fmt.Sprintf("Missing egress rule for policy engine: %s, name: %s, ns: %s need add port %d to ingress rule for istio ambient to work", engineName, policyRef.Name, policyRef.Namespace, ZtunnelHBONEPort)
						errors = append(errors, errMsg)
					}
				}
			}
		}

	}
	return errors
}
