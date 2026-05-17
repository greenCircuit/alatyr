package graph

import networkingv1 "k8s.io/api/networking/v1"

// buildStatusKeys computes status badges for a node given all policies that select it.
// NetworkPolicy rules are additive (OR), so all policies are scanned before deciding.
// Assumes every namespace has the auto-injected `kubernetes.io/metadata.name` label
// (k8s 1.21+) — cross-NS detection uses it directly.
func buildStatusKeys(policies []networkingv1.NetworkPolicy) []StatusKey {

	var result []StatusKey
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

	for _, policy := range policies {
		for _, pt := range policy.Spec.PolicyTypes {
			if pt == networkingv1.PolicyTypeEgress {
				egressLocked = true
			}
			if pt == networkingv1.PolicyTypeIngress {
				ingressLocked = true
			}
		}
		// when PolicyTypes unset: ingress always locked; egress locked only if egress rules present
		if len(policy.Spec.PolicyTypes) == 0 {
			ingressLocked = true
			if len(policy.Spec.Egress) > 0 {
				egressLocked = true
			}
		}

		for _, rule := range policy.Spec.Egress {
			for _, peer := range rule.To {
				if peer.NamespaceSelector != nil && policy.Namespace != peer.NamespaceSelector.MatchLabels["kubernetes.io/metadata.name"] {
					hasCrossNS = true
				}
				if peer.IPBlock != nil {
					if IsIpBlockInternetAccess(*peer.IPBlock) {
						hasInternetEgress = true
					}
					if IsIpBlockApiServerAccess(*peer.IPBlock) {
						hasApiServerEgress = true
					}
					if IsIpBlockLanAccess(*peer.IPBlock) {
						hasLanEgress = true
					}
				}
				podCatchAll := peer.PodSelector == nil || isCatchAll(peer.PodSelector.MatchLabels, len(peer.PodSelector.MatchExpressions))
				if peer.IPBlock == nil && podCatchAll &&
					(peer.NamespaceSelector == nil || peer.NamespaceSelector.MatchLabels["kubernetes.io/metadata.name"] == policy.Namespace) {
					innerNsEgress = true
				}
			}
		}

		for _, rule := range policy.Spec.Ingress {
			for _, peer := range rule.From {
				if peer.NamespaceSelector != nil && policy.Namespace != peer.NamespaceSelector.MatchLabels["kubernetes.io/metadata.name"] {
					hasCrossNS = true
				}
				if peer.IPBlock != nil {
					if IsIpBlockInternetAccess(*peer.IPBlock) {
						hasInternetIngress = true
					}
					if IsIpBlockLanAccess(*peer.IPBlock) {
						hasLanIngress = true
					}
				}
				podCatchAll := peer.PodSelector == nil || isCatchAll(peer.PodSelector.MatchLabels, len(peer.PodSelector.MatchExpressions))
				if peer.IPBlock == nil && podCatchAll &&
					(peer.NamespaceSelector == nil || peer.NamespaceSelector.MatchLabels["kubernetes.io/metadata.name"] == policy.Namespace) {
					innerNsIngress = true
				}
			}
		}
	}

	internetEgress := !egressLocked || hasInternetEgress
	internetIngress := !ingressLocked || hasInternetIngress
	if internetEgress && internetIngress {
		result = append(result, StatusInternetFull)
	} else if internetEgress {
		result = append(result, StatusInternetEgress)
	} else if internetIngress {
		result = append(result, StatusInternetIngress)
	}

	if hasLanEgress && hasLanIngress {
		result = append(result, StatusLanFull)
	} else if hasLanEgress {
		result = append(result, StatusLanEgress)
	} else if hasLanIngress {
		result = append(result, StatusLanIngress)
	}

	if hasApiServerEgress {
		result = append(result, StatusApiServerEgress)
	}

	// air-gapped means no path out at all: locked on both sides and no internet,
	// LAN, or API-server escape hatches.
	if egressLocked && ingressLocked &&
		!hasInternetEgress && !hasInternetIngress &&
		!hasLanEgress && !hasLanIngress &&
		!hasApiServerEgress {
		result = append(result, StatusIsolated)
	}
	if hasCrossNS {
		result = append(result, StatusCrossNamespace)
	}
	if innerNsEgress && innerNsIngress {
		result = append(result, StatusNamespaceFull)
	} else if innerNsEgress {
		result = append(result, StatusNamespaceEgress)
	} else if innerNsIngress {
		result = append(result, StatusNamespaceIngress)
	}

	return result
}

func GetStatusKeys() []StatusKey {
	return []StatusKey{
		StatusInternetIngress,
		StatusInternetEgress,
		StatusInternetFull,
		StatusLanIngress,
		StatusLanEgress,
		StatusLanFull,
		StatusApiServerEgress,
		StatusIsolated,
		StatusCrossNamespace,
		StatusNamespaceEgress,
		StatusNamespaceIngress,
		StatusNamespaceFull,
	}
}
