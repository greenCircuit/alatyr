package k8spolicy

import (
	"graph/internal/models"
	"graph/internal/policy"

	networkingv1 "k8s.io/api/networking/v1"
)

// BuildStatusKeys computes status badges for a node given all policies that select it.
// NetworkPolicy rules are additive (OR), so all policies are scanned before deciding.
// Assumes every namespace has the auto-injected `kubernetes.io/metadata.name` label
// (k8s 1.21+) — cross-NS detection uses it directly.
func BuildStatusKeys(policies []networkingv1.NetworkPolicy) []models.StatusKey {

	var result []models.StatusKey
	var (
		egressLocked       bool
		ingressLocked      bool
		hasInternetEgress  bool
		hasInternetIngress bool
		hasLanEgress       bool
		hasLanIngress      bool
		hasApiServerEgress bool
		hasCrossNS         bool
		innerNsEgress      bool
		innerNsIngress     bool
	)

	for _, networkPolicy := range policies {
		for _, policyType := range networkPolicy.Spec.PolicyTypes {
			if policyType == networkingv1.PolicyTypeEgress {
				egressLocked = true
			}
			if policyType == networkingv1.PolicyTypeIngress {
				ingressLocked = true
			}
		}
		// when PolicyTypes unset: ingress always locked; egress locked only if egress rules present
		if len(networkPolicy.Spec.PolicyTypes) == 0 {
			ingressLocked = true
			if len(networkPolicy.Spec.Egress) > 0 {
				egressLocked = true
			}
		}

		for _, rule := range networkPolicy.Spec.Egress {
			for _, peer := range rule.To {
				if peer.NamespaceSelector != nil && networkPolicy.Namespace != peer.NamespaceSelector.MatchLabels["kubernetes.io/metadata.name"] {
					hasCrossNS = true
				}
				if peer.IPBlock != nil {
					if policy.IsIpBlockInternetAccess(*peer.IPBlock) {
						hasInternetEgress = true
					}
					if policy.IsIpBlockApiServerAccess(*peer.IPBlock) {
						hasApiServerEgress = true
					}
					if policy.IsIpBlockLanAccess(*peer.IPBlock) {
						hasLanEgress = true
					}
				}
				podCatchAll := peer.PodSelector == nil || isCatchAll(peer.PodSelector.MatchLabels, len(peer.PodSelector.MatchExpressions))
				if peer.IPBlock == nil && podCatchAll &&
					(peer.NamespaceSelector == nil || peer.NamespaceSelector.MatchLabels["kubernetes.io/metadata.name"] == networkPolicy.Namespace) {
					innerNsEgress = true
				}
			}
		}

		for _, rule := range networkPolicy.Spec.Ingress {
			for _, peer := range rule.From {
				if peer.NamespaceSelector != nil && networkPolicy.Namespace != peer.NamespaceSelector.MatchLabels["kubernetes.io/metadata.name"] {
					hasCrossNS = true
				}
				if peer.IPBlock != nil {
					if policy.IsIpBlockInternetAccess(*peer.IPBlock) {
						hasInternetIngress = true
					}
					if policy.IsIpBlockLanAccess(*peer.IPBlock) {
						hasLanIngress = true
					}
				}
				podCatchAll := peer.PodSelector == nil || isCatchAll(peer.PodSelector.MatchLabels, len(peer.PodSelector.MatchExpressions))
				if peer.IPBlock == nil && podCatchAll &&
					(peer.NamespaceSelector == nil || peer.NamespaceSelector.MatchLabels["kubernetes.io/metadata.name"] == networkPolicy.Namespace) {
					innerNsIngress = true
				}
			}
		}
	}

	internetEgress := !egressLocked || hasInternetEgress
	internetIngress := !ingressLocked || hasInternetIngress
	if internetEgress && internetIngress {
		result = append(result, models.StatusInternetFull)
	} else if internetEgress {
		result = append(result, models.StatusInternetEgress)
	} else if internetIngress {
		result = append(result, models.StatusInternetIngress)
	}

	if hasLanEgress && hasLanIngress {
		result = append(result, models.StatusLanFull)
	} else if hasLanEgress {
		result = append(result, models.StatusLanEgress)
	} else if hasLanIngress {
		result = append(result, models.StatusLanIngress)
	}

	if hasApiServerEgress {
		result = append(result, models.StatusApiServerEgress)
	}

	// air-gapped means no path out at all: locked on both sides and no internet,
	// LAN, or API-server escape hatches.
	if egressLocked && ingressLocked &&
		!hasInternetEgress && !hasInternetIngress &&
		!hasLanEgress && !hasLanIngress &&
		!hasApiServerEgress {
		result = append(result, models.StatusIsolated)
	}
	if hasCrossNS {
		result = append(result, models.StatusCrossNamespace)
	}
	if innerNsEgress && innerNsIngress {
		result = append(result, models.StatusNamespaceFull)
	} else if innerNsEgress {
		result = append(result, models.StatusNamespaceEgress)
	} else if innerNsIngress {
		result = append(result, models.StatusNamespaceIngress)
	}

	return result
}

func GetStatusKeys() []models.StatusKey {
	return []models.StatusKey{
		models.StatusInternetIngress,
		models.StatusInternetEgress,
		models.StatusInternetFull,
		models.StatusLanIngress,
		models.StatusLanEgress,
		models.StatusLanFull,
		models.StatusApiServerEgress,
		models.StatusIsolated,
		models.StatusCrossNamespace,
		models.StatusNamespaceEgress,
		models.StatusNamespaceIngress,
		models.StatusNamespaceFull,
	}
}
