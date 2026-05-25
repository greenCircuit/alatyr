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