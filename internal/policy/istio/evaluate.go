package istio

import (
	"context"
	"fmt"
	"log/slog"

	"graph/internal/k8s"
	"graph/internal/models"

	istiosec "istio.io/client-go/pkg/apis/security/v1"
)

const sourceName = "istio"

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

// Evaluate fetches AuthorizationPolicies for the given namespaces, splits them
// by action, and returns Allow rules, Deny rules, and per-workload PolicyStatus.
// Root-namespace (mesh-wide) policies feed PolicyStatus but do not yet drive
// rules — buildRules selects workloads by the policy's own namespace, so
// applying a root-ns policy to every namespace needs a separate fan-out (TODO).
func (s *source) Evaluate(_ context.Context, namespaces []string, index map[string]models.NSIndex) (models.EvaluationResult, error) {
	policiesByNS, err := s.getPolicies(namespaces)
	if err != nil {
		return models.EvaluationResult{}, fmt.Errorf("istio policy fetch: %w", err)
	}
	globalPolicies, err := s.client.GetAuthorizationPolicies(RootNamespace)
	if err != nil {
		return models.EvaluationResult{}, fmt.Errorf("istio root-ns policy fetch: %w", err)
	}

	allowByNS, denyByNS := splitPoliciesByAction(policiesByNS)

	policyStatuses := map[string]models.PolicyStatus{}
	nodePolicies := map[string][]models.PolicyRef{}
	for _, ns := range namespaces {
		statuses, refs := generatePolicyStatusAssignment(index[ns].Workloads, policiesByNS[ns], globalPolicies)
		for nodeID, status := range statuses {
			policyStatuses[nodeID] = status
		}
		for nodeID, policyRefs := range refs {
			nodePolicies[nodeID] = policyRefs
		}
	}

	nodeRulesPtr := map[string]*models.NodeRules{}
	allowByNsRules := buildRulesByNs(index, allowByNS, nodeRulesPtr)
	denyByNsRules := buildRulesByNs(index, denyByNS, nodeRulesPtr)
	for ns, rules := range denyByNsRules {
		for ruleIndex := range rules {
			rules[ruleIndex].Action = models.ActionDeny
		}
		denyByNsRules[ns] = rules
	}

	// dereference to hand back a frozen (non-pointer) index
	nodeRules := make(map[string]models.NodeRules, len(nodeRulesPtr))
	for nodeID, bucket := range nodeRulesPtr {
		nodeRules[nodeID] = *bucket
	}

	return models.EvaluationResult{
		AllowByNs:      allowByNsRules,
		DenyByNs:       denyByNsRules,
		PolicyStatuses: policyStatuses,
		NodePolicies:   nodePolicies,
		NodeRules:      nodeRules,
	}, nil
}

// getPolicies fetches AuthorizationPolicies from every requested namespace
// via the shared k8s client. Logs the failing ns before returning so an
// operator can pinpoint which namespace stopped the whole engine.
func (s *source) getPolicies(namespaces []string) (map[string][]*istiosec.AuthorizationPolicy, error) {
	policiesByNS := map[string][]*istiosec.AuthorizationPolicy{}
	for _, ns := range namespaces {
		nsPolicies, err := s.client.GetAuthorizationPolicies(ns)
		if err != nil {
			s.log.Warn("istio policy fetch failed",
				slog.String("phase", "istio_get_policies"),
				slog.String("engine", sourceName),
				slog.String("ns", ns),
				slog.String("error", err.Error()),
			)
			return nil, err
		}
		policiesByNS[ns] = nsPolicies
	}
	return policiesByNS, nil
}

// splitPoliciesByAction partitions policies into ALLOW and DENY buckets.
// AUDIT (2) and CUSTOM (3) actions are dropped — they don't shape L3 edges.
func splitPoliciesByAction(policiesByNS map[string][]*istiosec.AuthorizationPolicy) (allow, deny map[string][]*istiosec.AuthorizationPolicy) {
	allow = map[string][]*istiosec.AuthorizationPolicy{}
	deny = map[string][]*istiosec.AuthorizationPolicy{}
	for ns, policies := range policiesByNS {
		for _, authzPolicy := range policies {
			switch authzPolicy.Spec.Action {
			case 0: // ALLOW
				allow[ns] = append(allow[ns], authzPolicy)
			case 1: // DENY
				deny[ns] = append(deny[ns], authzPolicy)
			}
		}
	}
	return allow, deny
}
