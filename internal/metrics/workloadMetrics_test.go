package metrics

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus/testutil"

	"graph/internal/models"
)

// coverageCache builds a two-namespace cluster where a single cluster-scoped
// Calico policy selects every workload, and only one workload additionally has
// a namespace-local NetworkPolicy. That is the live shape audited on the
// cluster: alatyr_workloads_unpoliced reads 0 everywhere because the global
// catch-all counts as coverage, hiding every workload with no policy of its own.
func coverageCache() (*models.Cache, []string) {
	shopWorkload := models.WorkloadNode{
		ID: "uid-checkout", Label: "checkout", Namespace: "shop", Type: models.NodeTypeDeployment,
	}
	billingWorkload := models.WorkloadNode{
		ID: "uid-payments", Label: "payments", Namespace: "billing", Type: models.NodeTypeDeployment,
	}
	// Synthetic nodes must not reach any denominator.
	billingNsNode := models.WorkloadNode{
		ID: "ns-billing", Label: "billing", Namespace: "billing", Type: models.NodeTypeNamespace,
	}
	billingCidr := models.WorkloadNode{
		ID: models.CIDRIDPrefix + "10.0.0.0/8", Label: "10.0.0.0/8", Type: models.NodeTypeCIDR,
	}

	globalRef := models.PolicyRef{Source: "calico", Name: "default-deny-global"} // no Namespace = cluster-scoped
	localRef := models.PolicyRef{Source: "k8s", Name: "checkout-ingress", Namespace: "shop"}

	cache := &models.Cache{
		NsIndex: map[string]models.NSIndex{
			"shop":    {Workloads: []models.WorkloadNode{shopWorkload}},
			"billing": {Workloads: []models.WorkloadNode{billingWorkload, billingNsNode, billingCidr}},
		},
		EvaluationResults: map[string]models.EvaluationResult{
			"calico": {NodePolicies: map[string][]models.PolicyRef{
				"uid-checkout": {globalRef},
				"uid-payments": {globalRef},
			}},
			"k8s": {NodePolicies: map[string][]models.PolicyRef{
				"uid-checkout": {localRef},
			}},
		},
	}
	return cache, []string{"calico", "k8s"}
}

// TestRecordWorkloads_UnpolicedExcludingGlobal pins the reason the second gauge
// exists: a cluster-wide catch-all pins the literal gauge at zero, so a
// workload with no policy of its own is invisible on the dashboard.
func TestRecordWorkloads_UnpolicedExcludingGlobal(t *testing.T) {
	recorder := New(Config{}, nil)
	cache, engines := coverageCache()
	recorder.recordWorkloads(cache, engines)

	cases := []struct {
		namespace                string
		unpoliced, exclGlobal    float64
		workloads                float64
	}{
		// checkout carries a namespace-local policy — covered under both readings.
		{namespace: "shop", unpoliced: 0, exclGlobal: 0, workloads: 1},
		// payments is selected only by the global. Literal gauge says covered;
		// the excluding-global gauge is the one that tells the truth.
		{namespace: "billing", unpoliced: 0, exclGlobal: 1, workloads: 1},
	}
	for _, testCase := range cases {
		if got := testutil.ToFloat64(recorder.workloads.WithLabelValues(testCase.namespace)); got != testCase.workloads {
			t.Errorf("alatyr_workloads{namespace=%q} = %v, want %v (synthetic nodes excluded)",
				testCase.namespace, got, testCase.workloads)
		}
		if got := testutil.ToFloat64(recorder.workloadsUnpoliced.WithLabelValues(testCase.namespace)); got != testCase.unpoliced {
			t.Errorf("alatyr_workloads_unpoliced{namespace=%q} = %v, want %v",
				testCase.namespace, got, testCase.unpoliced)
		}
		got := testutil.ToFloat64(recorder.workloadsUnpolicedExclGlobal.WithLabelValues(testCase.namespace))
		if got != testCase.exclGlobal {
			t.Errorf("alatyr_workloads_unpoliced_excluding_global{namespace=%q} = %v, want %v",
				testCase.namespace, got, testCase.exclGlobal)
		}
	}
}

// TestRecordWorkloads_UnpolicedGaugesOrdering guards the documented invariant
// between the two gauges: excluding-global can only ever be >= the literal one,
// per namespace. A future filter change that breaks this makes the pair
// nonsensical rather than merely wrong.
func TestRecordWorkloads_UnpolicedGaugesOrdering(t *testing.T) {
	recorder := New(Config{}, nil)
	cache, engines := coverageCache()
	// A workload no engine selects at all — unpoliced under both readings.
	cache.NsIndex["shop"] = models.NSIndex{Workloads: append(
		cache.NsIndex["shop"].Workloads,
		models.WorkloadNode{ID: "uid-orphan", Label: "orphan", Namespace: "shop", Type: models.NodeTypeDeployment},
	)}
	recorder.recordWorkloads(cache, engines)

	for _, namespace := range []string{"shop", "billing"} {
		literal := testutil.ToFloat64(recorder.workloadsUnpoliced.WithLabelValues(namespace))
		exclGlobal := testutil.ToFloat64(recorder.workloadsUnpolicedExclGlobal.WithLabelValues(namespace))
		if exclGlobal < literal {
			t.Errorf("namespace %q: excluding_global=%v < unpoliced=%v, invariant broken",
				namespace, exclGlobal, literal)
		}
	}
	if got := testutil.ToFloat64(recorder.workloadsUnpoliced.WithLabelValues("shop")); got != 1 {
		t.Errorf("alatyr_workloads_unpoliced{namespace=\"shop\"} = %v, want 1 (the orphan)", got)
	}
	if got := testutil.ToFloat64(recorder.workloadsUnpolicedExclGlobal.WithLabelValues("shop")); got != 1 {
		t.Errorf("alatyr_workloads_unpoliced_excluding_global{namespace=\"shop\"} = %v, want 1", got)
	}
}

// TestRecordCoverage_GlobalPolicyStillCounts keeps the per-engine coverage
// gauges on the literal reading — "did this engine look at the workload" is the
// question alatyr_workloads_covered answers, and a global policy genuinely does.
func TestRecordCoverage_GlobalPolicyStillCounts(t *testing.T) {
	recorder := New(Config{}, nil)
	cache, engines := coverageCache()
	recorder.recordCoverage(cache, engines)

	if got := testutil.ToFloat64(recorder.workloadsCovered.WithLabelValues("billing", "calico")); got != 1 {
		t.Errorf("calico coverage in billing = %v, want 1 (global policy selects it)", got)
	}
	if got := testutil.ToFloat64(recorder.workloadsCovered.WithLabelValues("billing", "k8s")); got != 0 {
		t.Errorf("k8s coverage in billing = %v, want 0", got)
	}
	// payments is selected by exactly one engine, checkout by two.
	if got := testutil.ToFloat64(recorder.workloadsSingleEngineCover.WithLabelValues("billing")); got != 1 {
		t.Errorf("single-engine count in billing = %v, want 1", got)
	}
	if got := testutil.ToFloat64(recorder.workloadsSingleEngineCover.WithLabelValues("shop")); got != 0 {
		t.Errorf("single-engine count in shop = %v, want 0", got)
	}
}
