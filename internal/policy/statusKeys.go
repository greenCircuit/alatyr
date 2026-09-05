package policy

import "alatyr/internal/models"

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
// across multiple policy engines. Reflects what the workload actually
// experiences in the cluster.
//
// Engines populate PolicyStatus with PURE INTENT — only flip dimensions the
// policy explicitly references. EgressLocked=false signals "this engine
// doesn't gate egress" (transparent); the intersection layer interprets
// transparency below — engines do NOT pre-fill access fields to true.
//
// Per-engine effective access on a dimension D = `!engineLocked(D) || engine.D`.
// "Engine not locked = transparent for D" matches the rule DeriveStatusKeys
// uses on the single-engine path; intersection propagates the same rule so
// per-engine views and intersected views stay consistent.
//
// Field semantics across engines:
//   - Access (Internet*, Lan*, InnerNs*, ApiServerEgress): AND of per-engine
//     effective values — every engine must permit for traffic to flow.
//   - CrossNS: same AND, but transparency requires BOTH directions unlocked
//     (CrossNS spans ingress + egress in buildPolicyStatus accumulators).
//   - Locks (EgressLocked, IngressLocked): OR — any engine locking restricts.
//   - HasL7: OR — any engine applying L7 surfaces the badge.
func IntersectPolicyStatus(perEngine []models.PolicyStatus) models.PolicyStatus {
	// Drop engines with zero PolicyStatus — they had no policy referencing
	// this workload at all. Treating them as "transparency on every
	// dimension" inflates the intersection (a transparent egress would
	// claim LAN/InnerNs/ApiServer egress even when DeriveStatusKeys' single-
	// engine path would only claim Internet). Filtering keeps single-engine
	// and multi-engine semantics aligned: no-opinion engines don't count.
	filtered := make([]models.PolicyStatus, 0, len(perEngine))
	for _, status := range perEngine {
		if status != (models.PolicyStatus{}) {
			filtered = append(filtered, status)
		}
	}
	if len(filtered) == 0 {
		return models.PolicyStatus{}
	}
	if len(filtered) == 1 {
		return filtered[0]
	}
	perEngine = filtered

	result := effectivePolicyStatus(perEngine[0])
	for _, raw := range perEngine[1:] {
		status := effectivePolicyStatus(raw)
		result.InternetEgress = result.InternetEgress && status.InternetEgress
		result.InternetIngress = result.InternetIngress && status.InternetIngress
		result.LanEgress = result.LanEgress && status.LanEgress
		result.LanIngress = result.LanIngress && status.LanIngress
		result.InnerNsEgress = result.InnerNsEgress && status.InnerNsEgress
		result.InnerNsIngress = result.InnerNsIngress && status.InnerNsIngress
		result.ApiServerEgress = result.ApiServerEgress && status.ApiServerEgress
		result.CrossNS = result.CrossNS && status.CrossNS

		result.EgressLocked = result.EgressLocked || status.EgressLocked
		result.IngressLocked = result.IngressLocked || status.IngressLocked

		result.HasL7 = result.HasL7 || status.HasL7
	}
	return result
}

// effectivePolicyStatus expands a single engine's raw intent into the form
// used by the cross-engine AND. Unlocked direction = transparent for every
// access field gated by that direction; CrossNS needs both directions
// unlocked to be transparent. Locks and HasL7 pass through unchanged.
func effectivePolicyStatus(raw models.PolicyStatus) models.PolicyStatus {
	return models.PolicyStatus{
		EgressLocked:    raw.EgressLocked,
		IngressLocked:   raw.IngressLocked,
		InternetEgress:  !raw.EgressLocked || raw.InternetEgress,
		InternetIngress: !raw.IngressLocked || raw.InternetIngress,
		LanEgress:       !raw.EgressLocked || raw.LanEgress,
		LanIngress:      !raw.IngressLocked || raw.LanIngress,
		InnerNsEgress:   !raw.EgressLocked || raw.InnerNsEgress,
		InnerNsIngress:  !raw.IngressLocked || raw.InnerNsIngress,
		ApiServerEgress: !raw.EgressLocked || raw.ApiServerEgress,
		CrossNS:         (!raw.EgressLocked && !raw.IngressLocked) || raw.CrossNS,
		HasL7:           raw.HasL7,
	}
}
