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
	r.workloadsInternetReach.Reset()
	r.workloadsCovered.Reset()
	r.workloadsSingleEngineCover.Reset()
	r.policies.Reset()
	r.issues.Reset()
	r.issuesByEngine.Reset()
	if r.issuesByPolicy != nil {
		r.issuesByPolicy.Reset()
	}
	r.meshWorkloads.Reset()
	r.meshMtls.Reset()
	r.meshHboneBlocked.Reset()
	if r.workloadStatus != nil {
		r.workloadStatus.Reset()
	}
	if r.workloadIssues != nil {
		r.workloadIssues.Reset()
	}
	r.ruleEdges.Reset()
	r.policyCoverage.Reset()
}

// recordWorkloads walks every namespace once and stamps the exposure +
// status gauges. Only real workloads count — synthetic namespace nodes
// (Type NodeTypeNamespace) and CIDR nodes are excluded so the denominator
// stays "workloads a policy could actually select".
func (r *Recorder) recordWorkloads(cache *models.Cache, enabledEngines []string) {
	for ns, nsIndex := range cache.NsIndex {
		total := 0
		byStatus := map[models.StatusKey]int{}
		unpoliced := 0
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
			if !isCoveredByAnyEngine(cache, workload.ID, enabledEngines) {
				unpoliced++
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
				if isCoveredByEngine(cache, workload.ID, engine) {
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

// recordIssues groups issues by (namespace, type) using the affected
// workload's namespace when available. Edge-scoped issues (policy
// conflicts) attribute to the destination — the ns whose posture is
// visibly wrong from the outside.
func (r *Recorder) recordIssues(issues []models.Issue) {
	byNsType := map[[2]string]int{}
	byNsTypeEngine := map[[3]string]int{}
	byNsWorkload := map[[3]string]int{}
	byNsTypeEnginePolicy := map[policyIssueKey]int{}

	for _, issue := range issues {
		namespace, workload := issueLocation(issue)
		if namespace == "" {
			continue
		}
		key := [2]string{namespace, string(issue.Type)}
		byNsType[key]++

		if issue.Engine != "" {
			engineKey := [3]string{namespace, string(issue.Type), issue.Engine}
			byNsTypeEngine[engineKey]++
		}

		if r.workloadIssues != nil && workload != "" {
			byNsWorkload[[3]string{namespace, workload, string(issue.Type)}]++
		}

		if r.issuesByPolicy != nil {
			bumpIssuePolicies(byNsTypeEnginePolicy, namespace, issue)
		}
	}

	for key, count := range byNsType {
		r.issues.WithLabelValues(key[0], key[1]).Set(float64(count))
	}
	for key, count := range byNsTypeEngine {
		r.issuesByEngine.WithLabelValues(key[0], key[1], key[2]).Set(float64(count))
	}
	if r.workloadIssues != nil {
		for key, count := range byNsWorkload {
			r.workloadIssues.WithLabelValues(key[0], key[1], key[2]).Set(float64(count))
		}
	}
	if r.issuesByPolicy != nil {
		for key, count := range byNsTypeEnginePolicy {
			r.issuesByPolicy.WithLabelValues(
				key.namespace,
				key.issueType,
				key.engine,
				key.policyNamespace,
				key.policyName,
			).Set(float64(count))
		}
	}
}

// policyIssueKey is the (namespace, type, engine, policy_ns, policy_name)
// tuple emitted by alatyr_issues_by_policy.
type policyIssueKey struct {
	namespace       string
	issueType       string
	engine          string
	policyNamespace string
	policyName      string
}

// bumpIssuePolicies fans one issue out across every culprit policy it names
// and increments counts per unique (issue, policy) pair. Dedup runs per-issue
// so a policy cited on both ingress and egress of the same issue counts once.
//
// Engine label prefers the culprit's Source (correct for cross-engine issues
// like policy conflicts where issue.Engine is blank). Falls back to
// issue.Engine when the culprit doesn't declare its source.
func bumpIssuePolicies(
	counts map[policyIssueKey]int,
	namespace string,
	issue models.Issue,
) {
	culprits := make([]models.PolicyRef, 0)
	culprits = append(culprits, issue.IngressCulprits...)
	culprits = append(culprits, issue.EgressCulprits...)
	culprits = append(culprits, issue.Culprits...)
	if len(culprits) == 0 {
		return
	}

	seen := map[policyIssueKey]struct{}{}
	for _, ref := range culprits {
		if ref.Name == "" {
			continue
		}
		engine := ref.Source
		if engine == "" {
			engine = issue.Engine
		}
		policyNamespace := ref.Namespace
		if policyNamespace == "" {
			policyNamespace = ClusterScopeNamespace
		}
		key := policyIssueKey{
			namespace:       namespace,
			issueType:       string(issue.Type),
			engine:          engine,
			policyNamespace: policyNamespace,
			policyName:      ref.Name,
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		counts[key]++
	}
}

// issueLocation returns the (namespace, workload label) an issue attaches to.
// Node > Dst > Src precedence: node-scoped findings own the workload, edge
// findings point at the receiver whose posture reads as wrong externally.
func issueLocation(issue models.Issue) (namespace, workload string) {
	switch {
	case issue.Node != nil:
		return issue.Node.Namespace, issue.Node.Label
	case issue.Dst != nil:
		return issue.Dst.Namespace, issue.Dst.Label
	case issue.Src != nil:
		return issue.Src.Namespace, issue.Src.Label
	}
	return "", ""
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
// selecting policy for the workload. Iterates only enabled engines so a
// disabled engine's stale NodePolicies entries don't mask an unpoliced
// workload.
func isCoveredByAnyEngine(cache *models.Cache, workloadID string, engines []string) bool {
	for _, engine := range engines {
		if isCoveredByEngine(cache, workloadID, engine) {
			return true
		}
	}
	return false
}

func isCoveredByEngine(cache *models.Cache, workloadID string, engine string) bool {
	eval, ok := cache.EvaluationResults[engine]
	if !ok {
		return false
	}
	return len(eval.NodePolicies[workloadID]) > 0
}


