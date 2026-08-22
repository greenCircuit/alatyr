package metrics

import "graph/internal/models"

// Issue-derived metrics live here: alatyr_issues, alatyr_issues_by_engine,
// alatyr_issues_by_policy, and the opt-in per-workload alatyr_workload_issues.
//
// The three families answer different questions and must not be compared:
//   - issues            — one count per finding, per (namespace, type)
//   - issues_by_engine  — same, split by the engine that produced the finding
//   - issues_by_policy  — finding x culprit-policy pairs. Findings with no
//     named culprit never appear; findings citing several policies appear
//     several times. sum() is NOT a finding count.

// resetIssueVecs clears every issue gauge before repopulation. Called from
// resetSnapshotVecs so a finding that cleared this cycle disappears instead of
// sticking at its last value forever.
func (r *Recorder) resetIssueVecs() {
	r.issues.Reset()
	r.issuesByEngine.Reset()
	if r.issuesByPolicy != nil {
		r.issuesByPolicy.Reset()
	}
	if r.workloadIssues != nil {
		r.workloadIssues.Reset()
	}
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
//
// Precedence skips endpoints with no namespace instead of stopping on them.
// CIDR and external nodes are synthetic peers with an empty Namespace, so a
// workload -> 0.0.0.0/0 conflict used to resolve to "" and get dropped by the
// caller — the finding vanished from /metrics while the UI still showed it,
// and Grafana's namespace=~"$namespace" filter would have hidden the empty
// label anyway. Falling through keeps the finding filed under the real
// workload's namespace, which is the one an operator filters on.
//
// CIDRScopedNamespace is the last resort for a finding whose every endpoint is
// synthetic: still no real namespace to file under, but a visible sentinel
// beats an empty label that no dashboard variable can match.
func issueLocation(issue models.Issue) (namespace, workload string) {
	endpoints := []*models.WorkloadNode{issue.Node, issue.Dst, issue.Src}
	for _, node := range endpoints {
		if node != nil && node.Namespace != "" {
			return node.Namespace, node.Label
		}
	}
	for _, node := range endpoints {
		if node != nil {
			return CIDRScopedNamespace, node.Label
		}
	}
	return "", ""
}
