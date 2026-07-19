package istio

import (
	"errors"
	"log/slog"
	"testing"
	"time"

	"graph/internal/k8s"
	"graph/internal/models"

	istioapi "istio.io/api/security/v1beta1"
	istiosec "istio.io/client-go/pkg/apis/security/v1"
)

// fakePAClient satisfies k8s.KubernetesClient via the embedded interface —
// BuildMeshMembership only calls GetPeerAuthentications. paByNs seeds the
// return per-ns; errByNs seeds a fetch error per-ns (empty = success).
type fakePAClient struct {
	k8s.KubernetesClient
	paByNs   map[string][]*istiosec.PeerAuthentication
	errByNs  map[string]error
	callsByNs map[string]int
}

func (f *fakePAClient) GetPeerAuthentications(ns string) ([]*istiosec.PeerAuthentication, error) {
	if f.callsByNs == nil {
		f.callsByNs = map[string]int{}
	}
	f.callsByNs[ns]++
	if err, ok := f.errByNs[ns]; ok {
		return nil, err
	}
	return f.paByNs[ns], nil
}

// buildSource returns a source wired with the given fake client.
func buildSource(client k8s.KubernetesClient) *source {
	return &source{client: client, log: slog.Default()}
}

// ambientNs builds an NSIndex with the ambient enrollment label on the ns node
// and the given workloads.
func ambientNs(nsName string, workloads ...models.WorkloadNode) models.NSIndex {
	return models.NSIndex{
		NSNode: &models.WorkloadNode{
			ID:        nsName,
			Namespace: nsName,
			Type:      models.NodeTypeNamespace,
			Labels:    map[string]string{AmbientEnrollmentKey: AmbientEnrollmentValue},
		},
		Workloads: workloads,
	}
}

// plainNs builds an NSIndex with no enrollment label — non-mesh workloads.
func plainNs(nsName string, workloads ...models.WorkloadNode) models.NSIndex {
	return models.NSIndex{
		NSNode: &models.WorkloadNode{
			ID:        nsName,
			Namespace: nsName,
			Type:      models.NodeTypeNamespace,
			Labels:    map[string]string{},
		},
		Workloads: workloads,
	}
}

// Non-enrolled ns: every workload gets InMesh=false, NO ns PA fetch happens
// (only root PA fetched). Guards the fast-path skip.
func TestBuildMeshMembership_NonEnrolledNsSkipsPaFetch(t *testing.T) {
	client := &fakePAClient{
		paByNs: map[string][]*istiosec.PeerAuthentication{RootNamespace: nil},
	}
	idx := map[string]models.NSIndex{
		"ns-plain": plainNs("ns-plain",
			models.WorkloadNode{ID: "wl1", Namespace: "ns-plain"},
			models.WorkloadNode{ID: "wl2", Namespace: "ns-plain"},
		),
	}

	result, err := buildSource(client).BuildMeshMembership(idx)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}

	if client.callsByNs["ns-plain"] != 0 {
		t.Errorf("non-enrolled ns PA fetched %d times; want 0", client.callsByNs["ns-plain"])
	}
	if client.callsByNs[RootNamespace] != 1 {
		t.Errorf("root PA fetches: want 1, got %d", client.callsByNs[RootNamespace])
	}
	for _, id := range []string{"wl1", "wl2"} {
		m, ok := result.Memberships[id]
		if !ok {
			t.Errorf("%s missing from memberships", id)
			continue
		}
		if m.InMesh {
			t.Errorf("%s: InMesh=true, want false", id)
		}
	}
	if result.Metrics.WorkloadsTotal != 2 {
		t.Errorf("WorkloadsTotal: want 2, got %d", result.Metrics.WorkloadsTotal)
	}
	if result.Metrics.WorkloadsEnrolled != 0 {
		t.Errorf("WorkloadsEnrolled: want 0, got %d", result.Metrics.WorkloadsEnrolled)
	}
	if result.Metrics.NsEnrolled != 0 || result.Metrics.NsTotal != 1 {
		t.Errorf("Ns metrics: want NsTotal=1 NsEnrolled=0, got total=%d enrolled=%d",
			result.Metrics.NsTotal, result.Metrics.NsEnrolled)
	}
}

// Root PA fetch error degrades: no error returned, mesh build continues,
// membership still computed from ns/workload PAs, failure surfaces as a
// MeshMisconfig issue. Prevents "one flaky RBAC row tears down /api/graph".
func TestBuildMeshMembership_RootPaErrorDegrades(t *testing.T) {
	sentinel := errors.New("root boom")
	client := &fakePAClient{
		paByNs:  map[string][]*istiosec.PeerAuthentication{"ns-a": nil},
		errByNs: map[string]error{RootNamespace: sentinel},
	}
	idx := map[string]models.NSIndex{
		"ns-a": ambientNs("ns-a", models.WorkloadNode{ID: "wl1", Namespace: "ns-a"}),
	}

	result, err := buildSource(client).BuildMeshMembership(idx)
	if err != nil {
		t.Fatalf("root PA error should not return err, got %v", err)
	}
	// Ns walk still ran — ns-a still fetched despite root failure.
	if client.callsByNs["ns-a"] != 1 {
		t.Errorf("ns-a PA should still be fetched after root failure, got %d calls", client.callsByNs["ns-a"])
	}
	m, ok := result.Memberships["wl1"]
	if !ok {
		t.Fatalf("wl1 missing from memberships despite degrade")
	}
	if !m.InMesh {
		t.Errorf("wl1 should still be InMesh from ns label, got %+v", m)
	}
	// One MeshMisconfig issue must attribute the fetch error to the source.
	var found bool
	for _, issue := range result.Issues {
		if issue.Type == models.MeshMisconfig && issue.Engine == SourceName &&
			containsSubstring(issue.Message, "root boom") {
			found = true
		}
	}
	if !found {
		t.Errorf("root PA failure should surface as MeshMisconfig issue with error text, got %+v", result.Issues)
	}
}

// Per-ns PA fetch error degrades: no error returned, other namespaces still
// resolve, workloads in the failed ns are still marked InMesh (from ns label)
// but with Mtls=nil (unknown). Failure surfaces as a MeshMisconfig issue.
func TestBuildMeshMembership_NsPaErrorDegrades(t *testing.T) {
	sentinel := errors.New("ns boom")
	strictPA := makePA("ns-strict", "ns-good", istioapi.PeerAuthentication_MutualTLS_STRICT, nil, time.Unix(100, 0))
	client := &fakePAClient{
		paByNs:  map[string][]*istiosec.PeerAuthentication{RootNamespace: nil, "ns-good": {strictPA}},
		errByNs: map[string]error{"ns-bad": sentinel},
	}
	idx := map[string]models.NSIndex{
		"ns-bad":  ambientNs("ns-bad", models.WorkloadNode{ID: "wl-bad", Namespace: "ns-bad"}),
		"ns-good": ambientNs("ns-good", models.WorkloadNode{ID: "wl-good", Namespace: "ns-good"}),
	}

	result, err := buildSource(client).BuildMeshMembership(idx)
	if err != nil {
		t.Fatalf("ns PA error should not return err, got %v", err)
	}
	// The healthy ns must still resolve fully — one bad ns cannot poison the pass.
	good := result.Memberships["wl-good"]
	if !good.InMesh || good.Mtls == nil || good.Mtls.Verdict != models.MeshStrict {
		t.Errorf("healthy ns must resolve normally, got %+v (mtls=%+v)", good, good.Mtls)
	}
	// The failing ns still marks workloads InMesh (from ns label) but Mtls unknown.
	bad, ok := result.Memberships["wl-bad"]
	if !ok {
		t.Fatalf("wl-bad missing from memberships despite degrade")
	}
	if !bad.InMesh {
		t.Errorf("wl-bad should still be InMesh from ns label, got %+v", bad)
	}
	if bad.Mtls != nil {
		t.Errorf("wl-bad Mtls should be nil (unknown), got %+v", bad.Mtls)
	}
	var found bool
	for _, issue := range result.Issues {
		if issue.Type == models.MeshMisconfig && issue.Engine == SourceName &&
			containsSubstring(issue.Message, "ns boom") && containsSubstring(issue.Message, "ns-bad") {
			found = true
		}
	}
	if !found {
		t.Errorf("ns PA failure should surface as MeshMisconfig issue naming the ns, got %+v", result.Issues)
	}
}

// containsSubstring keeps the test self-contained without pulling strings in.
func containsSubstring(haystack, needle string) bool {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
}

// Enrolled ns: every workload stamped with provider + mode ambient + resolved
// mTLS. Mtls strict from a ns-scoped STRICT PA.
func TestBuildMeshMembership_EnrolledNsStrict(t *testing.T) {
	strictPA := makePA("ns-strict", "ns-a", istioapi.PeerAuthentication_MutualTLS_STRICT, nil, time.Unix(100, 0))
	client := &fakePAClient{
		paByNs: map[string][]*istiosec.PeerAuthentication{
			RootNamespace: nil,
			"ns-a":        {strictPA},
		},
	}
	idx := map[string]models.NSIndex{
		"ns-a": ambientNs("ns-a",
			models.WorkloadNode{ID: "wl1", Namespace: "ns-a", Labels: map[string]string{"app": "a"}},
			models.WorkloadNode{ID: "wl2", Namespace: "ns-a", Labels: map[string]string{"app": "b"}},
		),
	}

	result, err := buildSource(client).BuildMeshMembership(idx)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	for _, id := range []string{"wl1", "wl2"} {
		m := result.Memberships[id]
		if !m.InMesh || m.Provider != SourceName || m.Mode != "ambient" {
			t.Errorf("%s: want ambient in-mesh, got %+v", id, m)
		}
		if m.Mtls == nil || m.Mtls.Verdict != models.MeshStrict {
			t.Errorf("%s: want strict Mtls, got %+v", id, m.Mtls)
		}
	}
	if result.Metrics.MtlsStrict != 2 {
		t.Errorf("MtlsStrict: want 2, got %d", result.Metrics.MtlsStrict)
	}
	if result.Metrics.WorkloadsEnrolled != 2 || result.Metrics.NsEnrolled != 1 {
		t.Errorf("want 2 enrolled + 1 ns enrolled, got wl=%d ns=%d",
			result.Metrics.WorkloadsEnrolled, result.Metrics.NsEnrolled)
	}
}

// Workload label overrides ns label. Ambient ns + opted-out workload =
// InMesh=false for that workload, plus NsPartial bumped.
func TestBuildMeshMembership_WorkloadLabelOverridesNs(t *testing.T) {
	client := &fakePAClient{
		paByNs: map[string][]*istiosec.PeerAuthentication{
			RootNamespace: nil,
			"ns-a":        nil,
		},
	}
	idx := map[string]models.NSIndex{
		"ns-a": ambientNs("ns-a",
			models.WorkloadNode{ID: "wl-in", Namespace: "ns-a"},
			models.WorkloadNode{ID: "wl-out", Namespace: "ns-a",
				Labels: map[string]string{AmbientEnrollmentKey: AmbientSkipValue}},
		),
	}

	result, err := buildSource(client).BuildMeshMembership(idx)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !result.Memberships["wl-in"].InMesh {
		t.Errorf("wl-in should be in mesh")
	}
	if result.Memberships["wl-out"].InMesh {
		t.Errorf("wl-out label opt-out ignored: %+v", result.Memberships["wl-out"])
	}
	if result.Metrics.NsPartial != 1 {
		t.Errorf("NsPartial: want 1 (mixed enrollment), got %d", result.Metrics.NsPartial)
	}
	if result.Metrics.WorkloadsEnrolled != 1 {
		t.Errorf("WorkloadsEnrolled: want 1, got %d", result.Metrics.WorkloadsEnrolled)
	}
	// Total counts opted-out workloads too — they still consume a slot.
	if result.Metrics.WorkloadsTotal != 2 {
		t.Errorf("WorkloadsTotal: want 2, got %d", result.Metrics.WorkloadsTotal)
	}
}

// Ns-node (Type=NodeTypeNamespace) sitting in the Workloads slice must NOT be
// counted as a workload nor stamped with a membership entry. Guards the
// "namespace-endpoint false-positive" footgun.
func TestBuildMeshMembership_NsNodeSkipped(t *testing.T) {
	client := &fakePAClient{
		paByNs: map[string][]*istiosec.PeerAuthentication{RootNamespace: nil, "ns-a": nil},
	}
	idx := map[string]models.NSIndex{
		"ns-a": {
			NSNode: &models.WorkloadNode{ID: "ns/ns-a", Namespace: "ns-a", Type: models.NodeTypeNamespace,
				Labels: map[string]string{AmbientEnrollmentKey: AmbientEnrollmentValue}},
			Workloads: []models.WorkloadNode{
				{ID: "wl1", Namespace: "ns-a"},
				{ID: "ns/ns-a", Namespace: "ns-a", Type: models.NodeTypeNamespace,
					Labels: map[string]string{AmbientEnrollmentKey: AmbientEnrollmentValue}},
			},
		},
	}

	result, err := buildSource(client).BuildMeshMembership(idx)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if _, ok := result.Memberships["ns/ns-a"]; ok {
		t.Errorf("ns node leaked into memberships map")
	}
	if result.Metrics.WorkloadsTotal != 1 {
		t.Errorf("WorkloadsTotal: want 1 (ns node skipped), got %d", result.Metrics.WorkloadsTotal)
	}
}

// PA hygiene warnings (e.g. duplicate at scope) surface as MeshMisconfig Issues
// so /api/issues can render them. One workload = many Issues rows possible.
func TestBuildMeshMembership_MtlsIssuesSurfaceAsMeshIssues(t *testing.T) {
	// Two ns-scoped PAs at the same scope → duplicate warning.
	pa1 := makePA("dup-a", "ns-a", istioapi.PeerAuthentication_MutualTLS_STRICT, nil, time.Unix(100, 0))
	pa2 := makePA("dup-b", "ns-a", istioapi.PeerAuthentication_MutualTLS_STRICT, nil, time.Unix(200, 0))
	client := &fakePAClient{
		paByNs: map[string][]*istiosec.PeerAuthentication{
			RootNamespace: nil,
			"ns-a":        {pa1, pa2},
		},
	}
	idx := map[string]models.NSIndex{
		"ns-a": ambientNs("ns-a", models.WorkloadNode{ID: "wl1", Namespace: "ns-a"}),
	}

	result, err := buildSource(client).BuildMeshMembership(idx)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if len(result.Issues) == 0 {
		t.Fatalf("expected MeshMisconfig issues for duplicate PAs, got 0")
	}
	for _, issue := range result.Issues {
		if issue.Type != models.MeshMisconfig {
			t.Errorf("issue type: want MeshMisconfig, got %q (%s)", issue.Type, issue.Message)
		}
		if issue.Engine != SourceName {
			t.Errorf("issue engine: want %q, got %q", SourceName, issue.Engine)
		}
		if issue.Node == nil || issue.Node.ID != "wl1" {
			t.Errorf("issue Node: want wl1, got %+v", issue.Node)
		}
	}
}

// Mixed cluster: enrolled + non-enrolled ns + mixed mTLS verdicts. Metrics
// aggregate across the whole pass without cross-ns contamination.
func TestBuildMeshMembership_MetricsAggregateAcrossNs(t *testing.T) {
	strictPA := makePA("ns-strict", "ns-strict", istioapi.PeerAuthentication_MutualTLS_STRICT, nil, time.Unix(100, 0))
	permissivePA := makePA("ns-perm", "ns-perm", istioapi.PeerAuthentication_MutualTLS_PERMISSIVE, nil, time.Unix(100, 0))
	client := &fakePAClient{
		paByNs: map[string][]*istiosec.PeerAuthentication{
			RootNamespace: nil,
			"ns-strict":   {strictPA},
			"ns-perm":     {permissivePA},
		},
	}
	idx := map[string]models.NSIndex{
		"ns-strict": ambientNs("ns-strict", models.WorkloadNode{ID: "s1", Namespace: "ns-strict"}),
		"ns-perm":   ambientNs("ns-perm", models.WorkloadNode{ID: "p1", Namespace: "ns-perm"}),
		"ns-plain":  plainNs("ns-plain", models.WorkloadNode{ID: "x1", Namespace: "ns-plain"}),
	}

	result, err := buildSource(client).BuildMeshMembership(idx)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if result.Metrics.NsTotal != 3 {
		t.Errorf("NsTotal: want 3, got %d", result.Metrics.NsTotal)
	}
	if result.Metrics.NsEnrolled != 2 {
		t.Errorf("NsEnrolled: want 2, got %d", result.Metrics.NsEnrolled)
	}
	if result.Metrics.WorkloadsTotal != 3 {
		t.Errorf("WorkloadsTotal: want 3, got %d", result.Metrics.WorkloadsTotal)
	}
	if result.Metrics.WorkloadsEnrolled != 2 {
		t.Errorf("WorkloadsEnrolled: want 2, got %d", result.Metrics.WorkloadsEnrolled)
	}
	if result.Metrics.MtlsStrict != 1 {
		t.Errorf("MtlsStrict: want 1, got %d", result.Metrics.MtlsStrict)
	}
	if result.Metrics.MtlsPermissive != 1 {
		t.Errorf("MtlsPermissive: want 1, got %d", result.Metrics.MtlsPermissive)
	}
}
