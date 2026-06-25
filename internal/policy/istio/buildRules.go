package istio

import (
	"graph/internal/models"
	"graph/internal/utils"

	istioapi "istio.io/api/security/v1beta1"
	istiosec "istio.io/client-go/pkg/apis/security/v1"
)


// Note: Istio AuthorizationPolicy is ingress-only at the L3 layer — it gates
// traffic INTO the selected workload. Egress is handled by Sidecar /
// ServiceEntry resources (out of scope here). All produced rules use
// allow and deny rules will use this since the way they are working are the same
// buildRulesByNs expands AuthorizationPolicies into rules, grouped by the
// policy's namespace for per-ns cache invalidation.
func buildRulesByNs(index map[string]models.NSIndex, policiesByNS map[string][]*istiosec.AuthorizationPolicy) map[string][]models.Rule {
	result := map[string][]models.Rule{}
	for ns, policies := range policiesByNS {
		var nsRules []models.Rule
		for _, authzPolicy := range policies {
			srcNodes := getSourceNodes(authzPolicy, index)  // find all nodes that this policy will be applied to
			ingressRules := expandRules(authzPolicy, index) // pass all rules from single policy, and find out what are riles will be
			// do a matrix multiplication to for source node x all policies to get the
			// final result how edges/final polices will look like
			for _, srcNode := range srcNodes {
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

func getSourceNodes(authzPolicy *istiosec.AuthorizationPolicy, index map[string]models.NSIndex) []*models.WorkloadNode {
	nsIndex := index[authzPolicy.Namespace]
	// nil OR empty MatchLabels → every workload in ns
	if isCatchAllSelector(authzPolicy.Spec.Selector) {
		result := make([]*models.WorkloadNode, len(nsIndex.Workloads))
		for position := range nsIndex.Workloads {
			result[position] = &nsIndex.Workloads[position]
		}
		return result
	}
	return utils.IndexLabelMatch(authzPolicy.Spec.Selector.MatchLabels, nsIndex.LabelIndex)
}

func expandRules(authzPolicy *istiosec.AuthorizationPolicy, index map[string]models.NSIndex) []models.Rule {
	var rulesMatrix []models.Rule
	rules := authzPolicy.Spec.Rules
	ruleAction := actionFromSpec(authzPolicy.Spec.Action)
	for ruleIndex, rule := range rules {
		var matchWorkloads []models.WorkloadNode

		for _, fromBlock := range rule.From {
			workloads := expandFromSource(fromBlock.Source, authzPolicy.Namespace, index)
			matchWorkloads = append(matchWorkloads, workloads...)
		}

		contributor := models.PolicyRef{
			Source:    sourceName,
			Name:      authzPolicy.Name,
			Namespace: authzPolicy.Namespace,
			RuleIndex: ruleIndex,
		}

		if len(rule.To) == 0 {
			for _, workload := range matchWorkloads {
				rulesMatrix = append(rulesMatrix, models.Rule{
					DstID:       workload.ID,
					Direction:   models.DirectionIngress,
					Contributor: contributor,
					Action:      ruleAction,
					AllPorts:    true,
					AllL7:       true,
				})
			}
			continue
		}

		for _, toBlock := range rule.To {
			var rulePorts []models.Port
			var ruleL7 models.L7Match
			ports, l7policies := expandToOperation(toBlock.Operation)
			rulePorts = ports
			ruleL7 = l7policies
			allL7 := true
			var l7Ptr *models.L7Match

			// have l7 policies
			if !ruleL7.IsEmpty() {
				l7Copy := ruleL7
				l7Ptr = &l7Copy
				allL7 = false
			}
			

			// do matrix multiplication to find all rules that can exists from this policy
			for _, workload := range matchWorkloads {
				if len(rulePorts) == 0 {
					rulesMatrix = append(rulesMatrix, models.Rule{
						DstID:       workload.ID,
						Direction:   models.DirectionIngress,
						Contributor: contributor,
						Action:      ruleAction,
						L7Match:     l7Ptr,
						AllPorts:    true,
						AllL7:       allL7,
					})
					continue
				}

				rulesMatrix = append(rulesMatrix, models.Rule{
					DstID:       workload.ID,
					Ports:       rulePorts,
					Direction:   models.DirectionIngress,
					Contributor: contributor,
					Action:      ruleAction,
					L7Match:     l7Ptr,
					AllPorts:    false,
					AllL7:       allL7,
				})
			}
		}
	}

	return rulesMatrix
}

// find all workloads, that from single source policy
func expandFromSource(src *istioapi.Source, policyNamespace string, index map[string]models.NSIndex) []models.WorkloadNode {
	var workloads []models.WorkloadNode
	if src == nil {
		return workloads
	}
	if len(src.Namespaces) != 0 {
		for _, ns := range src.Namespaces {
			_, exists := index[ns]
			if exists {
				matchWorkload := index[ns].NSNode
				workloads = append(workloads, *matchWorkload)
			}
		}
	}
	return workloads
}

// all workloads that match single operation
func expandToOperation(operation *istioapi.Operation) ([]models.Port, models.L7Match) {
	var ports []models.Port
	var l7Match models.L7Match
	if len(operation.Ports) != 0 {
		converted := convertPorts(operation.Ports)
		ports = converted
	}
	l7Match.Hosts = append(l7Match.Hosts, operation.Hosts...)

	l7Match.Methods = append(l7Match.Methods, operation.Methods...)

	l7Match.Paths = append(l7Match.Paths, operation.Paths...)

	return ports, l7Match
}
