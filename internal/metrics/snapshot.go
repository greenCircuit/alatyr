package metrics

import (
	"time"

	"graph/internal/models"
)

// RecordSnapshot rebuilds every gauge derived from a completed cache from
// scratch. Called by the snapshot loop after the new cache is swapped in.
//
// Reset-then-repopulate on purpose: gauges are a current-state census, so
// tuples for a deleted namespace or a status that no workload carries
// anymore must disappear this cycle. DeletePartialMatch would be finer
// but would need to track every prior label combo — Reset is correct and
// cheap for this size.
//
// Concurrency: safe to call from the snapshot goroutine as long as the
// cache pointer is the freshly-swapped one (immutable post-swap in
// api.Server.RefreshAll's contract). Never call with a cache that may
// still be mutated.
func (r *Recorder) RecordSnapshot(cache *models.Cache, issues []models.Issue, enabledEngines []string) {
	if cache == nil {
		return
	}
	r.resetSnapshotVecs()
	r.recordWorkloads(cache, enabledEngines)
	r.recordCoverage(cache, enabledEngines)
	r.recordPolicies(cache)
	r.recordRulMetrics(cache)
	r.recordIssues(issues)
	r.recordPolicyLayering(issues)
	r.recordMesh(cache)
	r.evalTimestamp.Set(float64(time.Now().Unix()))
}

// resetSnapshotVecs clears every gauge repopulated by RecordSnapshot.
// Counters + histograms + long-lived gauges (engine_enabled, build_info,
// informer_cache_synced, evaluation_timestamp) are excluded on purpose.
func (r *Recorder) resetSnapshotVecs() {
	r.workloads.Reset()
	r.workloadsByStatus.Reset()
	r.workloadsUnpoliced.Reset()
	r.workloadsUnpolicedExclGlobal.Reset()
	r.workloadsInternetReach.Reset()
	r.workloadsCovered.Reset()
	r.workloadsSingleEngineCover.Reset()
	r.policies.Reset()
	r.resetIssueVecs()
	r.resetPolicyLayeringVecs()
	r.meshWorkloads.Reset()
	r.meshMtls.Reset()
	r.meshHboneBlocked.Reset()
	if r.workloadStatus != nil {
		r.workloadStatus.Reset()
	}
	r.ruleEdges.Reset()
	r.policyCoverage.Reset()
}

// policyLabelKey is the (namespace, engine) tuple emitted as the
// alatyr_policies label set. Action lives on alatyr_rule_edges — some engines
// (Calico, Kyverno) mix allow+deny inside a single manifest, so keying policy
// counts by action would double-count manifests.
type policyLabelKey struct {
	namespace string
	engine    string
}

// recordPolicies dedupes policy manifests across every rule that references
// them, keyed by (engine, namespace, name). Reads AllowByNs + DenyByNs
// directly — same source renderEdges walks — so a manifest that produces
// both allow and deny rules still counts once.
func (r *Recorder) recordPolicies(cache *models.Cache) {
	seen := map[string]struct{}{}
	counts := map[policyLabelKey]int{}
	for engine, eval := range cache.EvaluationResults {
		for _, rules := range eval.AllowByNs {
			for _, rule := range rules {
				bumpPolicy(seen, counts, engine, rule)
			}
		}
		for _, rules := range eval.DenyByNs {
			for _, rule := range rules {
				bumpPolicy(seen, counts, engine, rule)
			}
		}
	}
	for key, count := range counts {
		r.policies.WithLabelValues(key.namespace, key.engine).Set(float64(count))
	}
}

// bumpPolicy dedupes one rule's contributor into the manifest count. Keyed by
// (engine, ns, name) so each manifest counts once regardless of how many rules
// it produced or which actions those rules carried. Cluster-scoped policies
// (empty ref.Namespace) are stamped with ClusterScopeNamespace so the null
// doesn't render as a phantom "0" namespace on the dashboard.
func bumpPolicy(seen map[string]struct{}, counts map[policyLabelKey]int, engine string, rule models.Rule) {
	ref := rule.Contributor
	if ref.Name == "" {
		return
	}
	namespace := ref.Namespace
	if namespace == "" {
		namespace = ClusterScopeNamespace
	}
	dedupKey := engine + "|" + namespace + "|" + ref.Name
	if _, ok := seen[dedupKey]; ok {
		return
	}
	seen[dedupKey] = struct{}{}
	counts[policyLabelKey{namespace: namespace, engine: engine}]++
}

// recordMesh reads MeshMembership per namespace for enrollment + mTLS-verdict
// breakdown, MeshMetrics for the cluster-wide partial-enrollment count, and
// MeshIssues for HBONE-blocked counts (already parked on the cache by store).
// Only enrolled workloads contribute to the mTLS breakdown — an unenrolled
// workload has no verdict to report and would smear the "unset" bucket.
func (r *Recorder) recordMesh(cache *models.Cache) {
	enrolled := map[string]int{}
	notEnrolled := map[string]int{}
	mtlsByNsMode := map[[2]string]int{}
	for nodeID, membership := range cache.MeshMembership {
		workload, ok := cache.WorkloadByID[nodeID]
		if !ok || !isCountableWorkload(&workload) {
			continue
		}
		if membership.InMesh {
			enrolled[workload.Namespace]++
			mode := mtlsModeLabel(membership.Mtls)
			mtlsByNsMode[[2]string{workload.Namespace, mode}]++
		} else {
			notEnrolled[workload.Namespace]++
		}
	}
	for ns, count := range enrolled {
		r.meshWorkloads.WithLabelValues(ns, "true").Set(float64(count))
	}
	for ns, count := range notEnrolled {
		r.meshWorkloads.WithLabelValues(ns, "false").Set(float64(count))
	}
	for key, count := range mtlsByNsMode {
		r.meshMtls.WithLabelValues(key[0], key[1]).Set(float64(count))
	}
	r.meshNsPartial.Set(float64(cache.MeshMetrics.NsPartial))

	hbone := map[string]int{}
	for _, issue := range cache.MeshIssues {
		if issue.Type != models.MeshTransportBlocked || issue.Node == nil {
			continue
		}
		hbone[issue.Node.Namespace]++
	}
	for ns, count := range hbone {
		r.meshHboneBlocked.WithLabelValues(ns).Set(float64(count))
	}
}

// mtlsModeLabel maps a MeshMembership.Mtls verdict to the wire vocabulary
// documented for alatyr_mesh_mtls_workloads (strict|permissive|disabled|unset|
// unknown). Nil verdict → "unknown"; a PA fetch failure already lands as
// MeshUnknown at build time, so this branch only fires if a future code path
// leaves Mtls unset on an enrolled workload.
func mtlsModeLabel(mtls *models.MtlsState) string {
	if mtls == nil {
		return "unknown"
	}
	switch mtls.Verdict {
	case models.MeshStrict:
		return "strict"
	case models.MeshPermissive:
		return "permissive"
	case models.MeshDisable:
		return "disabled"
	case models.MeshUnset:
		return "unset"
	}
	return "unknown"
}
