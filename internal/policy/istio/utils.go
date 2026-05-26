package istio
import (
		
	istioapi "istio.io/api/security/v1beta1"
	istioapitype "istio.io/api/type/v1beta1"
)

// isCatchAllSelector reports whether the WorkloadSelector applies to every
// workload in the namespace (nil OR empty MatchLabels).
func isCatchAllSelector(selector *istioapitype.WorkloadSelector) bool {
	if selector == nil {
		return true
	}
	return len(selector.MatchLabels) == 0
}

// ruleMatchesAnySource reports whether a Rule's From list permits any source.
// True when From is empty (no source constraint) or any entry has a nil
// Source (wildcard within the OR'd From list).
func ruleMatchesAnySource(rule *istioapi.Rule) bool {
	if len(rule.From) == 0 {
		return true
	}
	for _, from := range rule.From {
		if from.Source == nil {
			return true
		}
	}
	return false
}

// ruleHasL7Match reports whether a Rule's To list carries any L7 matcher
// (hosts/methods/paths or their notX exclusions). Drives the StatusL7Applied
// badge so users can see at a glance which workloads have L7 gating.
func ruleHasL7Match(rule *istioapi.Rule) bool {
	for _, to := range rule.To {
		if to.Operation == nil {
			continue
		}
		op := to.Operation
		if len(op.Hosts)+len(op.Methods)+len(op.Paths)+
			len(op.NotHosts)+len(op.NotMethods)+len(op.NotPaths) > 0 {
			return true
		}
	}
	return false
}