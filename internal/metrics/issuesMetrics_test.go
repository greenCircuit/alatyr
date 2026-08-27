package metrics

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus/testutil"

	"graph/internal/models"
)

// Fixture endpoint shapes. Synthetic peers (CIDR, external) carry an empty
// Namespace by construction — that is the shape that used to make an issue
// resolve to namespace "" and get dropped before it ever reached a gauge.
var (
	checkoutWorkload = models.WorkloadNode{
		ID: "uid-checkout", Label: "checkout", Namespace: "shop", Type: models.NodeTypeService,
	}
	paymentsWorkload = models.WorkloadNode{
		ID: "uid-payments", Label: "payments", Namespace: "billing", Type: models.NodeTypeService,
	}
	internetCidr = models.WorkloadNode{
		ID: models.CIDRIDPrefix + "0.0.0.0/0", Label: "0.0.0.0/0", Type: models.NodeTypeCIDR, CidrType: models.CIDRWan,
	}
	externalPeer = models.WorkloadNode{
		ID: "external-ingress", Label: "external", Type: models.NodeTypeExternal,
	}
)

// issueFixture covers every endpoint shape recordIssues has to survive. Kept in
// one place because the counts asserted below are derived from it — the UI has
// a mirror of these shapes in ui/src/store/clusterStats.test.ts, where the
// displayed count is legitimately lower (mergeIssuesByPair folds the two
// same-pair engine rows into one, and the info-tier row is hidden).
func issueFixture() []models.Issue {
	return []models.Issue{
		// 1. workload -> workload, both namespaced. Attributes to dst (billing).
		{
			Type: models.PolicyConflicts, Engine: "k8spolicy",
			Src: &checkoutWorkload, Dst: &paymentsWorkload,
			IngressCulprits: []models.PolicyRef{{Source: "k8spolicy", Name: "deny-all", Namespace: "billing"}},
		},
		// 2. same pair, second engine. Metrics keep both — engine is the
		// actionable dimension; the UI merges them into one row.
		{
			Type: models.PolicyConflicts, Engine: "istio",
			Src: &checkoutWorkload, Dst: &paymentsWorkload,
			IngressCulprits: []models.PolicyRef{{Source: "istio", Name: "deny-all", Namespace: "billing"}},
		},
		// 3. workload -> CIDR. Dst has no namespace; must file under the src's.
		{
			Type: models.PolicyConflicts, Engine: "k8spolicy",
			Src: &checkoutWorkload, Dst: &internetCidr,
			EgressCulprits: []models.PolicyRef{{Source: "k8spolicy", Name: "egress-lockdown", Namespace: "shop"}},
		},
		// 4. CIDR -> workload. Same fall-through, other direction.
		{
			Type: models.PolicyConflicts, Engine: "k8spolicy",
			Src: &internetCidr, Dst: &paymentsWorkload,
		},
		// 5. node-scoped finding, no src/dst.
		{
			Type: models.NodeLockOut, Engine: "k8spolicy",
			Node:     &checkoutWorkload,
			Culprits: []models.PolicyRef{{Source: "k8spolicy", Name: "default-deny", Namespace: "shop"}},
		},
		// 6. info-tier finding. Metrics count it; the UI hides it by tier.
		{
			Type: models.IssuesPartial, Engine: "istio",
			Src: &checkoutWorkload, Dst: &paymentsWorkload,
		},
	}
}

// TestRecordIssues_NoIssueSilentlyDropped is the invariant that matters: every
// finding handed to the recorder lands in some alatyr_issues series. A finding
// that exists in the API but not in /metrics is worse than no metric — the
// dashboard reads clean while the UI shows the problem.
func TestRecordIssues_NoIssueSilentlyDropped(t *testing.T) {
	recorder := New(Config{}, nil)
	issues := issueFixture()
	recorder.recordIssues(issues)

	// Fixture has one IssuesPartial row, which now lives in alatyr_policy_layering
	// and must NOT appear in alatyr_issues — the split is the whole point of the
	// two families.
	expected := 0
	for _, issue := range issues {
		if issue.Type != models.IssuesPartial {
			expected++
		}
	}
	if total := sumGauge(t, recorder, "alatyr_issues"); total != float64(expected) {
		t.Fatalf("alatyr_issues total = %v, want %d (partial-access lives on alatyr_policy_layering)", total, expected)
	}

	// Per-namespace breakdown, derived from the fixture: billing gets the two
	// engine rows for the same pair plus the CIDR-src conflict; shop gets the
	// CIDR-dst conflict and the node-scoped lockout. The IssuesPartial row is
	// asserted in policyLayeringMetrics_test.go.
	cases := []struct {
		namespace string
		issueType string
		want      float64
	}{
		{"billing", string(models.PolicyConflicts), 3},
		{"shop", string(models.PolicyConflicts), 1},
		{"shop", string(models.NodeLockOut), 1},
	}
	for _, testCase := range cases {
		got := testutil.ToFloat64(recorder.issues.WithLabelValues(testCase.namespace, testCase.issueType))
		if got != testCase.want {
			t.Errorf("alatyr_issues{namespace=%q,type=%q} = %v, want %v",
				testCase.namespace, testCase.issueType, got, testCase.want)
		}
	}
}

// TestIssueLocation_SyntheticEndpointsFallThrough pins the attribution rule
// itself. Regression guard for the CIDR bug: an endpoint with no namespace
// must not win precedence, and an all-synthetic finding must get the sentinel
// rather than an empty label no Grafana variable can match.
func TestIssueLocation_SyntheticEndpointsFallThrough(t *testing.T) {
	cases := []struct {
		name          string
		issue         models.Issue
		wantNamespace string
		wantWorkload  string
	}{
		{
			name:          "dst wins when namespaced",
			issue:         models.Issue{Src: &checkoutWorkload, Dst: &paymentsWorkload},
			wantNamespace: "billing", wantWorkload: "payments",
		},
		{
			name:          "cidr dst falls through to src",
			issue:         models.Issue{Src: &checkoutWorkload, Dst: &internetCidr},
			wantNamespace: "shop", wantWorkload: "checkout",
		},
		{
			name:          "cidr src falls through to dst",
			issue:         models.Issue{Src: &internetCidr, Dst: &paymentsWorkload},
			wantNamespace: "billing", wantWorkload: "payments",
		},
		{
			name:          "node scoped wins over both endpoints",
			issue:         models.Issue{Node: &paymentsWorkload, Src: &checkoutWorkload, Dst: &internetCidr},
			wantNamespace: "billing", wantWorkload: "payments",
		},
		{
			name:          "all synthetic endpoints get the sentinel",
			issue:         models.Issue{Src: &externalPeer, Dst: &internetCidr},
			wantNamespace: CIDRScopedNamespace, wantWorkload: "0.0.0.0/0",
		},
		{
			name:          "no endpoints at all",
			issue:         models.Issue{Type: models.IssuesFailedToFetch},
			wantNamespace: "", wantWorkload: "",
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			namespace, workload := issueLocation(testCase.issue)
			if namespace != testCase.wantNamespace || workload != testCase.wantWorkload {
				t.Errorf("issueLocation() = (%q, %q), want (%q, %q)",
					namespace, workload, testCase.wantNamespace, testCase.wantWorkload)
			}
		})
	}
}

// TestRecordIssues_CIDREndpointReachesGauges is the end-to-end form of the bug
// report: conflicts against CIDR peers used to be absent from /metrics
// entirely while the UI listed them.
func TestRecordIssues_CIDREndpointReachesGauges(t *testing.T) {
	recorder := New(Config{}, nil)
	recorder.recordIssues([]models.Issue{
		{
			Type: models.PolicyConflicts, Engine: "k8spolicy",
			Src: &checkoutWorkload, Dst: &internetCidr,
			EgressCulprits: []models.PolicyRef{{Source: "k8spolicy", Name: "egress-lockdown", Namespace: "shop"}},
		},
		{
			Type: models.PolicyConflicts, Engine: "istio",
			Src: &externalPeer, Dst: &internetCidr,
		},
	})

	if got := testutil.ToFloat64(recorder.issues.WithLabelValues("shop", string(models.PolicyConflicts))); got != 1 {
		t.Errorf("CIDR-dst conflict under source namespace = %v, want 1", got)
	}
	if got := testutil.ToFloat64(recorder.issues.WithLabelValues(CIDRScopedNamespace, string(models.PolicyConflicts))); got != 1 {
		t.Errorf("all-synthetic conflict under %s = %v, want 1", CIDRScopedNamespace, got)
	}
	if got := testutil.ToFloat64(recorder.issuesByPolicy.WithLabelValues(
		"shop", string(models.PolicyConflicts), "k8spolicy", "shop", "egress-lockdown")); got != 1 {
		t.Errorf("CIDR-dst conflict missing from alatyr_issues_by_policy: got %v, want 1", got)
	}
}

// TestRecordIssuesByPolicy_FanOutSemantics documents why issues_by_policy can
// never be summed as a finding count: findings with no named culprit are
// absent, findings citing several policies appear once per policy, and a
// policy cited on both directions of one finding still counts once.
func TestRecordIssuesByPolicy_FanOutSemantics(t *testing.T) {
	recorder := New(Config{}, nil)
	bothDirections := models.PolicyRef{Source: "k8spolicy", Name: "default-deny", Namespace: "billing"}
	recorder.recordIssues([]models.Issue{
		// no culprits — counted in alatyr_issues, invisible in the breakdown
		{Type: models.PolicyConflicts, Engine: "istio", Src: &checkoutWorkload, Dst: &paymentsWorkload},
		// three distinct culprits — three series from one finding
		{
			Type: models.PolicyConflicts, Engine: "k8spolicy",
			Src: &paymentsWorkload, Dst: &checkoutWorkload,
			IngressCulprits: []models.PolicyRef{
				{Source: "k8spolicy", Name: "deny-all", Namespace: "shop"},
				{Source: "calico", Name: "global-egress"},
			},
			EgressCulprits: []models.PolicyRef{{Source: "k8spolicy", Name: "egress-lockdown", Namespace: "billing"}},
		},
		// same policy on both directions — deduped to one series
		{
			Type: models.NodeLockOut, Engine: "k8spolicy", Node: &paymentsWorkload,
			IngressCulprits: []models.PolicyRef{bothDirections},
			EgressCulprits:  []models.PolicyRef{bothDirections},
		},
	})

	if got := testutil.CollectAndCount(recorder.issuesByPolicy); got != 4 {
		t.Errorf("alatyr_issues_by_policy series = %d, want 4 (3 culprits + 1 deduped, zero-culprit finding absent)", got)
	}
	if got := testutil.ToFloat64(recorder.issuesByPolicy.WithLabelValues(
		"billing", string(models.NodeLockOut), "k8spolicy", "billing", "default-deny")); got != 1 {
		t.Errorf("policy cited on both directions counted %v times, want 1", got)
	}
	// Cluster-scoped culprit (Calico global policy) keeps its own sentinel.
	if got := testutil.ToFloat64(recorder.issuesByPolicy.WithLabelValues(
		"shop", string(models.PolicyConflicts), "calico", ClusterScopeNamespace, "global-egress")); got != 1 {
		t.Errorf("cluster-scoped culprit under %s = %v, want 1", ClusterScopeNamespace, got)
	}
	// Three findings, four pairs: one contributes nothing (no culprits), one
	// contributes three, one contributes a single deduped pair. Proof the two
	// families measure different things and must not be compared on a dashboard.
	if total := sumGauge(t, recorder, "alatyr_issues_by_policy"); total != 4 {
		t.Errorf("alatyr_issues_by_policy total = %v, want 4 finding x policy pairs", total)
	}
	if total := sumGauge(t, recorder, "alatyr_issues"); total != 3 {
		t.Errorf("alatyr_issues total = %v, want 3 findings", total)
	}
}

// TestRecordIssues_BreakdownDisabled guards the nil issuesByPolicy path: the
// per-culprit vec is unregistered when METRICS_ISSUE_POLICY_BREAKDOWN=false,
// and that must not change what alatyr_issues reports.
func TestRecordIssues_BreakdownDisabled(t *testing.T) {
	disabled := false
	recorder := New(Config{IssuePolicyBreakdown: &disabled}, nil)
	issues := issueFixture()
	recorder.recordIssues(issues)

	if recorder.issuesByPolicy != nil {
		t.Fatal("issuesByPolicy registered while breakdown is disabled")
	}
	expected := 0
	for _, issue := range issues {
		if issue.Type != models.IssuesPartial {
			expected++
		}
	}
	if total := sumGauge(t, recorder, "alatyr_issues"); total != float64(expected) {
		t.Errorf("alatyr_issues total = %v, want %d with breakdown disabled", total, expected)
	}
}

// TestRecordIssues_WorkloadsWithIssues_DedupBlastRadius pins the reason both
// blast-radius vecs exist: one workload hit by many findings must count once
// per namespace, and once per (namespace, type). If dedup regresses, the
// gauge starts inflating in lock-step with alatyr_issues and stops being a
// blast-radius signal.
func TestRecordIssues_WorkloadsWithIssues_DedupBlastRadius(t *testing.T) {
	recorder := New(Config{}, nil)
	// payments in billing carries 4 findings across 2 types; checkout in shop
	// carries 2 findings across 2 types. Distinct workloads per ns: 1 each.
	recorder.recordIssues([]models.Issue{
		{Type: models.PolicyConflicts, Engine: "k8spolicy", Src: &checkoutWorkload, Dst: &paymentsWorkload},
		{Type: models.PolicyConflicts, Engine: "istio", Src: &checkoutWorkload, Dst: &paymentsWorkload},
		{Type: models.IssuesPartial, Engine: "istio", Src: &checkoutWorkload, Dst: &paymentsWorkload},
		{Type: models.NodeLockOut, Engine: "k8spolicy", Node: &paymentsWorkload},
		{Type: models.NodeLockOut, Engine: "k8spolicy", Node: &checkoutWorkload},
		{Type: models.PolicyConflicts, Engine: "k8spolicy", Src: &checkoutWorkload, Dst: &internetCidr},
	})

	distinctCases := []struct {
		namespace string
		want      float64
	}{
		{"billing", 1},
		{"shop", 1},
	}
	for _, testCase := range distinctCases {
		got := testutil.ToFloat64(recorder.workloadsWithIssues.WithLabelValues(testCase.namespace))
		if got != testCase.want {
			t.Errorf("alatyr_workloads_with_issues{namespace=%q} = %v, want %v",
				testCase.namespace, got, testCase.want)
		}
	}

	// IssuesPartial rows now land on the policy_layering family, so the
	// by-type breakdown on alatyr_issues drops that entry.
	byTypeCases := []struct {
		namespace string
		issueType string
		want      float64
	}{
		{"billing", string(models.PolicyConflicts), 1},
		{"billing", string(models.NodeLockOut), 1},
		{"shop", string(models.PolicyConflicts), 1},
		{"shop", string(models.NodeLockOut), 1},
	}
	for _, testCase := range byTypeCases {
		got := testutil.ToFloat64(recorder.workloadsWithIssuesByType.WithLabelValues(testCase.namespace, testCase.issueType))
		if got != testCase.want {
			t.Errorf("alatyr_workloads_with_issues_by_type{namespace=%q,type=%q} = %v, want %v",
				testCase.namespace, testCase.issueType, got, testCase.want)
		}
	}
}

// TestRecordIssues_WorkloadsWithIssues_MultiWorkloadPerNs covers the case
// alatyr_issues cannot distinguish: N findings in one namespace could all be
// on one workload (blast radius = 1) or spread across N workloads (blast
// radius = N). Blast-radius vec must reflect the actual distinct count.
func TestRecordIssues_WorkloadsWithIssues_MultiWorkloadPerNs(t *testing.T) {
	authWorkload := models.WorkloadNode{
		ID: "uid-auth", Label: "auth", Namespace: "billing", Type: models.NodeTypeService,
	}
	ordersWorkload := models.WorkloadNode{
		ID: "uid-orders", Label: "orders", Namespace: "billing", Type: models.NodeTypeService,
	}
	recorder := New(Config{}, nil)
	recorder.recordIssues([]models.Issue{
		{Type: models.PolicyConflicts, Engine: "k8spolicy", Src: &checkoutWorkload, Dst: &paymentsWorkload},
		{Type: models.PolicyConflicts, Engine: "k8spolicy", Src: &checkoutWorkload, Dst: &authWorkload},
		{Type: models.NodeLockOut, Engine: "k8spolicy", Node: &ordersWorkload},
	})

	if got := testutil.ToFloat64(recorder.workloadsWithIssues.WithLabelValues("billing")); got != 3 {
		t.Errorf("alatyr_workloads_with_issues{namespace=\"billing\"} = %v, want 3 (payments, auth, orders)", got)
	}
	if got := testutil.ToFloat64(recorder.workloadsWithIssuesByType.WithLabelValues("billing", string(models.PolicyConflicts))); got != 2 {
		t.Errorf("alatyr_workloads_with_issues_by_type PolicyConflicts = %v, want 2 (payments, auth)", got)
	}
	if got := testutil.ToFloat64(recorder.workloadsWithIssuesByType.WithLabelValues("billing", string(models.NodeLockOut))); got != 1 {
		t.Errorf("alatyr_workloads_with_issues_by_type NodeLockOut = %v, want 1 (orders)", got)
	}
}

// TestRecordIssues_WorkloadsWithIssues_NamespaceIsolation guards that two
// workloads sharing the same label in different namespaces don't collapse
// into one set entry. Dedup key must include the namespace, otherwise a
// cross-namespace naming collision (e.g. "api" in every ns) silently
// undercounts.
func TestRecordIssues_WorkloadsWithIssues_NamespaceIsolation(t *testing.T) {
	apiShop := models.WorkloadNode{
		ID: "uid-api-shop", Label: "api", Namespace: "shop", Type: models.NodeTypeService,
	}
	apiBilling := models.WorkloadNode{
		ID: "uid-api-billing", Label: "api", Namespace: "billing", Type: models.NodeTypeService,
	}
	recorder := New(Config{}, nil)
	recorder.recordIssues([]models.Issue{
		{Type: models.NodeLockOut, Engine: "k8spolicy", Node: &apiShop},
		{Type: models.NodeLockOut, Engine: "k8spolicy", Node: &apiBilling},
	})

	if got := testutil.ToFloat64(recorder.workloadsWithIssues.WithLabelValues("shop")); got != 1 {
		t.Errorf("shop distinct = %v, want 1", got)
	}
	if got := testutil.ToFloat64(recorder.workloadsWithIssues.WithLabelValues("billing")); got != 1 {
		t.Errorf("billing distinct = %v, want 1", got)
	}
}

// TestRecordIssues_WorkloadsWithIssues_FromFixture pins the invariant against
// the shared fixture: alatyr_issues counts 6 findings, blast-radius reports
// 1 distinct workload per real namespace + the CIDR sentinel. If someone
// changes the fixture and this assertion drifts, the workloads_with_issues
// contract needs an explicit re-review — the two families must not blur.
func TestRecordIssues_WorkloadsWithIssues_FromFixture(t *testing.T) {
	recorder := New(Config{}, nil)
	recorder.recordIssues(issueFixture())

	cases := []struct {
		namespace string
		want      float64
	}{
		{"billing", 1}, // payments: non-layering findings collapse to 1
		{"shop", 1},    // checkout: non-layering findings collapse to 1
	}
	for _, testCase := range cases {
		got := testutil.ToFloat64(recorder.workloadsWithIssues.WithLabelValues(testCase.namespace))
		if got != testCase.want {
			t.Errorf("alatyr_workloads_with_issues{namespace=%q} = %v, want %v",
				testCase.namespace, got, testCase.want)
		}
	}
}

// TestRecordIssues_WorkloadsWithIssues_NotGatedByPolicyBreakdown proves the
// blast-radius vecs are always on. They live in the base issue-metrics tier,
// not behind Config.IssuePolicyBreakdown — turning off the culprit-policy
// breakdown must not accidentally take the blast-radius view down with it.
func TestRecordIssues_WorkloadsWithIssues_NotGatedByPolicyBreakdown(t *testing.T) {
	disabled := false
	recorder := New(Config{IssuePolicyBreakdown: &disabled}, nil)
	recorder.recordIssues(issueFixture())

	if recorder.issuesByPolicy != nil {
		t.Fatal("issuesByPolicy registered while breakdown is disabled")
	}
	if got := testutil.CollectAndCount(recorder.workloadsWithIssues); got == 0 {
		t.Error("alatyr_workloads_with_issues empty when policy breakdown disabled — vec must be always-on")
	}
	if got := testutil.CollectAndCount(recorder.workloadsWithIssuesByType); got == 0 {
		t.Error("alatyr_workloads_with_issues_by_type empty when policy breakdown disabled — vec must be always-on")
	}
}

// TestRecordIssues_WorkloadsWithIssues_EmptyWorkloadSkipped covers the guard
// on the workload label: a finding that resolves to (namespace, "") — no
// endpoint at all besides namespace attribution — must still populate
// alatyr_issues, but must not add a bogus "" entry to the distinct-workload
// set. sumGauge across workloads_with_issues stays bounded to real workloads.
func TestRecordIssues_WorkloadsWithIssues_EmptyWorkloadSkipped(t *testing.T) {
	// Construct an endpoint with a namespace but no label so issueLocation
	// returns ("billing", "").
	unlabeledInBilling := models.WorkloadNode{
		ID: "uid-unlabeled", Namespace: "billing", Type: models.NodeTypeService,
	}
	recorder := New(Config{}, nil)
	recorder.recordIssues([]models.Issue{
		{Type: models.NodeLockOut, Engine: "k8spolicy", Node: &unlabeledInBilling},
	})

	if got := testutil.ToFloat64(recorder.issues.WithLabelValues("billing", string(models.NodeLockOut))); got != 1 {
		t.Errorf("alatyr_issues still records the finding: got %v, want 1", got)
	}
	if got := testutil.CollectAndCount(recorder.workloadsWithIssues); got != 0 {
		t.Errorf("alatyr_workloads_with_issues series = %d, want 0 (empty workload must not create a series)", got)
	}
	if got := testutil.CollectAndCount(recorder.workloadsWithIssuesByType); got != 0 {
		t.Errorf("alatyr_workloads_with_issues_by_type series = %d, want 0", got)
	}
}

// TestRecordIssues_ResetClearsStaleSeries covers the Reset-then-repopulate
// contract: a finding that cleared between cycles must disappear, not linger
// at its last value. Reset lives in resetIssueVecs, one refactor away from
// being dropped silently.
func TestRecordIssues_ResetClearsStaleSeries(t *testing.T) {
	recorder := New(Config{}, nil)
	recorder.recordIssues(issueFixture())
	recorder.resetIssueVecs()
	recorder.recordIssues([]models.Issue{
		{Type: models.PolicyConflicts, Engine: "istio", Src: &checkoutWorkload, Dst: &paymentsWorkload},
	})

	if got := testutil.CollectAndCount(recorder.issues); got != 1 {
		t.Errorf("alatyr_issues series after reset = %d, want 1", got)
	}
	if total := sumGauge(t, recorder, "alatyr_issues"); total != 1 {
		t.Errorf("alatyr_issues total after reset = %v, want 1", total)
	}
	if got := testutil.CollectAndCount(recorder.workloadsWithIssues); got != 1 {
		t.Errorf("alatyr_workloads_with_issues series after reset = %d, want 1", got)
	}
	if got := testutil.CollectAndCount(recorder.workloadsWithIssuesByType); got != 1 {
		t.Errorf("alatyr_workloads_with_issues_by_type series after reset = %d, want 1", got)
	}
}

// sumGauge adds up every series of one metric family off the recorder's own
// registry. Gathering by name keeps the assertion scoped — the registry holds
// 20+ unrelated families.
func sumGauge(t *testing.T, recorder *Recorder, name string) float64 {
	t.Helper()
	families, err := recorder.registry.Gather()
	if err != nil {
		t.Fatalf("gather: %v", err)
	}
	total := 0.0
	for _, family := range families {
		if family.GetName() != name {
			continue
		}
		for _, metric := range family.GetMetric() {
			total += metric.GetGauge().GetValue()
		}
	}
	return total
}
