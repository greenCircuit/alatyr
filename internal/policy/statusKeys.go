package policy

import "graph/internal/models"

// DeriveStatusKeys derives status badges from accumulated PolicyStatus signals.
// Default derivation shared across L3/L4 selector-based policy engines
// (k8s NetworkPolicy, Cilium NetworkPolicy, etc). Engines with non-matching
// semantics (e.g. Istio AuthorizationPolicy) should emit their own keys directly.
//
// Internet, LAN, and inner-namespace badges follow the same pattern: emit the
// "full" key when both directions fire, otherwise the matching directional key.
// Internet is special: an unlocked direction implicitly counts as internet
// access on that side.
func DeriveStatusKeys(policyStatus models.PolicyStatus) []models.StatusKey {
	var result []models.StatusKey

	// Internet: unlocked direction = open to anything (including internet).
	internetEgress := !policyStatus.EgressLocked || policyStatus.InternetEgress
	internetIngress := !policyStatus.IngressLocked || policyStatus.InternetIngress
	if internetEgress && internetIngress {
		result = append(result, models.StatusInternetFull)
	} else if internetEgress {
		result = append(result, models.StatusInternetEgress)
	} else if internetIngress {
		result = append(result, models.StatusInternetIngress)
	}

	// LAN: only fires on explicit private-CIDR peers.
	if policyStatus.LanEgress && policyStatus.LanIngress {
		result = append(result, models.StatusLanFull)
	} else if policyStatus.LanEgress {
		result = append(result, models.StatusLanEgress)
	} else if policyStatus.LanIngress {
		result = append(result, models.StatusLanIngress)
	}

	if policyStatus.ApiServerEgress {
		result = append(result, models.StatusApiServerEgress)
	}

	// air-gapped: both directions locked and no escape hatch on either side.
	hasEscapeHatch := policyStatus.InternetEgress || policyStatus.InternetIngress ||
		policyStatus.LanEgress || policyStatus.LanIngress ||
		policyStatus.ApiServerEgress
	if policyStatus.EgressLocked && policyStatus.IngressLocked && !hasEscapeHatch {
		result = append(result, models.StatusIsolated)
	}

	if policyStatus.CrossNS {
		result = append(result, models.StatusCrossNamespace)
	}

	// inner-namespace: catch-all peer scoped to own namespace.
	if policyStatus.InnerNsEgress && policyStatus.InnerNsIngress {
		result = append(result, models.StatusNamespaceFull)
	} else if policyStatus.InnerNsEgress {
		result = append(result, models.StatusNamespaceEgress)
	} else if policyStatus.InnerNsIngress {
		result = append(result, models.StatusNamespaceIngress)
	}

	if policyStatus.HasL7 {
		result = append(result, models.StatusL7Applied)
	}

	return result
}

// IntersectPolicyStatus computes the effective PolicyStatus for a workload
// across multiple policy engines. Pure field-wise boolean composition —
// reflects what the workload actually experiences in the cluster.
//
// Convention each engine MUST follow when populating its PolicyStatus:
//   - Hierarchy baked in: granting internet → also set LAN + inner-ns true.
//     Granting LAN → also set inner-ns true.
//   - "Doesn't constrain this dimension" = true. An engine that does not gate
//     a given dimension (e.g. Istio AuthZ does not gate L3/L4 CIDR egress)
//     sets the relevant fields true so it doesn't subtract access in the AND.
//   - No policy on workload = WAN access. EgressLocked=false implies
//     InternetEgress/LanEgress/InnerNsEgress all true.
//
// Access fields (Internet*, Lan*, InnerNs*, ApiServerEgress, CrossNS) AND
// across engines — every engine must permit for traffic to flow.
//
// Lock fields (EgressLocked, IngressLocked) OR — if any engine has locked
// the direction, the effective state is locked.
func IntersectPolicyStatus(perEngine []models.PolicyStatus) models.PolicyStatus {
	if len(perEngine) == 0 {
		return models.PolicyStatus{}
	}
	if len(perEngine) == 1 {
		return perEngine[0]
	}

	result := perEngine[0]
	for _, status := range perEngine[1:] {
		// access fields: AND (all engines must permit)
		result.InternetEgress = result.InternetEgress && status.InternetEgress
		result.InternetIngress = result.InternetIngress && status.InternetIngress
		result.LanEgress = result.LanEgress && status.LanEgress
		result.LanIngress = result.LanIngress && status.LanIngress
		result.InnerNsEgress = result.InnerNsEgress && status.InnerNsEgress
		result.InnerNsIngress = result.InnerNsIngress && status.InnerNsIngress
		result.ApiServerEgress = result.ApiServerEgress && status.ApiServerEgress
		result.CrossNS = result.CrossNS && status.CrossNS

		// lock fields: OR (any engine locking restricts)
		result.EgressLocked = result.EgressLocked || status.EgressLocked
		result.IngressLocked = result.IngressLocked || status.IngressLocked

		// L7 presence: OR (any engine applying L7 surfaces the badge)
		result.HasL7 = result.HasL7 || status.HasL7
	}
	return result
}