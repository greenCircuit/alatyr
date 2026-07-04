package k8spolicy

import (
	"graph/internal/models"
	"graph/internal/utils"
	networkingv1 "k8s.io/api/networking/v1"
)

// buildAllowRulesByNs expands every NetworkPolicy into pod-level allow rules,
// grouped by the policy's namespace so per-ns cache invalidation can replace
// one ns's rules without touching others.
func buildAllowRulesByNs(index map[string]models.NSIndex, policiesByNS map[string][]*networkingv1.NetworkPolicy) map[string][]models.Rule {
	result := map[string][]models.Rule{}
	for ns, policies := range policiesByNS {
		var nsRules []models.Rule
		for _, networkPolicy := range policies {
			srcNodes := getSourceNodes(networkPolicy, index)
			egressRules := expandEgressRules(networkPolicy, index)
			ingressRules := expandIngressRules(networkPolicy, index)

			for _, srcNode := range srcNodes {
				for _, rule := range egressRules {
					rule.SrcID = srcNode.ID
					nsRules = append(nsRules, rule)
				}
				for _, rule := range ingressRules {
					rule.SrcID = rule.DstID
					rule.DstID = srcNode.ID
					nsRules = append(nsRules, rule)
				}
			}
		}
		if len(nsRules) > 0 {
			result[ns] = nsRules
		}
	}
	return result
}

func getSourceNodes(networkPolicy *networkingv1.NetworkPolicy, index map[string]models.NSIndex) []*models.WorkloadNode {
	nsIndex := index[networkPolicy.Namespace]
	if isCatchAll(networkPolicy.Spec.PodSelector.MatchLabels, len(networkPolicy.Spec.PodSelector.MatchExpressions)) {
		if nsIndex.NSNode != nil {
			return []*models.WorkloadNode{nsIndex.NSNode}
		}
		return nil
	}
	return utils.IndexLabelMatch(networkPolicy.Spec.PodSelector.MatchLabels, nsIndex.LabelIndex)
}

// egressLocked reports whether the policy governs egress: "Egress" listed in
// PolicyTypes. Egress is never implied — only ingress is.
func egressLocked(networkPolicy *networkingv1.NetworkPolicy) bool {
	for _, policyType := range networkPolicy.Spec.PolicyTypes {
		if policyType == networkingv1.PolicyTypeEgress {
			return true
		}
	}
	return false
}

// ingressLocked reports whether the policy governs ingress. Omitted PolicyTypes
// implies Ingress per k8s, so an empty list still locks ingress.
func ingressLocked(networkPolicy *networkingv1.NetworkPolicy) bool {
	if len(networkPolicy.Spec.PolicyTypes) == 0 {
		return true
	}
	for _, policyType := range networkPolicy.Spec.PolicyTypes {
		if policyType == networkingv1.PolicyTypeIngress {
			return true
		}
	}
	return false
}

func expandEgressRules(networkPolicy *networkingv1.NetworkPolicy, index map[string]models.NSIndex) []models.Rule {
	var out []models.Rule

	// No egress rules: deny-all when egress is locked, otherwise the policy
	// doesn't govern egress at all — recorded as unenforced so it still shows.
	if len(networkPolicy.Spec.Egress) == 0 {
		coverage := models.CoverageUnenforced
		if egressLocked(networkPolicy) {
			coverage = models.CoverageDenyAll
		}
		return append(out, models.Rule{
			Direction:   models.DirectionEgress,
			Coverage:    coverage,
			Contributor: models.PolicyRef{Source: sourceName, Name: networkPolicy.Name, Namespace: networkPolicy.Namespace},
		})
	}

	for ruleIndex, rule := range networkPolicy.Spec.Egress {
		ports := convertPorts(rule.Ports)

		// empty peer list = allow-all destinations
		if len(rule.To) == 0 {
			out = append(out, models.Rule{
				Direction:   models.DirectionEgress,
				Coverage:    models.CoverageAllowAll,
				Ports:       ports,
				AllPorts:    len(ports) == 0,
				AllL7:       true,
				Contributor: models.PolicyRef{Source: sourceName, Name: networkPolicy.Name, Namespace: networkPolicy.Namespace, RuleIndex: ruleIndex},
			})
			continue
		}

		for _, peer := range rule.To {
			matchRules := expandPeerRules(networkPolicy.Name, networkPolicy.Namespace, ruleIndex, models.DirectionEgress, peer, ports, index)
			for _, rule := range matchRules {
				rule.SrcSelector.LabelSelector = networkPolicy.Spec.PodSelector.MatchLabels
				if peer.PodSelector != nil {
					rule.DstSelector.LabelSelector = peer.PodSelector.MatchLabels
				}
				if peer.NamespaceSelector != nil {
					rule.DstSelector.NsSelector = peer.NamespaceSelector.MatchLabels
				}

				out = append(out, rule)
			}
		}
	}
	return out
}

func expandIngressRules(networkPolicy *networkingv1.NetworkPolicy, index map[string]models.NSIndex) []models.Rule {
	var out []models.Rule

	// No ingress rules: deny-all when ingress is locked (always, unless
	// PolicyTypes explicitly lists only Egress), otherwise unenforced.
	if len(networkPolicy.Spec.Ingress) == 0 {
		coverage := models.CoverageUnenforced
		if ingressLocked(networkPolicy) {
			coverage = models.CoverageDenyAll
		}
		return append(out, models.Rule{
			Direction:   models.DirectionIngress,
			Coverage:    coverage,
			Contributor: models.PolicyRef{Source: sourceName, Name: networkPolicy.Name, Namespace: networkPolicy.Namespace},
		})
	}

	for ruleIndex, rule := range networkPolicy.Spec.Ingress {
		ports := convertPorts(rule.Ports)

		// empty peer list = allow-all sources
		if len(rule.From) == 0 {
			out = append(out, models.Rule{
				Direction:   models.DirectionIngress,
				Coverage:    models.CoverageAllowAll,
				Ports:       ports,
				AllPorts:    len(ports) == 0,
				AllL7:       true,
				Contributor: models.PolicyRef{Source: sourceName, Name: networkPolicy.Name, Namespace: networkPolicy.Namespace, RuleIndex: ruleIndex},
			})
			continue
		}

		for _, peer := range rule.From {
			matchRules := expandPeerRules(networkPolicy.Name, networkPolicy.Namespace, ruleIndex, models.DirectionIngress, peer, ports, index)
			for _, rule := range matchRules {
				if peer.PodSelector != nil {
					rule.SrcSelector.LabelSelector = peer.PodSelector.MatchLabels
				}
				if peer.NamespaceSelector != nil {
					rule.SrcSelector.NsSelector = peer.NamespaceSelector.MatchLabels
				}
				rule.DstSelector.LabelSelector = networkPolicy.Spec.PodSelector.MatchLabels
				out = append(out, rule)
			}
		}
	}
	return out
}


// expandPeerRules produces one rule per (peer-match).
// Returned rules have SrcID empty — buildAllowRules fills it from the policy's selected workloads.
func expandPeerRules(policyName, policyNamespace string, ruleIndex int, direction models.Direction, peer networkingv1.NetworkPolicyPeer, ports []models.Port, index map[string]models.NSIndex) []models.Rule {
	var dstIDs []string

	coverage := models.CoverageRestricted       
	// namespace only set correct coverage enum
	if peer.PodSelector == nil && peer.NamespaceSelector != nil {
		if isCatchAll(peer.NamespaceSelector.MatchLabels, len(peer.NamespaceSelector.MatchExpressions)) {
			return nil
		}
		for _, nsIndex := range index {
			if nsIndex.NSNode == nil || !utils.LabelsMatch(peer.NamespaceSelector.MatchLabels, nsIndex.NSNode.Labels) {
				continue
			}
			dstIDs = append(dstIDs, nsIndex.NSNode.ID)
		}
	}

	// pod selector only — same namespace
	if peer.PodSelector != nil && peer.NamespaceSelector == nil {
		nsIndex := index[policyNamespace]
		if isCatchAll(peer.PodSelector.MatchLabels, len(peer.PodSelector.MatchExpressions)) {
			if nsIndex.NSNode != nil {
				dstIDs = append(dstIDs, nsIndex.NSNode.ID)
				coverage = models.CoverageAllowAllNs
			}
		} else {
			for _, target := range utils.IndexLabelMatch(peer.PodSelector.MatchLabels, nsIndex.LabelIndex) {
				dstIDs = append(dstIDs, target.ID)
			}
		}
	}

	// both — pods in namespaces matching namespaceSelector
	if peer.PodSelector != nil && peer.NamespaceSelector != nil {
		catchAllPod := isCatchAll(peer.PodSelector.MatchLabels, len(peer.PodSelector.MatchExpressions))
		catchAllNS := isCatchAll(peer.NamespaceSelector.MatchLabels, len(peer.NamespaceSelector.MatchExpressions))
		for _, nsIndex := range index {
			if !catchAllNS {
				if nsIndex.NSNode == nil || !utils.LabelsMatch(peer.NamespaceSelector.MatchLabels, nsIndex.NSNode.Labels) {
					continue
				}
			}
			if catchAllPod {
				if nsIndex.NSNode != nil {
					dstIDs = append(dstIDs, nsIndex.NSNode.ID)
					coverage = models.CoverageAllowAllNs
				}
				continue
			}
			for _, target := range utils.IndexLabelMatch(peer.PodSelector.MatchLabels, nsIndex.LabelIndex) {
				dstIDs = append(dstIDs, target.ID)
			}
		}
	}

	// IP block — external traffic
	if peer.IPBlock != nil {
		dstIDs = append(dstIDs, peer.IPBlock.CIDR)
	}

	// metadata about policy
	contributor := models.PolicyRef{
		Source:    sourceName,
		Name:      policyName,
		Namespace: policyNamespace,
		RuleIndex: ruleIndex,
	}

	out := make([]models.Rule, 0, len(dstIDs))
	for _, dstID := range dstIDs {
		rule := models.Rule{
			DstID:        dstID,
			Ports:        ports,
			Direction:    direction,
			Contributor:  contributor,
			Coverage:     coverage,
			AllL7: 		  true,
		}

		if len(ports) == 0 {
			rule.AllPorts = true
		}

		out = append(out, rule)
	}

	return out
}