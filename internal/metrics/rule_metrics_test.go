package metrics

import (
	"log/slog"
	"testing"

	"github.com/prometheus/client_golang/prometheus/testutil"

	"graph/internal/models"
)

// newTestRecorder builds a Recorder with default config for tests.
func newTestRecorder(t *testing.T) *Recorder {
	t.Helper()
	return New(Config{}, slog.Default())
}

// makeRule builds a Rule with the fields recordRulMetrics + bumpPolicy read.
// Kept flat so test cases stay readable one-line-per-rule.
func makeRule(engine, ns, name, action, coverage, direction string) models.Rule {
	rule := models.Rule{
		Coverage:  models.Coverage(coverage),
		Direction: models.Direction(direction),
		Contributor: models.PolicyRef{
			Source:    engine,
			Name:      name,
			Namespace: ns,
			Action:    action,
		},
	}
	if action == "deny" {
		rule.Action = models.ActionDeny
	}
	return rule
}

// makeCacheAllow packs rules into a Cache with everything in AllowByNs, keyed
// by the rule's contributor namespace. recordRulMetrics only reads AllowByNs +
// DenyByNs, so this is enough for edge/coverage tests.
func makeCacheAllow(rulesByEngine map[string][]models.Rule) *models.Cache {
	results := map[string]models.EvaluationResult{}
	for engine, rules := range rulesByEngine {
		byNs := map[string][]models.Rule{}
		for _, rule := range rules {
			byNs[rule.Contributor.Namespace] = append(byNs[rule.Contributor.Namespace], rule)
		}
		results[engine] = models.EvaluationResult{AllowByNs: byNs}
	}
	return &models.Cache{EvaluationResults: results}
}

// makeCacheSplit routes rules into AllowByNs or DenyByNs based on rule.Action.
// Exercises the combined-loop pattern in recordRulMetrics that walks both maps.
func makeCacheSplit(rulesByEngine map[string][]models.Rule) *models.Cache {
	results := map[string]models.EvaluationResult{}
	for engine, rules := range rulesByEngine {
		allow := map[string][]models.Rule{}
		deny := map[string][]models.Rule{}
		for _, rule := range rules {
			ns := rule.Contributor.Namespace
			if rule.Action == models.ActionDeny {
				deny[ns] = append(deny[ns], rule)
			} else {
				allow[ns] = append(allow[ns], rule)
			}
		}
		results[engine] = models.EvaluationResult{AllowByNs: allow, DenyByNs: deny}
	}
	return &models.Cache{EvaluationResults: results}
}

// TestBumpPolicy_DedupSameManifestMixedActions locks in the reason alatyr_policies
// dropped the action label: Calico/Kyverno manifests routinely mix allow+deny rules,
// and keying the count by action would report one manifest as two.
func TestBumpPolicy_DedupSameManifestMixedActions(t *testing.T) {
	rec := newTestRecorder(t)
	cache := makeCacheSplit(map[string][]models.Rule{
		"calico": {
			makeRule("calico", "ns1", "manifest-a", "allow", "restricted", "ingress"),
			makeRule("calico", "ns1", "manifest-a", "deny", "restricted", "ingress"),
		},
	})

	rec.recordPolicies(cache)

	got := testutil.ToFloat64(rec.policies.WithLabelValues("ns1", "calico"))
	if got != 1 {
		t.Fatalf("policies{ns1, calico} = %v, want 1 (same manifest, mixed actions must dedup)", got)
	}
}

// TestBumpPolicy_DedupSameManifestManyRules confirms one manifest producing many
// rules (post-expansion fan-out) still counts as one manifest.
func TestBumpPolicy_DedupSameManifestManyRules(t *testing.T) {
	rec := newTestRecorder(t)
	rules := []models.Rule{}
	for i := 0; i < 50; i++ {
		rules = append(rules, makeRule("k8s", "ns1", "blanket", "allow", "allow all ns", "ingress"))
	}
	cache := makeCacheAllow(map[string][]models.Rule{"k8s": rules})

	rec.recordPolicies(cache)

	got := testutil.ToFloat64(rec.policies.WithLabelValues("ns1", "k8s"))
	if got != 1 {
		t.Fatalf("policies{ns1, k8s} = %v, want 1 (fan-out must not inflate manifest count)", got)
	}
}

// TestBumpPolicy_ClusterScopedFallback verifies cluster-scoped policies (empty
// Namespace on Contributor) roll up under ClusterScopeNamespace instead of
// rendering as a phantom empty-string label.
func TestBumpPolicy_ClusterScopedFallback(t *testing.T) {
	rec := newTestRecorder(t)
	cache := makeCacheAllow(map[string][]models.Rule{
		"calico": {makeRule("calico", "", "global-policy", "allow", "restricted", "ingress")},
	})

	rec.recordPolicies(cache)

	got := testutil.ToFloat64(rec.policies.WithLabelValues(ClusterScopeNamespace, "calico"))
	if got != 1 {
		t.Fatalf("policies{%s, calico} = %v, want 1", ClusterScopeNamespace, got)
	}
}

// TestBumpPolicy_DistinctManifests confirms different (engine, ns, name) tuples
// each get their own count — the dedup only collapses identical identities.
func TestBumpPolicy_DistinctManifests(t *testing.T) {
	rec := newTestRecorder(t)
	cache := makeCacheAllow(map[string][]models.Rule{
		"k8s": {
			makeRule("k8s", "ns1", "policy-a", "allow", "restricted", "ingress"),
			makeRule("k8s", "ns1", "policy-b", "allow", "restricted", "ingress"),
			makeRule("k8s", "ns2", "policy-a", "allow", "restricted", "ingress"),
		},
	})

	rec.recordPolicies(cache)

	if got := testutil.ToFloat64(rec.policies.WithLabelValues("ns1", "k8s")); got != 2 {
		t.Errorf("policies{ns1, k8s} = %v, want 2", got)
	}
	if got := testutil.ToFloat64(rec.policies.WithLabelValues("ns2", "k8s")); got != 1 {
		t.Errorf("policies{ns2, k8s} = %v, want 1", got)
	}
}

// TestRecordRulMetrics_RuleEdgesRawCount verifies alatyr_rule_edges counts every
// rule iteration without dedup — the invariant that lets sum(rule_edges) equal
// total graph edges.
func TestRecordRulMetrics_RuleEdgesRawCount(t *testing.T) {
	rec := newTestRecorder(t)
	rules := []models.Rule{}
	for i := 0; i < 5; i++ {
		rules = append(rules, makeRule("k8s", "ns1", "blanket", "allow", "allow all ns", "ingress"))
	}
	cache := makeCacheAllow(map[string][]models.Rule{"k8s": rules})

	rec.recordRulMetrics(cache)

	got := testutil.ToFloat64(rec.ruleEdges.WithLabelValues("ns1", "k8s", "allow", "allow all ns", "ingress"))
	if got != 5 {
		t.Fatalf("rule_edges = %v, want 5 (raw count must not dedup)", got)
	}
}

// TestRecordRulMetrics_PolicyCoverageDedupPerBucket confirms alatyr_policy_coverage
// collapses many rules from the same policy in the same bucket to one — the
// invariant that makes it a distinct-policy count.
func TestRecordRulMetrics_PolicyCoverageDedupPerBucket(t *testing.T) {
	rec := newTestRecorder(t)
	rules := []models.Rule{}
	for i := 0; i < 10; i++ {
		rules = append(rules, makeRule("k8s", "ns1", "blanket", "allow", "allow all ns", "ingress"))
	}
	cache := makeCacheAllow(map[string][]models.Rule{"k8s": rules})

	rec.recordRulMetrics(cache)

	got := testutil.ToFloat64(rec.policyCoverage.WithLabelValues("ns1", "k8s", "allow", "allow all ns", "ingress"))
	if got != 1 {
		t.Fatalf("policy_coverage = %v, want 1 (same policy in same bucket must dedup)", got)
	}
}

// TestRecordRulMetrics_PolicyCoverageMultipleBuckets locks in the "one count per
// bucket" contract: the same policy landing in ingress + egress counts once per
// direction. This is why sum(policy_coverage) is NOT a valid total.
func TestRecordRulMetrics_PolicyCoverageMultipleBuckets(t *testing.T) {
	rec := newTestRecorder(t)
	cache := makeCacheAllow(map[string][]models.Rule{
		"k8s": {
			makeRule("k8s", "ns1", "both-dirs", "allow", "restricted", "ingress"),
			makeRule("k8s", "ns1", "both-dirs", "allow", "restricted", "egress"),
		},
	})

	rec.recordRulMetrics(cache)

	if got := testutil.ToFloat64(rec.policyCoverage.WithLabelValues("ns1", "k8s", "allow", "restricted", "ingress")); got != 1 {
		t.Errorf("policy_coverage ingress = %v, want 1", got)
	}
	if got := testutil.ToFloat64(rec.policyCoverage.WithLabelValues("ns1", "k8s", "allow", "restricted", "egress")); got != 1 {
		t.Errorf("policy_coverage egress = %v, want 1", got)
	}
}

// TestRecordRulMetrics_EdgesUnknownFallback confirms rules with missing coverage or
// direction get an "unknown" label instead of an empty string — keeping the raw
// count whole (a dropped rule silently under-reports graph size).
func TestRecordRulMetrics_EdgesUnknownFallback(t *testing.T) {
	rec := newTestRecorder(t)
	cache := makeCacheAllow(map[string][]models.Rule{
		"k8s": {makeRule("k8s", "ns1", "opaque", "allow", "", "")},
	})

	rec.recordRulMetrics(cache)

	got := testutil.ToFloat64(rec.ruleEdges.WithLabelValues("ns1", "k8s", "allow", "unknown", "unknown"))
	if got != 1 {
		t.Fatalf("rule_edges{coverage=unknown, direction=unknown} = %v, want 1", got)
	}
}

// TestRecordRulMetrics_CoverageSkipsMissingCoverage confirms alatyr_policy_coverage
// skips rules with no coverage attribution (nothing to bucket) — but the edge
// count still records the rule.
func TestRecordRulMetrics_CoverageSkipsMissingCoverage(t *testing.T) {
	rec := newTestRecorder(t)
	cache := makeCacheAllow(map[string][]models.Rule{
		"k8s": {makeRule("k8s", "ns1", "opaque", "allow", "", "ingress")},
	})

	rec.recordRulMetrics(cache)

	if got := testutil.ToFloat64(rec.ruleEdges.WithLabelValues("ns1", "k8s", "allow", "unknown", "ingress")); got != 1 {
		t.Errorf("rule_edges = %v, want 1 (edge count still records the rule)", got)
	}
	// policy_coverage should have no series for this rule.
	if got := testutil.CollectAndCount(rec.policyCoverage); got != 0 {
		t.Errorf("policy_coverage series count = %v, want 0 (empty coverage must not attribute)", got)
	}
}

// TestRecordRulMetrics_ActionDerivedFromRuleAction covers the fallback path in
// ruleMetrics: when Contributor.Action is empty, rule.Action drives the label.
func TestRecordRulMetrics_ActionDerivedFromRuleAction(t *testing.T) {
	rec := newTestRecorder(t)
	rule := models.Rule{
		Action:    models.ActionDeny,
		Coverage:  models.CoverageRestricted,
		Direction: models.DirectionIngress,
		Contributor: models.PolicyRef{
			Source:    "istio",
			Name:      "deny-policy",
			Namespace: "ns1",
			// Action deliberately empty so fallback triggers.
		},
	}
	cache := makeCacheSplit(map[string][]models.Rule{"istio": {rule}})

	rec.recordRulMetrics(cache)

	got := testutil.ToFloat64(rec.ruleEdges.WithLabelValues("ns1", "istio", "deny", "restricted", "ingress"))
	if got != 1 {
		t.Fatalf("rule_edges{action=deny} = %v, want 1 (fallback from rule.Action must set label)", got)
	}
}

// TestResetSnapshotVecs_ClearsPhantomSeries proves the reset-then-repopulate
// contract: a namespace that disappears between snapshots must not leave phantom
// series behind. The whole point of resetSnapshotVecs is preventing this.
func TestResetSnapshotVecs_ClearsPhantomSeries(t *testing.T) {
	rec := newTestRecorder(t)

	first := makeCacheAllow(map[string][]models.Rule{
		"k8s": {makeRule("k8s", "gone-ns", "policy-a", "allow", "restricted", "ingress")},
	})
	rec.recordRulMetrics(first)
	if got := testutil.ToFloat64(rec.ruleEdges.WithLabelValues("gone-ns", "k8s", "allow", "restricted", "ingress")); got != 1 {
		t.Fatalf("first snapshot: rule_edges = %v, want 1", got)
	}

	rec.resetSnapshotVecs()
	second := makeCacheAllow(map[string][]models.Rule{
		"k8s": {makeRule("k8s", "stay-ns", "policy-b", "allow", "restricted", "ingress")},
	})
	rec.recordRulMetrics(second)

	// The old namespace's series must be gone; the new one must be present.
	if got := testutil.CollectAndCount(rec.ruleEdges); got != 1 {
		t.Errorf("after reset: rule_edges series = %v, want 1 (phantom gone-ns must not survive)", got)
	}
	if got := testutil.ToFloat64(rec.ruleEdges.WithLabelValues("stay-ns", "k8s", "allow", "restricted", "ingress")); got != 1 {
		t.Errorf("after reset: stay-ns edges = %v, want 1", got)
	}
}
