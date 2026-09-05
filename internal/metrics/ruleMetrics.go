package metrics

import "alatyr/internal/models"

// recordRulMetrics records rule-derived metrics in one pass over every
// engine's AllowByNs + DenyByNs.
//
// alatyr_rule_edges is a raw post-expansion count — one podSelector:{}
// manifest fans into src×dst rules, so numbers reflect graph edges, not
// policy objects. Sum-safe across any label subset.
//
// alatyr_rule_coverage dedups by (engine, ns, name, action, coverage,
// direction) so a single policy counts once per bucket it touches. NOT
// sum-safe — sum inflates policies that span buckets.
func (r *Recorder) recordRulMetrics(cache *models.Cache) {
	edges := map[[5]string]int{}
	seen := map[string]struct{}{}
	counts := map[policyCoverageKey]int{}

	for engine, eval := range cache.EvaluationResults {
		for _, rulesByNs := range []map[string][]models.Rule{eval.AllowByNs, eval.DenyByNs} {
			for _, rules := range rulesByNs {
				for _, rule := range rules {
					ref := rule.Contributor
					namespace := ref.Namespace
					if namespace == "" {
						namespace = ClusterScopeNamespace
					}
					action := ref.Action
					if action == "" {
						if rule.Action == models.ActionDeny {
							action = "deny"
						} else {
							action = "allow"
						}
					}
					coverage := string(rule.Coverage)
					direction := string(rule.Direction)

					covBucket, dirBucket := coverage, direction
					if covBucket == "" {
						covBucket = "unknown"
					}
					if dirBucket == "" {
						dirBucket = "unknown"
					}
					edges[[5]string{namespace, engine, action, covBucket, dirBucket}]++

					if ref.Name == "" || coverage == "" {
						continue
					}
					dedupKey := engine + "|" + namespace + "|" + ref.Name + "|" +
						action + "|" + coverage + "|" + direction
					if _, dup := seen[dedupKey]; dup {
						continue
					}
					seen[dedupKey] = struct{}{}
					counts[policyCoverageKey{
						namespace: namespace,
						engine:    engine,
						action:    action,
						coverage:  coverage,
						direction: direction,
					}]++
				}
			}
		}
	}

	for key, count := range edges {
		r.ruleEdges.WithLabelValues(key[0], key[1], key[2], key[3], key[4]).Set(float64(count))
	}
	for key, count := range counts {
		r.policyCoverage.WithLabelValues(
			key.namespace, key.engine, key.action, key.coverage, key.direction,
		).Set(float64(count))
	}
}
