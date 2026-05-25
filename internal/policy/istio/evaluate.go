package istio

import (
	"context"

	"graph/internal/k8s"
	"graph/internal/models"
	"graph/internal/policy"

	istiosec "istio.io/client-go/pkg/apis/security/v1"
)

const sourceName = "istio"

type source struct {
	client k8s.KubernetesClient
}

func New(client k8s.KubernetesClient) *source {
	return &source{client: client}
}

func (s *source) Name() string {
	return sourceName
}

// Coverage returns whether Istio AuthorizationPolicy applies to this workload.
// TODO: return CoverageNotApplicable for workloads not in the mesh (no sidecar
// injected — check for `istio-injection: enabled` on the namespace label or
// the `sidecar.istio.io/inject` annotation on the pod). For now assume
// DefaultAllow on every workload.
func (s *source) Coverage(node *models.WorkloadNode, dir models.Direction) policy.CoverageMode {
	return policy.CoverageDefaultAllow
}

// Evaluate fetches AuthorizationPolicies for the given namespaces, splits them
// by action, and returns Allow rules, Deny rules, and per-workload PolicyStatus.
// Root-namespace (mesh-wide) policies feed PolicyStatus but do not yet drive
// rules — buildRules selects workloads by the policy's own namespace, so
// applying a root-ns policy to every namespace needs a separate fan-out (TODO).
func (s *source) Evaluate(_ context.Context, namespaces []string, index map[string]models.NSIndex) policy.EvaluationResult {
	policiesByNS, _ := s.getPolicies(namespaces)
	globalPolicies, _ := s.client.GetAuthorizationPolicies(RootNamespace)

	allowByNS, denyByNS := splitPoliciesByAction(policiesByNS)

	policyStatuses := map[string]models.PolicyStatus{}
	for _, ns := range namespaces {
		assignments := generatePolicyStatusAssignment(index[ns].Workloads, policiesByNS[ns], globalPolicies)
		for nodeID, status := range assignments {
			policyStatuses[nodeID] = status
		}
	}

	denyRules := buildRules(index, denyByNS)
	for ruleIndex := range denyRules {
		denyRules[ruleIndex].Action = policy.ActionDeny
	}

	return policy.EvaluationResult{
		Allow:          buildRules(index, allowByNS),
		Deny:           denyRules,
		PolicyStatuses: policyStatuses,
	}
}

// getPolicies fetches AuthorizationPolicies from every requested namespace
// via the shared k8s client.
func (s *source) getPolicies(namespaces []string) (map[string][]*istiosec.AuthorizationPolicy, error) {
	policiesByNS := map[string][]*istiosec.AuthorizationPolicy{}
	for _, ns := range namespaces {
		nsPolicies, err := s.client.GetAuthorizationPolicies(ns)
		if err != nil {
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
