package istio

import (
	"time"

	"graph/internal/models"
	"graph/internal/utils"

	istioapi "istio.io/api/security/v1beta1"
	istiosec "istio.io/client-go/pkg/apis/security/v1"
)

// creationTime returns a pointer to the policy's CreationTimestamp, or nil
// when unset — omitempty then drops it from the wire rather than marshaling
// year 0001.
func creationTime(ap *istiosec.AuthorizationPolicy) *time.Time {
	if ap.CreationTimestamp.IsZero() {
		return nil
	}
	t := ap.CreationTimestamp.Time
	return &t
}

// Note: Istio AuthorizationPolicy is ingress-only at the L3 layer — it gates
// traffic INTO the selected workload. Egress is handled by Sidecar /
// ServiceEntry resources (out of scope here). All produced rules use
// allow and deny rules will use this since the way they are working are the same
// buildRulesByNs expands AuthorizationPolicies into rules, grouped by the
// policy's namespace for per-ns cache invalidation.
// buildRulesByNs also accumulates the per-node rule index into nodeRules
// (ingress-only for Istio), keyed by the protected workload. Runs once per
// action bucket; pass the same nodeRules map to merge allow + deny.
func buildRulesByNs(index map[string]models.NSIndex, policiesByNS map[string][]*istiosec.AuthorizationPolicy, nodeRules map[string]*models.NodeRules) map[string][]models.Rule {
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

					// index by the protected node (DstID after swap), ingress-only
					protectedID := rule.DstID
					bucket, ok := nodeRules[protectedID]
					if !ok {
						bucket = &models.NodeRules{}
						nodeRules[protectedID] = bucket
					}
					if rule.Action == models.ActionAllow {
						bucket.Ingress.Allow = append(bucket.Ingress.Allow, rule)
					} else {
						bucket.Ingress.Deny = append(bucket.Ingress.Deny, rule)
					}
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

// spec.rules: {}
func expandBlanketRule(authzPolicy *istiosec.AuthorizationPolicy) models.Rule{
		ruleAction := actionFromSpec(authzPolicy.Spec.Action) // allow vs deny
		// field is not even defined
		var computedCoverage models.Coverage
		if ruleAction == models.ActionAllow {
			computedCoverage = models.CoverageDenyAll
		} else {
			computedCoverage = models.CoverageUnenforced
		}

		rule := models.Rule{
			Direction:   models.DirectionIngress,
			Action:      ruleAction,
			AllPorts:    true,
			AllL7:       true,
			Coverage:    computedCoverage,
		}
		return rule
}

// spec.rules: - {}
func expandCatchAllRule(authzPolicy *istiosec.AuthorizationPolicy) models.Rule{
		ruleAction := actionFromSpec(authzPolicy.Spec.Action) // allow vs deny
		// field is not even defined
		var computedCoverage models.Coverage
		if ruleAction == models.ActionAllow {
			computedCoverage = models.CoverageUnenforced
		} else {
			computedCoverage = models.CoverageDenyAll
		}

		rule := models.Rule{
			Direction:   models.DirectionIngress,
			Action:      ruleAction,
			AllPorts:    true,
			AllL7:       true,
			Coverage:    computedCoverage,
		}
		return rule
}

// spec.rules[to] = {}
func isEmptyToOperation(toBlock *istioapi.Rule_To) bool {
	if toBlock == nil || toBlock.Operation == nil {
		return true
	}
	operation := toBlock.Operation
	return len(operation.Hosts) == 0 && len(operation.NotHosts) == 0 &&
		len(operation.Ports) == 0 && len(operation.NotPorts) == 0 &&
		len(operation.Methods) == 0 && len(operation.NotMethods) == 0 &&
		len(operation.Paths) == 0 && len(operation.NotPaths) == 0
}

// spec.rules[].to all operations empty (no port/L7 narrowing)
func isEmptyToOperations(to []*istioapi.Rule_To) bool {
	for _, toBlock := range to {
		if !isEmptyToOperation(toBlock) {
			return false
		}
	}
	return true
}

func expandRules(authzPolicy *istiosec.AuthorizationPolicy, index map[string]models.NSIndex) []models.Rule {
	var rulesMatrix []models.Rule
	var dstSelector models.PolicySelector
	if authzPolicy.Spec.Selector != nil && authzPolicy.Spec.Selector.MatchLabels != nil {
		dstSelector.LabelSelector = authzPolicy.Spec.Selector.MatchLabels
	}
	
	rules := authzPolicy.Spec.Rules
	ruleAction := actionFromSpec(authzPolicy.Spec.Action) // allow vs deny

	// no rules section defined will be deny all policy
	// spec.rules: {}
	if len(rules) == 0 {
		contributor := models.PolicyRef{
			Source:    sourceName,
			Name:      authzPolicy.Name,
			Namespace: authzPolicy.Namespace,
			CreatedAt: creationTime(authzPolicy),
		}
		rule := expandBlanketRule(authzPolicy)
		rule.Contributor = contributor
		rulesMatrix = append(rulesMatrix, rule)
	}

	for _, rule := range rules {
		var matchWorkloads []models.WorkloadNode
		nsFrom := []string{}
		for _, fromBlock := range rule.From {
			workloads := expandFromSource(fromBlock.Source, authzPolicy.Namespace, index)
			matchWorkloads = append(matchWorkloads, workloads...)
			if fromBlock.Source != nil && fromBlock.Source.Namespaces != nil {
				nsFrom = append(nsFrom, fromBlock.Source.Namespaces...)
			}
		}
		var srcSelector models.PolicySelector
		srcSelector.Namespaces = nsFrom

		contributor := models.PolicyRef{
			Source:    sourceName,
			Name:      authzPolicy.Name,
			Namespace: authzPolicy.Namespace,
			CreatedAt: creationTime(authzPolicy),
		}

		// spec.rules: - {}, it is array with empty rule defined
		if len(rules[0].To) == 0 && len(rules[0].From) == 0 {
			rule := expandCatchAllRule(authzPolicy)
			rule.Contributor = contributor
			rulesMatrix = append(rulesMatrix, rule)
			continue
		}

		// no extra rules will assume that allow/deny all
		if len(rule.To) == 0 {
			var coverageComputed models.Coverage
			if ruleAction == models.ActionAllow {
				coverageComputed = models.CoverageAllowAll
			} else {
				coverageComputed = models.CoverageUnenforced
			}
			for _, workload := range matchWorkloads {
				rulesMatrix = append(rulesMatrix, models.Rule{
					DstID:       workload.ID,
					Direction:   models.DirectionIngress,
					Contributor: contributor,
					Action:      ruleAction,
					AllPorts:    true,
					AllL7:       true,
					SrcSelector: srcSelector,
					DstSelector: dstSelector,
					Coverage:    coverageComputed,
				})
			}
			continue
		}

		// no from = any source; empty To = no port/L7 narrowing → allow-all.
		// matchWorkloads is empty here, so the matrix-mult below emits nothing;
		// synthesize one marker. DstID stays "" — buildRulesByNs swap fills it
		// from the selected workload (getSourceNodes), SrcID stays "" = any source.
		if len(rule.From) == 0 && isEmptyToOperations(rule.To) {
			coverage := models.CoverageAllowAll
			if ruleAction == models.ActionDeny {
				coverage = models.CoverageDenyAll
			}
			rulesMatrix = append(rulesMatrix, models.Rule{
				Direction:   models.DirectionIngress,
				Contributor: contributor,
				Action:      ruleAction,
				AllPorts:    true,
				AllL7:       true,
				DstSelector: dstSelector,
				Coverage:    coverage,
			})
			continue
		}
		// have to blocks
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

			// any source (no from) + narrowed To. no workloads to multiply
			// against; emit one marker. DstID stays "" → buildRulesByNs swap
			// fills it from the selected workload.
			if len(matchWorkloads) == 0 {
				allPorts := len(rulePorts) == 0

				// ports-only narrowing = still allow-all shape; L7 = restricted
				coverage := models.CoverageAllowAll
				if !ruleL7.IsEmpty() {
					coverage = models.CoverageRestricted
				}

				rulesMatrix = append(rulesMatrix, models.Rule{
					Direction:   models.DirectionIngress,
					Contributor: contributor,
					Action:      ruleAction,
					L7Match:     l7Ptr,
					AllPorts:    allPorts,
					AllL7:       allL7,
					SrcSelector: srcSelector,
					DstSelector: dstSelector,
					Ports:       rulePorts,
					Coverage:    coverage,
				})
				continue
			}
			// do matrix multiplication to find all rules that can exists from this policy
			for _, workload := range matchWorkloads {
				var allPorts bool
				if len(rulePorts) == 0 {
					allPorts = true
				}
				
				// determine what type of coverage depending on node type
				coverage := models.CoverageRestricted  // default
				if workload.Type == models.NodeTypeNamespace {
					coverage = models.CoverageAllowAllNs
				}

				rulesMatrix = append(rulesMatrix, models.Rule{
					DstID:       workload.ID,
					Direction:   models.DirectionIngress,
					Contributor: contributor,
					Action:      ruleAction,
					L7Match:     l7Ptr,
					AllPorts:    allPorts,
					AllL7:       allL7,
					SrcSelector: srcSelector,
					DstSelector: dstSelector,
					Ports: 		 rulePorts,		
					Coverage:    coverage,				
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
