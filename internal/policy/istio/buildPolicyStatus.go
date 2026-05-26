package istio

import (
	"graph/internal/models"
	"graph/internal/policy"
	"graph/internal/utils"

	istiosec "istio.io/client-go/pkg/apis/security/v1"
	networkingv1 "k8s.io/api/networking/v1"
)

// generatePolicyStatusAssignment returns per-workload PolicyStatus for every
// node in the namespace, even when no AuthorizationPolicy selects it
// (zero-value PolicyStatus). The graph layer intersects across engines and
// derives status keys downstream — engines never derive keys themselves.
//
// `nsPolicies` are AuthorizationPolicies declared in this workload's
// namespace. `globalPolicies` are policies declared in the Istio root
// namespace (typically "istio-system") — Istio treats those as mesh-wide
// rules that apply to every workload, so they're candidates for every node
// alongside the namespace-local set.
func generatePolicyStatusAssignment(
	nodes []models.WorkloadNode,
	nsPolicies []*istiosec.AuthorizationPolicy,
	globalPolicies []*istiosec.AuthorizationPolicy,
) map[string]models.PolicyStatus {
	assignments := map[string]models.PolicyStatus{}
	for nodeIndex := range nodes {
		node := nodes[nodeIndex]

		// Combine namespace-local + mesh-wide global policies as the
		// candidate set; getNodePolicies filters further by WorkloadSelector.
		candidates := make([]*istiosec.AuthorizationPolicy, 0, len(nsPolicies)+len(globalPolicies))
		candidates = append(candidates, nsPolicies...)
		candidates = append(candidates, globalPolicies...)

		matchPolicies := getNodePolicies(node, candidates)
		assignments[node.ID] = buildPolicyStatus(matchPolicies)
	}
	return assignments
}

// RootNamespace is the Istio mesh root namespace. AuthorizationPolicies
// declared here apply mesh-wide rather than to a single namespace.
// Convention is "istio-system"; configurable via Istio install values.
const RootNamespace = "istio-system"

// getNodePolicies returns every AuthorizationPolicy that selects the given
// workload via its WorkloadSelector. Nil selector OR empty MatchLabels =
// catch-all (applies to every workload in the policy's namespace).
// Namespace nodes only match catch-all policies (same rule as k8s engine).
func getNodePolicies(node models.WorkloadNode, policies []*istiosec.AuthorizationPolicy) []*istiosec.AuthorizationPolicy {
	var matches []*istiosec.AuthorizationPolicy
	for _, authzPolicy := range policies {
		catchAll := isCatchAllSelector(authzPolicy.Spec.Selector)

		if node.Type == models.NodeTypeNamespace {
			if catchAll {
				matches = append(matches, authzPolicy)
			}
			continue
		}

		if catchAll {
			matches = append(matches, authzPolicy)
			continue
		}
		if utils.LabelsMatch(authzPolicy.Spec.Selector.MatchLabels, node.Labels) {
			matches = append(matches, authzPolicy)
		}
	}
	return matches
}

// buildPolicyStatus accumulates raw policy signals for a single workload from
// the AuthorizationPolicies that select it. The output PolicyStatus is consumed
// by policy.DeriveStatusKeys downstream.
//
// Badges reflect what Istio policies EXPLICITLY say (same discipline as the
// k8s engine in policy/k8spolicy/buildStatusKeys.go). Dimensions are only
// flipped when a policy references them — no "default true" for dimensions
// Istio doesn't gate. Egress fields stay false because Istio AuthZ doesn't
// define egress rules; the resulting badge correctly shows "ingress-side
// intent only."
//
// L3 signals populated:
//   - IngressLocked    = any selecting policy gates ingress
//   - LanIngress       = source.ipBlocks contains private CIDR
//   - InternetIngress  = source.ipBlocks contains public CIDR / 0.0.0.0/0
//   - InnerNsIngress   = source.namespaces contains policy's own namespace
//   - CrossNS          = source.namespaces contains a different namespace
//   - notNamespaces / notIpBlocks → over-claim the dimensions they touch
//     (explicit "broad allow" intent in the policy)
func buildPolicyStatus(policies []*istiosec.AuthorizationPolicy) models.PolicyStatus {
	var status models.PolicyStatus
	if len(policies) == 0 {
		return status
	}

	// Deny-all short-circuit: any DENY policy with a wildcard-source rule
	// blocks every ingress dimension regardless of allow policies.
	// DENY wins, no need to walk allow accumulation.
	for _, authzPolicy := range policies {
		if authzPolicy.Spec.Action != 1 { // skip non-DENY
			continue
		}
		for _, rule := range authzPolicy.Spec.Rules {
			if ruleHasL7Match(rule) {
				status.HasL7 = true
			}
			if ruleMatchesAnySource(rule) {
				return models.PolicyStatus{IngressLocked: true, HasL7: status.HasL7}
			}
		}
	}

	// Allow-all short-circuit: any ALLOW policy with a wildcard-source rule
	// grants every ingress dimension. Specific rules in the same policy are
	// OR'd into the same effect (Istio rule semantics) — allow-all wins.
	// Runs AFTER deny-all check so deny-all takes precedence.
	for _, authzPolicy := range policies {
		if authzPolicy.Spec.Action != 0 { // skip non-ALLOW
			continue
		}
		for _, rule := range authzPolicy.Spec.Rules {
			if ruleHasL7Match(rule) {
				status.HasL7 = true
			}
			if ruleMatchesAnySource(rule) {
				return models.PolicyStatus{
					InnerNsIngress: true,
					CrossNS:        true,
					HasL7:          status.HasL7,
				}
			}
		}
	}

	// Two accumulators — one for everything ALLOW policies grant, one for
	// everything DENY policies block. Combine step subtracts deny from allow.
	allowSignals := newPolicySignals()
	denySignals := newPolicySignals()

	for _, authzPolicy := range policies {
		// Action enum: 0 = ALLOW, 1 = DENY, 2 = AUDIT, 3 = CUSTOM.
		// Only ALLOW + DENY participate in L3 status derivation.
		var target *policySignals
		switch authzPolicy.Spec.Action {
		case 0: // ALLOW
			target = &allowSignals
		case 1: // DENY
			target = &denySignals
		default:
			continue
		}

		for _, rule := range authzPolicy.Spec.Rules {
			if ruleHasL7Match(rule) {
				status.HasL7 = true
			}
			for _, from := range rule.From {
				if from.Source == nil {
					continue
				}
				source := from.Source
				for _, namespace := range source.Namespaces {
					target.namespaces[namespace] = true
				}
				for _, notNamespace := range source.NotNamespaces {
					target.notNamespaces[notNamespace] = true
				}
				for _, ipBlock := range source.IpBlocks {
					target.ipBlocks[ipBlock] = true
				}
				for _, notIpBlock := range source.NotIpBlocks {
					target.notIpBlocks[notIpBlock] = true
				}
			}
		}
	}

	combinedSignals := combinePolicySignals(allowSignals, denySignals)

	// Any selecting policy gates ingress (default-deny-then-allow for ALLOW,
	// narrow block for DENY). Needed for downstream air-gapped detection.
	status.IngressLocked = true

	originatingNs := policies[0].Namespace // selected workload shares ns with policy

	// Inner-ns ingress: own ns appears in survivors.
	if combinedSignals.namespaces[originatingNs] {
		status.InnerNsIngress = true
	}
	// Cross-ns ingress: any survivor that's not own ns.
	for namespace := range combinedSignals.namespaces {
		if namespace != originatingNs {
			status.CrossNS = true
			break
		}
	}

	// CIDR classification — same helpers k8s engine uses.
	for cidr := range combinedSignals.ipBlocks {
		block := networkingv1.IPBlock{CIDR: cidr}
		if policy.IsIpBlockInternetAccess(block) {
			status.InternetIngress = true
		}
		if policy.IsIpBlockLanAccess(block) {
			status.LanIngress = true
		}
	}

	// Wildcard intent: notNamespaces / notIpBlocks = "allow from any except these."
	// Conservative over-claim — flip both dimensions in the affected category.
	if len(combinedSignals.notNamespaces) > 0 {
		status.InnerNsIngress = true
		status.CrossNS = true
	}
	if len(combinedSignals.notIpBlocks) > 0 {
		status.LanIngress = true
		status.InternetIngress = true
	}

	return status
}


// combinePolicySignals takes everything ALLOW policies granted minus everything
// DENY policies block, returns the effective signals that survive. Caller
// derives PolicyStatus dimension booleans from this output.
//
// Subtraction rules per dimension:
//   - Drop allowed entry if it's in denied set (explicit deny match)
//   - Drop allowed entry if denied has wildcard (notNamespaces/notIpBlocks)
//     AND the entry is NOT in the wildcard's exception list
//
// Wildcard allow (allowed.notNamespaces / notIpBlocks) pass through unchanged
// — the derivation step over-claims the affected dimensions.
func combinePolicySignals(allowed policySignals, denied policySignals) policySignals {
	combined := newPolicySignals()

	for namespace := range allowed.namespaces {
		if denied.namespaces[namespace] {
			continue
		}
		if isBlockedByWildcardDeny(namespace, denied.notNamespaces) {
			continue
		}
		combined.namespaces[namespace] = true
	}

	for ipBlock := range allowed.ipBlocks {
		if denied.ipBlocks[ipBlock] {
			continue
		}
		if isBlockedByWildcardDeny(ipBlock, denied.notIpBlocks) {
			continue
		}
		combined.ipBlocks[ipBlock] = true
	}

	// Wildcard allow pass-through.
	for notNamespace := range allowed.notNamespaces {
		combined.notNamespaces[notNamespace] = true
	}
	for notIpBlock := range allowed.notIpBlocks {
		combined.notIpBlocks[notIpBlock] = true
	}

	return combined
}

// isBlockedByWildcardDeny reports whether entry is blocked by a wildcard deny.
// Non-empty exclusion set means "deny everything except these" — entry is
// blocked iff it's NOT in the exclusion set.
func isBlockedByWildcardDeny(entry string, denyExclusions map[string]bool) bool {
	if len(denyExclusions) == 0 {
		return false
	}
	return !denyExclusions[entry]
}