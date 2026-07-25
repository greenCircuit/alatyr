package k8spolicy

import (
	"context"
	"fmt"
	"log/slog"
	"sync"

	"graph/internal/k8s"
	"graph/internal/models"

	networkingv1 "k8s.io/api/networking/v1"
)

const sourceName = "k8s"

type source struct {
	client k8s.KubernetesClient
	log    *slog.Logger
}

func New(client k8s.KubernetesClient, logger *slog.Logger) *source {
	if logger == nil {
		logger = slog.Default()
	}
	return &source{client: client, log: logger}
}

func (s *source) Name() string {
	return sourceName
}

func (s *source) getPolicies(namespaces []string) (map[string][]*networkingv1.NetworkPolicy, error) {
	policiesByNS := map[string][]*networkingv1.NetworkPolicy{}
	var mu sync.Mutex
	var firstErr error
	var wg sync.WaitGroup
	for _, ns := range namespaces {
		wg.Add(1)
		go func(ns string) {
			defer wg.Done()
			nsPolicies, err := s.client.GetPolicies(ns)
			if err != nil {
				s.log.Warn("k8s policy fetch failed",
					slog.String("phase", "k8s_get_policies"),
					slog.String("engine", sourceName),
					slog.String("ns", ns),
					slog.String("error", err.Error()),
				)
				mu.Lock()
				if firstErr == nil {
					firstErr = err
				}
				mu.Unlock()
				return
			}
			mu.Lock()
			policiesByNS[ns] = nsPolicies
			mu.Unlock()

		} (ns)
	}
	wg.Wait()

	if firstErr != nil {
		return nil, firstErr
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

	allowByNs, nodeRules, cidrNodes := buildAllowRulesByNs(index, policiesByNS)
	return models.EvaluationResult{
		AllowByNs:      allowByNs,
		PolicyStatuses: policyStatuses,
		NodePolicies:   nodePolicies,
		NodeRules:      nodeRules,
		Nodes:          cidrNodes,
	}, nil
}


func isCatchAll(matchLabels map[string]string, nExpressions int) bool {
	return len(matchLabels) == 0 && nExpressions == 0
}
