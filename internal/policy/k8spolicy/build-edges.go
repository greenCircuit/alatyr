package k8spolicy

import (
	"context"

	"graph/internal/k8s"
	"graph/internal/models"
	"graph/internal/policy"
	"graph/internal/utils"

	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

const sourceName = "k8s"

type source struct {
	client k8s.KubernetesClient
}

func New(client k8s.KubernetesClient) *source {
	return &source{client: client}
}

func (s *source) getPolicies(namespaces []string) (map[string][]networkingv1.NetworkPolicy, error) {
	policiesByNS := map[string][]networkingv1.NetworkPolicy{}
	for _, ns := range namespaces {
		nsPolicies, err := s.client.GetPolicies(ns)
		if err != nil {
			return nil, err
		}
		policiesByNS[ns] = nsPolicies
	}
	return policiesByNS, nil
}

func (s *source) Evaluate(_ context.Context, namespaces []string, index map[string]models.NSIndex) policy.EvaluationResult {
	policiesByNS, _ := s.getPolicies(namespaces)
	return policy.EvaluationResult{
		Allow: buildAllowRules(index, policiesByNS),
	}
}

// buildAllowRules expands every NetworkPolicy into pod-level allow rules.
// One rule per (src, dst, port, direction, policy-rule).
func buildAllowRules(index map[string]models.NSIndex, policiesByNS map[string][]networkingv1.NetworkPolicy) []policy.Rule {
	var allRules []policy.Rule
	for _, policies := range policiesByNS {
		for _, networkPolicy := range policies {
			srcNodes := getSourceNodes(networkPolicy, index)
			egressRules := expandEgressRules(networkPolicy, index)
			ingressRules := expandIngressRules(networkPolicy, index)

			for _, srcNode := range srcNodes {
				for _, rule := range egressRules {
					rule.SrcID = srcNode.ID
					allRules = append(allRules, rule)
				}
				for _, rule := range ingressRules {
					rule.SrcID = rule.DstID
					rule.DstID = srcNode.ID
					allRules = append(allRules, rule)
				}
			}
		}
	}
	return allRules
}

func expandEgressRules(networkPolicy networkingv1.NetworkPolicy, index map[string]models.NSIndex) []policy.Rule {
	var out []policy.Rule
	for ruleIndex, rule := range networkPolicy.Spec.Egress {
		ports := convertPorts(rule.Ports)
		for _, peer := range rule.To {
			out = append(out, expandPeerRules(networkPolicy.Name, networkPolicy.Namespace, ruleIndex, models.DirectionEgress, peer, ports, index)...)
		}
	}
	return out
}

func expandIngressRules(networkPolicy networkingv1.NetworkPolicy, index map[string]models.NSIndex) []policy.Rule {
	var out []policy.Rule
	for ruleIndex, rule := range networkPolicy.Spec.Ingress {
		ports := convertPorts(rule.Ports)
		for _, peer := range rule.From {
			out = append(out, expandPeerRules(networkPolicy.Name, networkPolicy.Namespace, ruleIndex, models.DirectionIngress, peer, ports, index)...)
		}
	}
	return out
}


// expandPeerRules produces one rule per (peer-match × port).
// Returned rules have SrcID empty — buildAllowRules fills it from the policy's selected workloads.
func expandPeerRules(policyName, policyNamespace string, ruleIndex int, direction models.Direction, peer networkingv1.NetworkPolicyPeer, ports []models.Port, index map[string]models.NSIndex) []policy.Rule {
	var dstIDs []string

	// namespace only
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

	contributors := []policy.PolicyRef{{
		Source:    sourceName,
		Name:      policyName,
		Namespace: policyNamespace,
		RuleIndex: ruleIndex,
	}}

	effectivePorts := ports
	if len(effectivePorts) == 0 {
		effectivePorts = []models.Port{{Protocol: "TCP"}}
	}

	out := make([]policy.Rule, 0, len(dstIDs)*len(effectivePorts))
	for _, dstID := range dstIDs {
		for _, port := range effectivePorts {
			out = append(out, policy.Rule{
				DstID:        dstID,
				Port:         port,
				Direction:    direction,
				Contributors: contributors,
			})
		}
	}
	return out
}

// GetNodePolicies returns all NetworkPolicies that select the given workload node
// via PodSelector. Namespace nodes only match catch-all selectors.
func GetNodePolicies(node models.WorkloadNode, policies []networkingv1.NetworkPolicy) []networkingv1.NetworkPolicy {
	var matches []networkingv1.NetworkPolicy
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

func getSourceNodes(networkPolicy networkingv1.NetworkPolicy, index map[string]models.NSIndex) []*models.WorkloadNode {
	nsIndex := index[networkPolicy.Namespace]
	if isCatchAll(networkPolicy.Spec.PodSelector.MatchLabels, len(networkPolicy.Spec.PodSelector.MatchExpressions)) {
		if nsIndex.NSNode != nil {
			return []*models.WorkloadNode{nsIndex.NSNode}
		}
		return nil
	}
	return utils.IndexLabelMatch(networkPolicy.Spec.PodSelector.MatchLabels, nsIndex.LabelIndex)
}

func isCatchAll(matchLabels map[string]string, nExpressions int) bool {
	return len(matchLabels) == 0 && nExpressions == 0
}

// convertPorts translates rule-level NetworkPolicyPort entries into models.Port.
// Empty input returns nil — expandPeerRules expands nil into a single all-ports rule.
func convertPorts(rulePorts []networkingv1.NetworkPolicyPort) []models.Port {
	if len(rulePorts) == 0 {
		return nil
	}
	out := make([]models.Port, 0, len(rulePorts))
	for _, rulePort := range rulePorts {
		port := models.Port{Protocol: "TCP"}
		if rulePort.Protocol != nil {
			port.Protocol = string(*rulePort.Protocol)
		}
		if rulePort.Port != nil {
			if rulePort.Port.Type == intstr.String {
				port.Name = rulePort.Port.StrVal
			} else {
				port.Port = int(rulePort.Port.IntVal)
			}
		}
		if rulePort.EndPort != nil {
			port.EndPort = int(*rulePort.EndPort)
		}
		out = append(out, port)
	}
	return out
}