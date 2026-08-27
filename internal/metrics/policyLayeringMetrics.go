package metrics

import "graph/internal/models"

// Policy-layering metrics live here: alatyr_policy_layering,
// alatyr_policy_layering_by_engine, alatyr_policy_layering_by_policy,
// alatyr_workloads_with_policy_layering,
// alatyr_workloads_with_policy_layering_by_type.
//
// The layering family exists because IssuesPartial fires on an expected
// wide+narrow policy stack (coarse rule blocks, fine rule allows verified
// paths). Counting it under alatyr_issues inflates finding counts with
// non-findings. Split family = one metric per truth.
//
// Shape mirrors alatyr_issues by design so dashboard panels can be
// duplicated with a single metric-name swap.

// resetPolicyLayeringVecs clears every layering gauge before repopulation.
// Same reset-then-repopulate contract as the issue family: a layering event
// that cleared this cycle must disappear instead of sticking forever.
func (r *Recorder) resetPolicyLayeringVecs() {
	r.policyLayering.Reset()
	r.policyLayeringByEngine.Reset()
	r.workloadsWithLayering.Reset()
	r.workloadsWithLayeringByType.Reset()
	if r.policyLayeringByPolicy != nil {
		r.policyLayeringByPolicy.Reset()
	}
}

// recordPolicyLayering fans layering findings (IssuesPartial today) into the
// layering gauge family. Ignores every other issue type — recordIssues owns
// those. Same location + culprit fan-out semantics as recordIssues so the
// families are safe to compare 1:1 on a dashboard.
func (r *Recorder) recordPolicyLayering(issues []models.Issue) {
	byNsType := map[[2]string]int{}
	byNsTypeEngine := map[[3]string]int{}
	byNsTypeEnginePolicy := map[policyIssueKey]int{}

	distinctByNs := map[string]map[string]struct{}{}
	distinctByNsType := map[[2]string]map[string]struct{}{}

	for _, issue := range issues {
		if issue.Type != models.IssuesPartial {
			continue
		}
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

		if workload != "" {
			if distinctByNs[namespace] == nil {
				distinctByNs[namespace] = map[string]struct{}{}
			}
			distinctByNs[namespace][workload] = struct{}{}
			if distinctByNsType[key] == nil {
				distinctByNsType[key] = map[string]struct{}{}
			}
			distinctByNsType[key][workload] = struct{}{}
		}

		if r.policyLayeringByPolicy != nil {
			bumpIssuePolicies(byNsTypeEnginePolicy, namespace, issue)
		}
	}

	for key, count := range byNsType {
		r.policyLayering.WithLabelValues(key[0], key[1]).Set(float64(count))
	}
	for key, count := range byNsTypeEngine {
		r.policyLayeringByEngine.WithLabelValues(key[0], key[1], key[2]).Set(float64(count))
	}
	for namespace, set := range distinctByNs {
		r.workloadsWithLayering.WithLabelValues(namespace).Set(float64(len(set)))
	}
	for key, set := range distinctByNsType {
		r.workloadsWithLayeringByType.WithLabelValues(key[0], key[1]).Set(float64(len(set)))
	}
	if r.policyLayeringByPolicy != nil {
		for key, count := range byNsTypeEnginePolicy {
			r.policyLayeringByPolicy.WithLabelValues(
				key.namespace,
				key.issueType,
				key.engine,
				key.policyNamespace,
				key.policyName,
			).Set(float64(count))
		}
	}
}
