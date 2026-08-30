package metrics

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"

	"graph/internal/models"
)

// snapshotFixtureCache assembles a cache that exercises every recordX branch
// RecordSnapshot invokes. If a sub-recorder is dropped from the orchestration
// glue, one of the family-presence assertions below flips to zero.
func snapshotFixtureCache() (*models.Cache, []string) {
	shopWorkload := models.WorkloadNode{
		ID: "uid-checkout", Label: "checkout", Namespace: "shop", Type: models.NodeTypeDeployment,
		Statuses: []models.StatusKey{models.StatusInternetIngress},
	}
	billingWorkload := models.WorkloadNode{
		ID: "uid-payments", Label: "payments", Namespace: "billing", Type: models.NodeTypeDeployment,
		Statuses: []models.StatusKey{models.StatusLanEgress},
	}
	rule := models.Rule{
		Coverage:  models.CoverageRestricted,
		Direction: models.DirectionIngress,
		Contributor: models.PolicyRef{
			Source: "k8s", Name: "allow-shop", Namespace: "shop", Action: "allow",
		},
	}
	cache := &models.Cache{
		NsIndex: map[string]models.NSIndex{
			"shop":    {Workloads: []models.WorkloadNode{shopWorkload}},
			"billing": {Workloads: []models.WorkloadNode{billingWorkload}},
		},
		WorkloadByID: map[string]models.WorkloadNode{
			"uid-checkout": shopWorkload,
			"uid-payments": billingWorkload,
		},
		EvaluationResults: map[string]models.EvaluationResult{
			"k8s": {
				AllowByNs:    map[string][]models.Rule{"shop": {rule}},
				NodePolicies: map[string][]models.PolicyRef{"uid-checkout": {rule.Contributor}},
			},
		},
		MeshMembership: map[string]models.MeshMembership{
			"uid-checkout": {InMesh: true, Mtls: &models.MtlsState{Verdict: models.MeshStrict}},
			"uid-payments": {InMesh: false},
		},
		MeshMetrics: models.MeshMetrics{NsPartial: 1},
		MeshIssues: []models.Issue{
			{Type: models.MeshTransportBlocked, Node: &shopWorkload},
		},
	}
	issues := []models.Issue{
		{Type: models.PolicyConflicts, Engine: "k8s", Src: &shopWorkload, Dst: &billingWorkload,
			IngressCulprits: []models.PolicyRef{{Source: "k8s", Name: "deny-all", Namespace: "billing"}}},
		{Type: models.IssuesPartial, Engine: "k8s", Src: &shopWorkload, Dst: &billingWorkload},
	}
	_ = issues
	return cache, []string{"k8s"}
}

// TestRecordSnapshot_InvokesEverySubRecorder is the glue-integrity guard. If a
// future refactor drops one of the recordX calls from RecordSnapshot, one
// family below stops producing series and the test flips red — instead of a
// silent scrape that names the metric but never populates it.
func TestRecordSnapshot_InvokesEverySubRecorder(t *testing.T) {
	recorder := newTestRecorder(t)
	cache, engines := snapshotFixtureCache()
	issues := []models.Issue{
		{Type: models.PolicyConflicts, Engine: "k8s",
			Src: &models.WorkloadNode{ID: "uid-checkout", Label: "checkout", Namespace: "shop"},
			Dst: &models.WorkloadNode{ID: "uid-payments", Label: "payments", Namespace: "billing"}},
		{Type: models.IssuesPartial, Engine: "k8s",
			Src: &models.WorkloadNode{ID: "uid-checkout", Label: "checkout", Namespace: "shop"},
			Dst: &models.WorkloadNode{ID: "uid-payments", Label: "payments", Namespace: "billing"}},
	}

	recorder.RecordSnapshot(cache, issues, engines)

	families := []struct {
		name string
		vec  prometheus.Collector
	}{
		{"workloads", recorder.workloads},
		{"workloads_by_status", recorder.workloadsByStatus},
		{"workloads_covered", recorder.workloadsCovered},
		{"policies", recorder.policies},
		{"issues", recorder.issues},
		{"policy_layering", recorder.policyLayering},
		{"mesh_workloads", recorder.meshWorkloads},
		{"mesh_mtls_workloads", recorder.meshMtls},
		{"mesh_hbone_blocked_workloads", recorder.meshHboneBlocked},
		{"rule_edges", recorder.ruleEdges},
		{"policy_coverage", recorder.policyCoverage},
	}
	for _, family := range families {
		if got := testutil.CollectAndCount(family.vec); got == 0 {
			t.Errorf("alatyr_%s: RecordSnapshot produced zero series — sub-recorder dropped from orchestration", family.name)
		}
	}
	// evaluation_timestamp_seconds is the freshness signal; RecordSnapshot must
	// stamp it every call, otherwise age(timestamp) alerts fire on a running
	// process.
	if got := testutil.ToFloat64(recorder.evalTimestamp); got == 0 {
		t.Errorf("alatyr_evaluation_timestamp_seconds not stamped by RecordSnapshot")
	}
}

// TestRecordSnapshot_NilCacheIsSafe pins the documented guard on RecordSnapshot:
// a nil cache pointer must be a no-op, not a panic. Prod path: snapshot loop
// fires before the first cache is ever built.
func TestRecordSnapshot_NilCacheIsSafe(t *testing.T) {
	recorder := newTestRecorder(t)
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("RecordSnapshot(nil) panicked: %v", r)
		}
	}()
	recorder.RecordSnapshot(nil, nil, []string{"k8s"})
	if got := testutil.CollectAndCount(recorder.workloads); got != 0 {
		t.Errorf("nil cache should not populate any series, got %d", got)
	}
	if got := testutil.ToFloat64(recorder.evalTimestamp); got != 0 {
		t.Errorf("nil cache stamped evalTimestamp = %v, want 0 (no snapshot happened)", got)
	}
}

// TestRecordMesh_EnrollmentBreakdown covers recordMesh: enrolled/not-enrolled
// per namespace, mTLS verdict fan-out, HBONE-blocked attribution. Every branch
// was 0% in coverage before this test.
func TestRecordMesh_EnrollmentBreakdown(t *testing.T) {
	shopEnrolled := models.WorkloadNode{ID: "uid-a", Label: "a", Namespace: "shop", Type: models.NodeTypeDeployment}
	shopNotEnrolled := models.WorkloadNode{ID: "uid-b", Label: "b", Namespace: "shop", Type: models.NodeTypeDeployment}
	shopNs := models.WorkloadNode{ID: "ns-shop", Namespace: "shop", Type: models.NodeTypeNamespace}
	billingPermissive := models.WorkloadNode{ID: "uid-c", Label: "c", Namespace: "billing", Type: models.NodeTypeDeployment}

	cache := &models.Cache{
		WorkloadByID: map[string]models.WorkloadNode{
			"uid-a":   shopEnrolled,
			"uid-b":   shopNotEnrolled,
			"ns-shop": shopNs,
			"uid-c":   billingPermissive,
		},
		MeshMembership: map[string]models.MeshMembership{
			"uid-a":   {InMesh: true, Mtls: &models.MtlsState{Verdict: models.MeshStrict}},
			"uid-b":   {InMesh: false},
			"ns-shop": {InMesh: true, Mtls: &models.MtlsState{Verdict: models.MeshStrict}}, // synthetic ns node must be filtered out
			"uid-c":   {InMesh: true, Mtls: &models.MtlsState{Verdict: models.MeshPermissive}},
		},
		MeshMetrics: models.MeshMetrics{NsPartial: 2},
		MeshIssues: []models.Issue{
			{Type: models.MeshTransportBlocked, Node: &shopEnrolled},
			{Type: models.MeshTransportBlocked, Node: &shopEnrolled},          // dedup by ns key: two findings, one workload — count of findings is 2
			{Type: models.PolicyConflicts, Node: &shopEnrolled},                // wrong type, must not count
		},
	}
	recorder := newTestRecorder(t)
	recorder.recordMesh(cache)

	if got := testutil.ToFloat64(recorder.meshWorkloads.WithLabelValues("shop", "true")); got != 1 {
		t.Errorf("mesh_workloads{shop, enrolled=true} = %v, want 1 (synthetic ns node must be filtered)", got)
	}
	if got := testutil.ToFloat64(recorder.meshWorkloads.WithLabelValues("shop", "false")); got != 1 {
		t.Errorf("mesh_workloads{shop, enrolled=false} = %v, want 1", got)
	}
	if got := testutil.ToFloat64(recorder.meshMtls.WithLabelValues("shop", "strict")); got != 1 {
		t.Errorf("mesh_mtls_workloads{shop, strict} = %v, want 1 (synthetic ns must not contribute)", got)
	}
	if got := testutil.ToFloat64(recorder.meshMtls.WithLabelValues("billing", "permissive")); got != 1 {
		t.Errorf("mesh_mtls_workloads{billing, permissive} = %v, want 1", got)
	}
	if got := testutil.ToFloat64(recorder.meshNsPartial); got != 2 {
		t.Errorf("mesh_namespaces_partially_enrolled = %v, want 2", got)
	}
	if got := testutil.ToFloat64(recorder.meshHboneBlocked.WithLabelValues("shop")); got != 2 {
		t.Errorf("mesh_hbone_blocked_workloads{shop} = %v, want 2 (only MeshTransportBlocked counts)", got)
	}
}

// TestRecordMesh_MtlsUnenrolledNotSmeared guards the "only enrolled workloads
// contribute a verdict" contract. An unenrolled workload has no mTLS verdict —
// counting it as "unset" would smear that bucket and hide real unset-verdict
// findings from operators.
func TestRecordMesh_MtlsUnenrolledNotSmeared(t *testing.T) {
	workload := models.WorkloadNode{ID: "uid-a", Label: "a", Namespace: "shop", Type: models.NodeTypeDeployment}
	cache := &models.Cache{
		WorkloadByID:   map[string]models.WorkloadNode{"uid-a": workload},
		MeshMembership: map[string]models.MeshMembership{"uid-a": {InMesh: false}},
	}
	recorder := newTestRecorder(t)
	recorder.recordMesh(cache)

	if got := testutil.CollectAndCount(recorder.meshMtls); got != 0 {
		t.Errorf("mesh_mtls_workloads series = %d, want 0 (unenrolled must not populate a verdict)", got)
	}
}

// TestMtlsModeLabel_AllVerdicts pins the wire vocabulary. Downstream Grafana
// panels legend-key on these exact strings — a typo silently drops the panel.
func TestMtlsModeLabel_AllVerdicts(t *testing.T) {
	cases := []struct {
		mtls *models.MtlsState
		want string
	}{
		{nil, "unknown"},
		{&models.MtlsState{Verdict: models.MeshStrict}, "strict"},
		{&models.MtlsState{Verdict: models.MeshPermissive}, "permissive"},
		{&models.MtlsState{Verdict: models.MeshDisable}, "disabled"},
		{&models.MtlsState{Verdict: models.MeshUnset}, "unset"},
		{&models.MtlsState{Verdict: models.MeshUnknown}, "unknown"},
		{&models.MtlsState{Verdict: models.MeshScope("bogus")}, "unknown"},
	}
	for _, testCase := range cases {
		if got := mtlsModeLabel(testCase.mtls); got != testCase.want {
			t.Errorf("mtlsModeLabel(%+v) = %q, want %q", testCase.mtls, got, testCase.want)
		}
	}
}
