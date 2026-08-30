package calico

import (
	"fmt"
	"math"
	"sort"
	"strings"

	calicoselector "graph/internal/thirdparty/calicoselector"
)

// catchAllCIDR — bucket for a rule with empty nets ({}) or explicit 0.0.0.0/0.
const catchAllCIDR = "0.0.0.0/0"

// normalizeOrder — unset order sorts last. (verify semantics — ADR 0005 q#2)
func normalizeOrder(order *float64) float64 {
	if order == nil {
		return math.MaxFloat64
	}
	return *order
}

// sortByPrecedence — tier, then order within tier, then name for determinism.
// (verify tier ordering + default-tier position — ADR 0005)
func sortByPrecedence(policies []globalPolicy) {
	sort.SliceStable(policies, func(left, right int) bool {
		if policies[left].tier != policies[right].tier {
			return policies[left].tier < policies[right].tier
		}
		if policies[left].order != policies[right].order {
			return policies[left].order < policies[right].order
		}
		return policies[left].name < policies[right].name
	})
}

// isCatchAllSelector — Calico treats an empty selector and all() as match-all.
// Anything else (even a selector that happens to match every current pod) is a
// label claim, not a namespace-wide one.
func isCatchAllSelector(expr string) bool {
	trimmed := strings.TrimSpace(expr)
	return trimmed == "" || trimmed == "all()"
}

// parseSelector compiles a Calico selector expression into a label matcher via
// the vendored libcalico parser (internal/thirdparty/calicoselector). Empty and
// all() both collapse to match-all upstream. Parse errors propagate so a
// malformed selector fails loud instead of silently under-matching. sel.Evaluate
// has the workloadMatcher signature, so it's returned directly.
func parseSelector(expr string) (workloadMatcher, error) {
	sel, err := calicoselector.Parse(expr)
	if err != nil {
		return nil, fmt.Errorf("calico selector %q: %w", expr, err)
	}
	return sel.Evaluate, nil
}
