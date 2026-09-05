package k8spolicy

import (
	"time"

	"alatyr/internal/models"
	"alatyr/internal/policy"
	"alatyr/internal/utils"

	networkingv1 "k8s.io/api/networking/v1"
)

// creationTime returns a pointer to the policy's CreationTimestamp, or nil
// when unset — omitempty then drops it from the wire rather than marshaling
// year 0001.
func creationTime(np *networkingv1.NetworkPolicy) *time.Time {
	if np.CreationTimestamp.IsZero() {
		return nil
	}
	t := np.CreationTimestamp.Time
	return &t
}

// buildAllowRulesByNs expands every NetworkPolicy into pod-level rules,
// grouped by the policy's namespace so per-ns cache invalidation can replace
// one ns's rules without touching others. Third return is CIDR peer nodes
// synthesized inline by expandPeerRules, deduped by ID across all policies.
func buildAllowRulesByNs(index map[string]models.NSIndex, policiesByNS map[string][]*networkingv1.NetworkPolicy) (map[string][]models.Rule, map[string]models.NodeRules, map[string]models.WorkloadNode) {
	policyMap := map[string]*models.NodeRules{} // have pointer so don't reconstruct map every time update it
	result := map[string][]models.Rule{}
	cidrNodes := map[string]models.WorkloadNode{}
	for ns, policies := range policiesByNS {
		var nsRules []models.Rule
		for _, networkPolicy := range policies {
			srcNodes := getSourceNodes(networkPolicy, index)
			egressRules, egressCIDRs := expandEgressRules(networkPolicy, index)
			ingressRules, ingressCIDRs := expandIngressRules(networkPolicy, index)
			for id, node := range egressCIDRs {
				cidrNodes[id] = node
			}
			for id, node := range ingressCIDRs {
				cidrNodes[id] = node
			}

			for _, srcNode := range srcNodes {
				for _, rule := range egressRules {
					rule.SrcID = srcNode.ID
					nsRules = append(nsRules, rule)

					// adding things to global map for policy index
					nodeRules, ok := policyMap[rule.SrcID]
					if !ok {
						nodeRules = &models.NodeRules{}
						policyMap[rule.SrcID] = nodeRules
					}
					if rule.Action == models.ActionAllow {
						nodeRules.Egress.Allow = append(nodeRules.Egress.Allow, rule)
					} else {
						nodeRules.Egress.Deny = append(nodeRules.Egress.Deny, rule)
					}

				}
				for _, rule := range ingressRules {
					rule.SrcID = rule.DstID
					rule.DstID = srcNode.ID
					nsRules = append(nsRules, rule)

					// adding things to global map for policy index. key by the
					// protected (selected) node, which after the swap is DstID.
					protectedID := rule.DstID
					nodeRules, ok := policyMap[protectedID]
					if !ok {
						nodeRules = &models.NodeRules{}
						policyMap[protectedID] = nodeRules
					}
					if rule.Action == models.ActionAllow {
						nodeRules.Ingress.Allow = append(nodeRules.Ingress.Allow, rule)
					} else {
						nodeRules.Ingress.Deny = append(nodeRules.Ingress.Deny, rule)
					}
				}
			}
		}
		if len(nsRules) > 0 {
			result[ns] = nsRules
		}
	}

	// dereference policy map so return struct so don't mutate state later on as guard
	nonPointerPolicyMap := make(map[string]models.NodeRules, len(policyMap))
	for nodeID, nodeRules := range policyMap {
		nonPointerPolicyMap[nodeID] = *nodeRules
	}

	return result, nonPointerPolicyMap, cidrNodes
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

func expandEgressRules(networkPolicy *networkingv1.NetworkPolicy, index map[string]models.NSIndex) ([]models.Rule, map[string]models.WorkloadNode) {
	var out []models.Rule
	cidrNodes := map[string]models.WorkloadNode{}

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
			Contributor: models.PolicyRef{Source: sourceName, Name: networkPolicy.Name, Namespace: networkPolicy.Namespace, CreatedAt: creationTime(networkPolicy)},
		}), cidrNodes
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
				Contributor: models.PolicyRef{Source: sourceName, Name: networkPolicy.Name, Namespace: networkPolicy.Namespace, CreatedAt: creationTime(networkPolicy)},
			})
			continue
		}

		for _, peer := range rule.To {
			matchRules, peerCIDRs := expandPeerRules(networkPolicy, ruleIndex, models.DirectionEgress, peer, ports, index)
			for id, node := range peerCIDRs {
				cidrNodes[id] = node
			}
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
	return out, cidrNodes
}

func expandIngressRules(networkPolicy *networkingv1.NetworkPolicy, index map[string]models.NSIndex) ([]models.Rule, map[string]models.WorkloadNode) {
	var out []models.Rule
	cidrNodes := map[string]models.WorkloadNode{}

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
			Contributor: models.PolicyRef{Source: sourceName, Name: networkPolicy.Name, Namespace: networkPolicy.Namespace, CreatedAt: creationTime(networkPolicy)},
		}), cidrNodes
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
				Contributor: models.PolicyRef{Source: sourceName, Name: networkPolicy.Name, Namespace: networkPolicy.Namespace, CreatedAt: creationTime(networkPolicy)},
			})
			continue
		}

		for _, peer := range rule.From {
			matchRules, peerCIDRs := expandPeerRules(networkPolicy, ruleIndex, models.DirectionIngress, peer, ports, index)
			for id, node := range peerCIDRs {
				cidrNodes[id] = node
			}
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
	return out, cidrNodes
}

// expandPeerRules produces one rule per (peer-match) and, when the peer is an
// ipBlock, the synthetic WorkloadNode(s) the rules point at. IDs for CIDR
// peers are prefixed with "cidr:" so the graph layer's node set stays
// authoritative for peer type (no PeerKind sidecar).
// IPBlock.Except entries emit separate ActionDeny rules with Coverage=CoverageExcept
// so the frontend can render them as carve-outs of the parent allow rather
// than confuse them with Istio DENY policies.
// Returned rules have SrcID empty — buildAllowRules fills it from the policy's selected workloads.
func expandPeerRules(networkPolicy *networkingv1.NetworkPolicy, ruleIndex int, direction models.Direction, peer networkingv1.NetworkPolicyPeer, ports []models.Port, index map[string]models.NSIndex) ([]models.Rule, map[string]models.WorkloadNode) {
	var dstIDs []string
	var exceptDstIDs []string
	cidrNodes := map[string]models.WorkloadNode{}
	coverage := models.CoverageRestricted
	// namespace only set correct coverage enum
	if peer.PodSelector == nil && peer.NamespaceSelector != nil {
		if isCatchAll(peer.NamespaceSelector.MatchLabels, len(peer.NamespaceSelector.MatchExpressions)) {
			return nil, nil
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
		nsIndex := index[networkPolicy.Namespace]
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
		cidrID := models.CIDRIDPrefix + peer.IPBlock.CIDR
		dstIDs = append(dstIDs, cidrID)
		cidrNodes[cidrID] = models.WorkloadNode{
			ID:       cidrID,
			Label:    peer.IPBlock.CIDR,
			Type:     models.NodeTypeCIDR,
			CidrType: policy.CidrType(peer.IPBlock.CIDR),
		}
		for _, exceptCIDR := range peer.IPBlock.Except {
			exceptID := models.CIDRIDPrefix + exceptCIDR
			exceptDstIDs = append(exceptDstIDs, exceptID)
			cidrNodes[exceptID] = models.WorkloadNode{
				ID:       exceptID,
				Label:    exceptCIDR,
				Type:     models.NodeTypeCIDR,
				CidrType: policy.CidrType(exceptCIDR),
			}
		}
	}

	// metadata about policy
	contributor := models.PolicyRef{
		Source:    sourceName,
		Name:      networkPolicy.Name,
		Namespace: networkPolicy.Namespace,
		CreatedAt: creationTime(networkPolicy),
	}

	out := make([]models.Rule, 0, len(dstIDs)+len(exceptDstIDs))
	for _, dstID := range dstIDs {
		rule := models.Rule{
			DstID:       dstID,
			Ports:       ports,
			Direction:   direction,
			Contributor: contributor,
			Coverage:    coverage,
			AllL7:       true,
		}

		if len(ports) == 0 {
			rule.AllPorts = true
		}

		out = append(out, rule)
	}

	// Except → separate deny rules, same contributor (still an allow policy),
	// Coverage=CoverageExcept marks them as carve-outs (distinct from Istio DENY).
	for _, exceptID := range exceptDstIDs {
		rule := models.Rule{
			DstID:       exceptID,
			Ports:       ports,
			Direction:   direction,
			Contributor: contributor,
			Coverage:    models.CoverageExcept,
			Action:      models.ActionDeny,
			AllL7:       true,
		}
		if len(ports) == 0 {
			rule.AllPorts = true
		}
		out = append(out, rule)
	}

	return out, cidrNodes
}
