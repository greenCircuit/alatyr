package metrics

import "graph/internal/models"

// Workload-derived metrics live here: the per-namespace census
// (alatyr_workloads), status + exposure breakdowns, and every coverage gauge.
//
// Coverage is asked twice on purpose. alatyr_workloads_unpoliced answers "is
// any policy selecting this workload at all", which is the honest literal
// answer. alatyr_workloads_unpoliced_excluding_global answers "is anything
// other than a cluster-wide catch-all selecting it" — on a cluster running
// Calico GlobalNetworkPolicy the first gauge is pinned at zero forever, since
// the global policy selects every pod, and a gauge that can never fire is
// worse than no gauge at all.

// recordWorkloads walks every namespace once and stamps the exposure +
// status gauges. Only real workloads count — synthetic namespace nodes
// (Type NodeTypeNamespace) and CIDR nodes are excluded so the denominator
// stays "workloads a policy could actually select".
func (r *Recorder) recordWorkloads(cache *models.Cache, enabledEngines []string) {
	for ns, nsIndex := range cache.NsIndex {
		total := 0
		byStatus := map[models.StatusKey]int{}
		unpoliced := 0
		unpolicedExcludingGlobal := 0
		internetIn, internetOut, internetBoth := 0, 0, 0
		for i := range nsIndex.Workloads {
			workload := &nsIndex.Workloads[i]
			if !isCountableWorkload(workload) {
				continue
			}
			total++
			hasInternetIn, hasInternetOut, hasInternetFull := false, false, false
			for _, status := range workload.Statuses {
				byStatus[status]++
				switch status {
				case models.StatusInternetIngress:
					hasInternetIn = true
				case models.StatusInternetEgress:
					hasInternetOut = true
				case models.StatusInternetFull:
					hasInternetFull = true
				}
			}
			switch {
			case hasInternetFull:
				internetBoth++
			case hasInternetIn && hasInternetOut:
				internetBoth++
			case hasInternetIn:
				internetIn++
			case hasInternetOut:
				internetOut++
			}
			if !isCoveredByAnyEngine(cache, workload.ID, enabledEngines, anyPolicy) {
				unpoliced++
			}
			if !isCoveredByAnyEngine(cache, workload.ID, enabledEngines, namespacedPolicy) {
				unpolicedExcludingGlobal++
			}
			if r.workloadStatus != nil {
				for _, status := range workload.Statuses {
					r.workloadStatus.WithLabelValues(ns, workload.Label, string(status)).Set(1)
				}
			}
		}
		r.workloads.WithLabelValues(ns).Set(float64(total))
		for status, count := range byStatus {
			r.workloadsByStatus.WithLabelValues(ns, string(status)).Set(float64(count))
		}
		r.workloadsUnpoliced.WithLabelValues(ns).Set(float64(unpoliced))
		r.workloadsUnpolicedExclGlobal.WithLabelValues(ns).Set(float64(unpolicedExcludingGlobal))
		if internetIn > 0 {
			r.workloadsInternetReach.WithLabelValues(ns, "ingress").Set(float64(internetIn))
		}
		if internetOut > 0 {
			r.workloadsInternetReach.WithLabelValues(ns, "egress").Set(float64(internetOut))
		}
		if internetBoth > 0 {
			r.workloadsInternetReach.WithLabelValues(ns, "both").Set(float64(internetBoth))
		}
	}
}

// recordCoverage stamps the per-engine "did any policy select this workload"
// gauges plus the single-engine view. "Covered by all engines" is intentionally
// not emitted — not every workload participates in every engine (Istio-only
// workloads should not count as "missing" k8s NetworkPolicy coverage).
func (r *Recorder) recordCoverage(cache *models.Cache, enabledEngines []string) {
	if len(enabledEngines) == 0 {
		return
	}
	for ns, nsIndex := range cache.NsIndex {
		perEngine := map[string]int{}
		singleEngine := 0
		for i := range nsIndex.Workloads {
			workload := &nsIndex.Workloads[i]
			if !isCountableWorkload(workload) {
				continue
			}
			coveringEngines := 0
			for _, engine := range enabledEngines {
				if isCoveredByEngine(cache, workload.ID, engine, anyPolicy) {
					perEngine[engine]++
					coveringEngines++
				}
			}
			if coveringEngines == 1 {
				singleEngine++
			}
		}
		for _, engine := range enabledEngines {
			r.workloadsCovered.WithLabelValues(ns, engine).Set(float64(perEngine[engine]))
		}
		r.workloadsSingleEngineCover.WithLabelValues(ns).Set(float64(singleEngine))
	}
}

// policyFilter decides which selecting policies count as coverage.
type policyFilter func(models.PolicyRef) bool

// anyPolicy counts every selecting policy — the literal "is anything looking
// at this workload" question.
func anyPolicy(models.PolicyRef) bool { return true }

// namespacedPolicy ignores cluster-scoped manifests (Calico
// GlobalNetworkPolicy, and any future cluster-wide kind), which is how a
// PolicyRef with no Namespace reaches us. One global catch-all otherwise marks
// every pod in the cluster as covered and hides the workloads that have no
// namespace-local policy of their own.
func namespacedPolicy(ref models.PolicyRef) bool { return ref.Namespace != "" }

// isCountableWorkload filters out synthetic nodes so aggregate ratios stay
// meaningful. Namespace nodes and CIDR peers are graph fixtures, not
// workloads a policy could select.
func isCountableWorkload(workload *models.WorkloadNode) bool {
	if workload == nil {
		return false
	}
	switch workload.Type {
	case models.NodeTypeNamespace, models.NodeTypeCIDR, models.NodeTypeExternal:
		return false
	}
	return true
}

// isCoveredByAnyEngine reports whether at least one enabled engine has a
// selecting policy for the workload that passes the filter. Iterates only
// enabled engines so a disabled engine's stale NodePolicies entries don't mask
// an unpoliced workload.
func isCoveredByAnyEngine(cache *models.Cache, workloadID string, engines []string, counts policyFilter) bool {
	for _, engine := range engines {
		if isCoveredByEngine(cache, workloadID, engine, counts) {
			return true
		}
	}
	return false
}

func isCoveredByEngine(cache *models.Cache, workloadID string, engine string, counts policyFilter) bool {
	eval, ok := cache.EvaluationResults[engine]
	if !ok {
		return false
	}
	for _, ref := range eval.NodePolicies[workloadID] {
		if counts(ref) {
			return true
		}
	}
	return false
}
