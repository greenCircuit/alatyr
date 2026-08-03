package k8spolicy

import (
	"graph/internal/models"
	"graph/internal/policy"
	"graph/internal/utils"

	networkingv1 "k8s.io/api/networking/v1"
)


// generatePolicyStatusAssignment returns the per-workload PolicyStatus and
// per-workload list of selecting policies (NodePolicies). Both keyed by
// workload ID. PolicyStatus is the boolean digest consumed by status-key
// derivation; NodePolicies preserves the policy refs so the detail panel can
// show "selected by NetworkPolicy X" even when the policy emits zero rules.
func generatePolicyStatusAssignment(nodes []models.WorkloadNode, policies []*networkingv1.NetworkPolicy) (map[string]models.PolicyStatus, map[string][]models.PolicyRef) {
	statuses := map[string]models.PolicyStatus{}
	nodePolicies := map[string][]models.PolicyRef{}
	for _, node := range nodes {
		matchPolicies := getNodePolicies(node, policies)
		statuses[node.ID] = buildPolicyStatus(matchPolicies)

		// Namespace node shows every policy in its ns (any policy here
		// governs *some* workload in this ns). Workload nodes show only
		// policies that select them. Status derivation stays strict above.
		panelPolicies := matchPolicies
		if node.Type == models.NodeTypeNamespace {
			panelPolicies = policies
		}
		if len(panelPolicies) == 0 {
			continue
		}
		refs := make([]models.PolicyRef, 0, len(panelPolicies))
		for _, networkPolicy := range panelPolicies {
			refs = append(refs, models.PolicyRef{
				Source:    sourceName,
				Name:      networkPolicy.Name,
				Namespace: networkPolicy.Namespace,
				Action:    "allow", // k8s NetworkPolicies are always allow-style (default-deny is structural)
				Direction: networkPolicyDirection(networkPolicy),
				CreatedAt: creationTime(networkPolicy),
			})
		}
		nodePolicies[node.ID] = refs
	}
	return statuses, nodePolicies
}

// networkPolicyDirection derives the effective direction string from a
// NetworkPolicy's PolicyTypes. Empty PolicyTypes → ingress implied (and
// egress added when egress rules are present), per k8s spec.
func networkPolicyDirection(networkPolicy *networkingv1.NetworkPolicy) models.Direction {
	var hasIngress, hasEgress bool
	for _, policyType := range networkPolicy.Spec.PolicyTypes {
		if policyType == networkingv1.PolicyTypeIngress {
			hasIngress = true
		}
		if policyType == networkingv1.PolicyTypeEgress {
			hasEgress = true
		}
	}
	if len(networkPolicy.Spec.PolicyTypes) == 0 {
		hasIngress = true
		if len(networkPolicy.Spec.Egress) > 0 {
			hasEgress = true
		}
	}
	if hasIngress && hasEgress {
		return models.DirectionBoth
	}
	if hasEgress {
		return models.DirectionEgress
	}
	return models.DirectionIngress
}

// GetNodePolicies returns all NetworkPolicies that select the given workload node
// via PodSelector. Namespace nodes only match catch-all selectors.
func getNodePolicies(node models.WorkloadNode, policies []*networkingv1.NetworkPolicy) []*networkingv1.NetworkPolicy {
	var matches []*networkingv1.NetworkPolicy
	for _, networkPolicy := range policies {
		podSelector := networkPolicy.Spec.PodSelector
		catchAll := isCatchAll(podSelector.MatchLabels, len(podSelector.MatchExpressions))
		if node.Type == models.NodeTypeNamespace {
			if catchAll {
				matches = append(matches, networkPolicy)
			}
			continue
		}
		if utils.LabelsMatch(podSelector.MatchLabels, node.Labels) {
			matches = append(matches, networkPolicy)
		}
	}
	return matches
}



// buildPolicyStatus accumulates raw policy signals for a single workload from
// the NetworkPolicies that select it. Returned PolicyStatus is consumed by
// status-key derivation. Assumes every namespace has the auto-injected
func buildPolicyStatus(policies []*networkingv1.NetworkPolicy) models.PolicyStatus {
	var policyStatus models.PolicyStatus
	for _, networkPolicy := range policies {
		for _, policyType := range networkPolicy.Spec.PolicyTypes {
			if policyType == networkingv1.PolicyTypeEgress {
				policyStatus.EgressLocked = true
			}
			if policyType == networkingv1.PolicyTypeIngress {
				policyStatus.IngressLocked = true
			}
		}
		// when PolicyTypes unset: ingress always locked; egress locked only if egress rules present
		if len(networkPolicy.Spec.PolicyTypes) == 0 {
			policyStatus.IngressLocked = true
			if len(networkPolicy.Spec.Egress) > 0 {
				policyStatus.EgressLocked = true
			}
		}

		for _, rule := range networkPolicy.Spec.Egress {
			for _, peer := range rule.To {
				if peer.NamespaceSelector != nil && networkPolicy.Namespace != peer.NamespaceSelector.MatchLabels["kubernetes.io/metadata.name"] {
					policyStatus.CrossNS = true
				}
				if peer.IPBlock != nil {
					if policy.IsIpBlockInternetAccess(*peer.IPBlock) {
						policyStatus.InternetEgress = true
					}
					if policy.IsIpBlockApiServerAccess(*peer.IPBlock) {
						policyStatus.ApiServerEgress = true
					}
					if policy.IsIpBlockLanAccess(*peer.IPBlock) {
						policyStatus.LanEgress = true
					}
				}
				podCatchAll := peer.PodSelector == nil || isCatchAll(peer.PodSelector.MatchLabels, len(peer.PodSelector.MatchExpressions))
				if peer.IPBlock == nil && podCatchAll &&
					(peer.NamespaceSelector == nil || peer.NamespaceSelector.MatchLabels["kubernetes.io/metadata.name"] == networkPolicy.Namespace) {
					policyStatus.InnerNsEgress = true
				}
			}
		}

		for _, rule := range networkPolicy.Spec.Ingress {
			for _, peer := range rule.From {
				if peer.NamespaceSelector != nil && networkPolicy.Namespace != peer.NamespaceSelector.MatchLabels["kubernetes.io/metadata.name"] {
					policyStatus.CrossNS = true
				}
				if peer.IPBlock != nil {
					if policy.IsIpBlockInternetAccess(*peer.IPBlock) {
						policyStatus.InternetIngress = true
					}
					if policy.IsIpBlockLanAccess(*peer.IPBlock) {
						policyStatus.LanIngress = true
					}
				}
				podCatchAll := peer.PodSelector == nil || isCatchAll(peer.PodSelector.MatchLabels, len(peer.PodSelector.MatchExpressions))
				if peer.IPBlock == nil && podCatchAll &&
					(peer.NamespaceSelector == nil || peer.NamespaceSelector.MatchLabels["kubernetes.io/metadata.name"] == networkPolicy.Namespace) {
					policyStatus.InnerNsIngress = true
				}
			}
		}
	}
	return policyStatus
}

