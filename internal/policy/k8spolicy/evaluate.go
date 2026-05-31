package k8spolicy

import (
	"context"
	"fmt"

	"graph/internal/k8s"
	"graph/internal/models"

	networkingv1 "k8s.io/api/networking/v1"
)

const sourceName = "k8s"

type source struct {
	client k8s.KubernetesClient
}

func New(client k8s.KubernetesClient) *source {
	return &source{client: client}
}

func (s *source) Name() string {
	return sourceName
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

func (s *source) Evaluate(_ context.Context, namespaces []string, index map[string]models.NSIndex) (models.EvaluationResult, error) {
	policiesByNS, err := s.getPolicies(namespaces)
	if err != nil {
		return models.EvaluationResult{}, fmt.Errorf("k8s policy fetch: %w", err)
	}

	policyStatuses := map[string]models.PolicyStatus{}
	nodePolicies := map[string][]models.PolicyRef{}
	for _, ns := range namespaces {
		statuses, refs := generatePolicyStatusAssignment(index[ns].Workloads, policiesByNS[ns])
		for nodeID, status := range statuses {
			policyStatuses[nodeID] = status
		}
		for nodeID, policyRefs := range refs {
			nodePolicies[nodeID] = policyRefs
		}
	}

	return models.EvaluationResult{
		AllowByNs:      buildAllowRulesByNs(index, policiesByNS),
		PolicyStatuses: policyStatuses,
		NodePolicies:   nodePolicies,
	}, nil
}


func isCatchAll(matchLabels map[string]string, nExpressions int) bool {
	return len(matchLabels) == 0 && nExpressions == 0
}
