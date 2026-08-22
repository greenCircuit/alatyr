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

	if total := sumGauge(t, recorder, "alatyr_issues"); total != float64(len(issues)) {
		t.Fatalf("alatyr_issues total = %v, want %d (one series entry per finding)", total, len(issues))
	}

	// Per-namespace breakdown, derived from the fixture: billing gets the two
	// engine rows for the same pair, the CIDR-src conflict, and the info-tier
	// row; shop gets the CIDR-dst conflict and the node-scoped lockout.
	cases := []struct {
		namespace string
		issueType string
		want      float64
	}{
		{"billing", string(models.PolicyConflicts), 3},
		{"billing", string(models.IssuesPartial), 1},
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
	if total := sumGauge(t, recorder, "alatyr_issues"); total != float64(len(issues)) {
		t.Errorf("alatyr_issues total = %v, want %d with breakdown disabled", total, len(issues))
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
