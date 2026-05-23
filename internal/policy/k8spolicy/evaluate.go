package k8spolicy

import (
	"context"

	"graph/internal/k8s"
	"graph/internal/models"
	"graph/internal/policy"
	"graph/internal/utils"

	networkingv1 "k8s.io/api/networking/v1"
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
