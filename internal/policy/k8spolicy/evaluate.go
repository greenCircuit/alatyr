package k8spolicy

import (
	"context"

	"graph/internal/k8s"
	"graph/internal/models"
	"graph/internal/policy"

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

// Coverage is a stub — k8s NetworkPolicy applies to every pod-bearing
// workload. Refine later if needed (e.g. node-type gating).
func (s *source) Coverage(node *models.WorkloadNode, dir models.Direction) policy.CoverageMode {
	return policy.CoverageDefaultAllow
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

	policyStatuses := map[string]models.PolicyStatus{}
	for _, ns := range namespaces {
		for nodeID, status := range generatePolicyStatusAssignment(index[ns].Workloads, policiesByNS[ns]) {
			policyStatuses[nodeID] = status
		}
	}

	return policy.EvaluationResult{
		Allow:          buildAllowRules(index, policiesByNS),
		PolicyStatuses: policyStatuses,
	}
}


func isCatchAll(matchLabels map[string]string, nExpressions int) bool {
	return len(matchLabels) == 0 && nExpressions == 0
}
