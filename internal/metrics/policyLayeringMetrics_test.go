package metrics

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus/testutil"

	"graph/internal/models"
)

// TestRecordPolicyLayering_NoLayeringSilentlyDropped mirrors the equivalent
// invariant on alatyr_issues: every IssuesPartial handed to the recorder must
// land in some alatyr_policy_layering series. A partial finding that lives in
// the API but is absent from /metrics is the same silent-lie the split was
// created to prevent.
func TestRecordPolicyLayering_NoLayeringSilentlyDropped(t *testing.T) {
	recorder := New(Config{}, nil)
	issues := issueFixture()
	recorder.recordPolicyLayering(issues)

	expected := 0
	for _, issue := range issues {
		if issue.Type == models.IssuesPartial {
			expected++
		}
	}
	if expected == 0 {
		t.Fatal("fixture has no IssuesPartial row — layering test would pass vacuously")
	}
	if total := sumGauge(t, recorder, "alatyr_policy_layering"); total != float64(expected) {
		t.Fatalf("alatyr_policy_layering total = %v, want %d", total, expected)
	}

	// Fixture has one IssuesPartial: checkoutWorkload → paymentsWorkload,
	// istio engine. Location resolves to (billing, payments) via dst-wins.
	if got := testutil.ToFloat64(recorder.policyLayering.WithLabelValues("billing", string(models.IssuesPartial))); got != 1 {
		t.Errorf("alatyr_policy_layering{billing,partial access} = %v, want 1", got)
	}
	if got := testutil.ToFloat64(recorder.policyLayeringByEngine.WithLabelValues("billing", string(models.IssuesPartial), "istio")); got != 1 {
		t.Errorf("alatyr_policy_layering_by_engine{billing,partial access,istio} = %v, want 1", got)
	}
}

// TestRecordPolicyLayering_IgnoresNonPartial guards the filter: any non-partial
// issue handed to recordPolicyLayering must produce zero series. Regression
// guard against a future refactor that accidentally routes everything here.
func TestRecordPolicyLayering_IgnoresNonPartial(t *testing.T) {
	recorder := New(Config{}, nil)
	recorder.recordPolicyLayering([]models.Issue{
		{Type: models.PolicyConflicts, Engine: "istio", Src: &checkoutWorkload, Dst: &paymentsWorkload},
		{Type: models.NodeLockOut, Engine: "k8spolicy", Node: &checkoutWorkload},
		{Type: models.IssuesCidrScope, Engine: "calico", Src: &checkoutWorkload, Dst: &internetCidr},
	})

	if got := testutil.CollectAndCount(recorder.policyLayering); got != 0 {
		t.Errorf("alatyr_policy_layering series with no IssuesPartial input = %d, want 0", got)
	}
	if got := testutil.CollectAndCount(recorder.workloadsWithLayering); got != 0 {
		t.Errorf("alatyr_workloads_with_policy_layering series with no IssuesPartial input = %d, want 0", got)
	}
}

// TestRecordPolicyLayering_BlastRadius covers the distinct-workload dedup for
// the layering family. Same semantics as workloadsWithIssues: one workload hit
// by N layering events counts once per namespace, once per (namespace, type).
func TestRecordPolicyLayering_BlastRadius(t *testing.T) {
	recorder := New(Config{}, nil)
	recorder.recordPolicyLayering([]models.Issue{
		{Type: models.IssuesPartial, Engine: "k8spolicy", Src: &checkoutWorkload, Dst: &paymentsWorkload},
		{Type: models.IssuesPartial, Engine: "istio", Src: &checkoutWorkload, Dst: &paymentsWorkload},
		{Type: models.IssuesPartial, Engine: "k8spolicy", Src: &checkoutWorkload, Dst: &internetCidr},
	})

	// billing: payments hit twice (same workload, dedup to 1).
	if got := testutil.ToFloat64(recorder.workloadsWithLayering.WithLabelValues("billing")); got != 1 {
		t.Errorf("alatyr_workloads_with_policy_layering{billing} = %v, want 1", got)
	}
	// shop: checkout hit once (CIDR-dst falls through to src).
	if got := testutil.ToFloat64(recorder.workloadsWithLayering.WithLabelValues("shop")); got != 1 {
		t.Errorf("alatyr_workloads_with_policy_layering{shop} = %v, want 1", got)
	}
	if got := testutil.ToFloat64(recorder.workloadsWithLayeringByType.WithLabelValues("billing", string(models.IssuesPartial))); got != 1 {
		t.Errorf("alatyr_workloads_with_policy_layering_by_type{billing,partial access} = %v, want 1", got)
	}
}

// TestRecordPolicyLayering_ByPolicyFanOut proves bumpIssuePolicies reuse works
// on the layering family: culprits fan out, findings with no named culprit
// stay absent, same-policy on both directions dedupes.
func TestRecordPolicyLayering_ByPolicyFanOut(t *testing.T) {
	recorder := New(Config{}, nil)
	bothDirections := models.PolicyRef{Source: "k8spolicy", Name: "wide-ns-allow", Namespace: "billing"}
	recorder.recordPolicyLayering([]models.Issue{
		// no culprits — counted in alatyr_policy_layering, absent from by_policy
		{Type: models.IssuesPartial, Engine: "istio", Src: &checkoutWorkload, Dst: &paymentsWorkload},
		// same policy on both directions — deduped to one series
		{
			Type: models.IssuesPartial, Engine: "k8spolicy",
			Src: &checkoutWorkload, Dst: &paymentsWorkload,
			IngressCulprits: []models.PolicyRef{bothDirections},
			EgressCulprits:  []models.PolicyRef{bothDirections},
		},
	})

	if got := testutil.CollectAndCount(recorder.policyLayeringByPolicy); got != 1 {
		t.Errorf("alatyr_policy_layering_by_policy series = %d, want 1 (one deduped, one absent)", got)
	}
	if got := testutil.ToFloat64(recorder.policyLayeringByPolicy.WithLabelValues(
		"billing", string(models.IssuesPartial), "k8spolicy", "billing", "wide-ns-allow")); got != 1 {
		t.Errorf("policy cited on both directions counted %v times, want 1", got)
	}
}

// TestRecordPolicyLayering_BreakdownDisabled guards the nil policyLayeringByPolicy
// path: same Config.IssuePolicyBreakdown gate as issues_by_policy, so disabling
// one disables both.
func TestRecordPolicyLayering_BreakdownDisabled(t *testing.T) {
	disabled := false
	recorder := New(Config{IssuePolicyBreakdown: &disabled}, nil)
	recorder.recordPolicyLayering(issueFixture())

	if recorder.policyLayeringByPolicy != nil {
		t.Fatal("policyLayeringByPolicy registered while breakdown is disabled")
	}
	if got := sumGauge(t, recorder, "alatyr_policy_layering"); got == 0 {
		t.Error("alatyr_policy_layering empty when breakdown disabled — base family must be always-on")
	}
}

// TestPartialAccess_LivesOnLayeringNotIssues is the split-contract guard.
// Runs both recorders on the shared fixture and asserts that:
//   - no alatyr_issues* series carries type="partial access"
//   - alatyr_policy_layering* carries exactly one such series
//
// Weak-total assertions elsewhere can pass if the filter breaks AND some other
// bug cancels the inflation. This test reads the actual label values off the
// registry — no cancellation path lets it pass wrongly.
func TestPartialAccess_LivesOnLayeringNotIssues(t *testing.T) {
	recorder := New(Config{}, nil)
	issues := issueFixture()
	recorder.recordIssues(issues)
	recorder.recordPolicyLayering(issues)

	families, err := recorder.registry.Gather()
	if err != nil {
		t.Fatalf("gather: %v", err)
	}

	partialLabel := string(models.IssuesPartial)
	issuesFamilyPrefix := "alatyr_issues"
	layeringFamilyPrefix := "alatyr_policy_layering"
	layeringWorkloadsPrefix := "alatyr_workloads_with_policy_layering"

	issueSeriesWithPartial := 0
	layeringSeriesWithPartial := 0

	for _, family := range families {
		name := family.GetName()
		isIssueFamily := len(name) >= len(issuesFamilyPrefix) && name[:len(issuesFamilyPrefix)] == issuesFamilyPrefix
		isLayeringFamily := (len(name) >= len(layeringFamilyPrefix) && name[:len(layeringFamilyPrefix)] == layeringFamilyPrefix) ||
			(len(name) >= len(layeringWorkloadsPrefix) && name[:len(layeringWorkloadsPrefix)] == layeringWorkloadsPrefix)

		// alatyr_issues_by_engine also starts with alatyr_issues — that's fine,
		// prefix match still catches it. But alatyr_policy_layering doesn't
		// start with alatyr_issues, so no cross-hit.
		if !isIssueFamily && !isLayeringFamily {
			continue
		}
		for _, metric := range family.GetMetric() {
			hasPartial := false
			for _, labelPair := range metric.GetLabel() {
				if labelPair.GetName() == "type" && labelPair.GetValue() == partialLabel {
					hasPartial = true
					break
				}
			}
			if !hasPartial {
				continue
			}
			if isIssueFamily {
				issueSeriesWithPartial++
				t.Errorf("alatyr_issues family leaked partial-access series: %s %v", name, metric.GetLabel())
			}
			if isLayeringFamily {
				layeringSeriesWithPartial++
			}
		}
	}

	if issueSeriesWithPartial != 0 {
		t.Errorf("alatyr_issues* carried %d partial-access series, want 0", issueSeriesWithPartial)
	}
	if layeringSeriesWithPartial == 0 {
		t.Error("alatyr_policy_layering* carried 0 partial-access series, want >= 1")
	}
}

// TestRecordPolicyLayering_ResetClearsStaleSeries covers the reset contract on
// the layering family: a layering event that cleared between cycles must not
// linger. Symmetric to the issue-family reset test.
func TestRecordPolicyLayering_ResetClearsStaleSeries(t *testing.T) {
	recorder := New(Config{}, nil)
	recorder.recordPolicyLayering(issueFixture())
	recorder.resetPolicyLayeringVecs()
	recorder.recordPolicyLayering([]models.Issue{
		{Type: models.IssuesPartial, Engine: "istio", Src: &checkoutWorkload, Dst: &paymentsWorkload},
	})

	if got := testutil.CollectAndCount(recorder.policyLayering); got != 1 {
		t.Errorf("alatyr_policy_layering series after reset = %d, want 1", got)
	}
	if got := testutil.CollectAndCount(recorder.workloadsWithLayering); got != 1 {
		t.Errorf("alatyr_workloads_with_policy_layering series after reset = %d, want 1", got)
	}
}
