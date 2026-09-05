package calico

import (
	"alatyr/internal/models"
	"alatyr/internal/policy"

	networkingv1 "k8s.io/api/networking/v1"
)

func policyRefs(selecting []globalPolicy) []models.PolicyRef {
	refs := make([]models.PolicyRef, 0, len(selecting))
	for _, gp := range selecting {
		refs = append(refs, gp.ref)
	}
	return refs
}

// buildPolicyStatus — derive status from the resolved walk, not a raw scan.
// Badges show intent: a direction locks once any selecting policy governs it;
// reach dimensions flip only on explicit Allow edges.
func buildPolicyStatus(selecting []globalPolicy, egressEdges, ingressEdges []resolvedEdge) models.PolicyStatus {
	var status models.PolicyStatus
	for _, gp := range selecting {
		if gp.doesEgress {
			status.EgressLocked = true
		}
		if gp.doesIngress {
			status.IngressLocked = true
		}
	}
	classifyReach(egressEdges, models.DirectionEgress, &status)
	classifyReach(ingressEdges, models.DirectionIngress, &status)
	return status
}

// classifyReach — flip internet/LAN dimensions from Allow-edge destinations.
func classifyReach(edges []resolvedEdge, direction models.Direction, status *models.PolicyStatus) {
	for _, edge := range edges {
		if edge.action != models.ActionAllow || edge.peer.kind != peerCIDR {
			continue
		}
		block := networkingv1.IPBlock{CIDR: edge.peer.cidr}
		internet := policy.IsIpBlockInternetAccess(block)
		lan := policy.IsIpBlockLanAccess(block)
		if direction == models.DirectionEgress {
			status.InternetEgress = status.InternetEgress || internet
			status.LanEgress = status.LanEgress || lan
		} else {
			status.InternetIngress = status.InternetIngress || internet
			status.LanIngress = status.LanIngress || lan
		}
	}
}
