package istio

import (
	"graph/internal/models"
	"graph/internal/policy"
	"graph/internal/utils"

	istioapi "istio.io/api/security/v1beta1"
	istiosec "istio.io/client-go/pkg/apis/security/v1"
)


// Note: Istio AuthorizationPolicy is ingress-only at the L3 layer — it gates
// traffic INTO the selected workload. Egress is handled by Sidecar /
// ServiceEntry resources (out of scope here). All produced rules use
// allow and deny rules will use this since the way they are working are the same
func buildRules(index map[string]models.NSIndex, policiesByNS map[string][]*istiosec.AuthorizationPolicy) []policy.Rule {
	var allRules []policy.Rule
	for _, policies := range policiesByNS {
		for _, authzPolicy := range policies {
			srcNodes := getSourceNodes(authzPolicy, index)  // find all nodes that this policy will be applied to
			ingressRules := expandRules(authzPolicy, index) // pass all rules from single policy, and find out what are riles will be
			// do a matrix multiplication to for source node x all policies to get the
			// final result how edges/final polices will look like
			for _, srcNode := range srcNodes {
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

func getSourceNodes(authzPolicy *istiosec.AuthorizationPolicy, index map[string]models.NSIndex) []*models.WorkloadNode {
	nsIndex := index[authzPolicy.Namespace]
	// nil OR empty MatchLabels → every workload in ns
	if isCatchAllSelector(authzPolicy.Spec.Selector) {
		result := make([]*models.WorkloadNode, len(nsIndex.Workloads))
		for index := range nsIndex.Workloads {
			result[index] = &nsIndex.Workloads[index]
		}
		return result
	}
	return utils.IndexLabelMatch(authzPolicy.Spec.Selector.MatchLabels, nsIndex.LabelIndex)
}

func expandRules(authzPolicy *istiosec.AuthorizationPolicy, index map[string]models.NSIndex) []policy.Rule {
	var rulesMatrix []policy.Rule
	rules := authzPolicy.Spec.Rules
	for ruleIndex, rule := range rules {
		var rulePorts []models.Port
		var ruleL7 policy.L7Match
		var matchWorkloads []models.WorkloadNode

		for _, fromBlock := range rule.From {
			matchWorkloads = expandFromSource(fromBlock.Source, authzPolicy.Namespace, index)
		}
		for _, toBlock := range rule.To {
			ports, l7policies := expandToOperation(toBlock.Operation)
			rulePorts = ports
			ruleL7 = l7policies
		}

		contributors := []policy.PolicyRef{{
			Source:    sourceName,
			Name:      authzPolicy.Name,
			Namespace: authzPolicy.Namespace,
			RuleIndex: ruleIndex,
		}}

		var l7Ptr *policy.L7Match
		if !ruleL7.IsEmpty() {
			l7Copy := ruleL7
			l7Ptr = &l7Copy
		}

		// do matrix multiplication to find all rules that can exists from this policy
		for _, workload := range matchWorkloads {
			if len(rulePorts) == 0 {
				rulesMatrix = append(rulesMatrix, policy.Rule{
					DstID:        workload.ID,
					Direction:    models.DirectionIngress,
					Contributors: contributors,
					L7Match:      l7Ptr,
				})
				continue
			}
			for _, port := range rulePorts {
				rulesMatrix = append(rulesMatrix, policy.Rule{
					DstID:        workload.ID,
					Port:         port,
					Direction:    models.DirectionIngress,
					Contributors: contributors,
					L7Match:      l7Ptr,
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
func expandToOperation(operation *istioapi.Operation) ([]models.Port, policy.L7Match) {
	var ports []models.Port
	var l7Match policy.L7Match
	if len(operation.Ports) != 0 {
		converted := convertPorts(operation.Ports)
		ports = converted
	}
	for _,host :=range operation.Hosts{
		l7Match.Hosts = append(l7Match.Hosts, host)
	}

	for _,method :=range operation.Methods{
		l7Match.Methods = append(l7Match.Hosts, method)
	}

	for _,path :=range operation.Paths{
		l7Match.Paths = append(l7Match.Paths, path)
	}

	return  ports, l7Match
}

// getSelectedWorkloads returns the workloads that an AuthorizationPolicy's
// WorkloadSelector matches in its namespace. Nil selector = every workload
// in the namespace (defaults to NS node when collapsing).
func getSelectedWorkloads(authzPolicy *istiosec.AuthorizationPolicy, index map[string]models.NSIndex) []*models.WorkloadNode {
	return nil
}
