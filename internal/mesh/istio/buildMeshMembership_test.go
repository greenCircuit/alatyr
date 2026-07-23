package istio

import (
	"errors"
	"log/slog"
	"strings"
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

// ptr returns the address of v — inline helper for building *WorkloadNode
// NSNode fields inside map literals.
func ptr[T any](v T) *T { return &v }

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

// Non-enrolled ns: every workload gets a bare InMesh=false membership (no
// provider, no Mtls — nothing invented for workloads outside the mesh).
func TestBuildMeshMembership_NonEnrolledNs(t *testing.T) {
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

	if client.callsByNs[RootNamespace] != 1 {
		t.Errorf("root PA fetches: want 1, got %d", client.callsByNs[RootNamespace])
	}
	for _, id := range []string{"wl1", "wl2"} {
		m, ok := result.Memberships[id]
		if !ok {
			t.Errorf("%s missing from memberships", id)
			continue
		}
		if m.InMesh || m.Provider != "" || m.Mtls != nil {
			t.Errorf("%s: want bare InMesh=false membership, got %+v", id, m)
		}
	}
	if result.Metrics.WorkloadsEnrolled != 0 {
		t.Errorf("WorkloadsEnrolled: want 0, got %d", result.Metrics.WorkloadsEnrolled)
	}
	if result.Metrics.NsEnrolled != 0 {
		t.Errorf("NsEnrolled: want 0, got %d", result.Metrics.NsEnrolled)
	}
}

// Root PA fetch error degrades: no error returned, mesh build continues,
// membership still resolves from labels, but every enrolled workload's mTLS
// verdict goes explicit-unknown (root fallback unavailable → any resolved
// verdict could lie). Failure surfaces as an IssuesFailedToFetch issue.
// Prevents "one flaky RBAC row tears down /api/graph".
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
	if m.Mtls == nil || m.Mtls.Verdict != models.MeshUnknown {
		t.Errorf("wl1 verdict must be explicit unknown under root failure, got %+v", m.Mtls)
	}
	if result.Metrics.MtlsUnknown != 1 {
		t.Errorf("MtlsUnknown: want 1, got %d", result.Metrics.MtlsUnknown)
	}
	var found bool
	for _, issue := range result.Issues {
		if issue.Type == models.IssuesFailedToFetch && issue.Engine == SourceName &&
			containsSubstring(issue.Message, "root boom") {
			found = true
		}
	}
	if !found {
		t.Errorf("root PA failure should surface as IssuesFailedToFetch issue with error text, got %+v", result.Issues)
	}
}

// Per-ns PA fetch error degrades: no error returned, other namespaces still
// resolve, enrolled workloads in the failed ns are still marked InMesh (from
// labels) with an explicit unknown mTLS verdict — and opted-out workloads
// stay InMesh=false, not blanket-stamped true. Failure surfaces as an
// IssuesFailedToFetch issue naming the ns.
func TestBuildMeshMembership_NsPaErrorDegrades(t *testing.T) {
	sentinel := errors.New("ns boom")
	strictPA := makePA("ns-strict", "ns-good", istioapi.PeerAuthentication_MutualTLS_STRICT, nil, time.Unix(100, 0))
	client := &fakePAClient{
		paByNs:  map[string][]*istiosec.PeerAuthentication{RootNamespace: nil, "ns-good": {strictPA}},
		errByNs: map[string]error{"ns-bad": sentinel},
	}
	idx := map[string]models.NSIndex{
		"ns-bad": ambientNs("ns-bad",
			models.WorkloadNode{ID: "wl-bad", Namespace: "ns-bad"},
			models.WorkloadNode{ID: "wl-bad-out", Namespace: "ns-bad",
				Labels: map[string]string{AmbientEnrollmentKey: AmbientSkipValue}},
		),
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
	// The failing ns still marks enrolled workloads InMesh, verdict unknown.
	bad, ok := result.Memberships["wl-bad"]
	if !ok {
		t.Fatalf("wl-bad missing from memberships despite degrade")
	}
	if !bad.InMesh {
		t.Errorf("wl-bad should still be InMesh from ns label, got %+v", bad)
	}
	if bad.Mtls == nil || bad.Mtls.Verdict != models.MeshUnknown {
		t.Errorf("wl-bad verdict must be explicit unknown, got %+v", bad.Mtls)
	}
	// Only the failing ns's enrolled workload counts unknown; the healthy
	// ns keeps its strict bucket. unknown + strict == enrolled.
	if result.Metrics.MtlsUnknown != 1 || result.Metrics.MtlsStrict != 1 {
		t.Errorf("mtls buckets: want unknown=1 strict=1, got unknown=%d strict=%d",
			result.Metrics.MtlsUnknown, result.Metrics.MtlsStrict)
	}
	// Opt-out label still wins during the degrade path.
	if result.Memberships["wl-bad-out"].InMesh {
		t.Errorf("wl-bad-out opted out; degrade path must not stamp it InMesh, got %+v",
			result.Memberships["wl-bad-out"])
	}
	var found bool
	for _, issue := range result.Issues {
		if issue.Type == models.IssuesFailedToFetch && issue.Engine == SourceName &&
			containsSubstring(issue.Message, "ns boom") && containsSubstring(issue.Message, "ns-bad") {
			found = true
		}
	}
	if !found {
		t.Errorf("ns PA failure should surface as IssuesFailedToFetch issue naming the ns, got %+v", result.Issues)
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

// Workload label overrides ns label. Ambient ns + opted-out workloads =
// InMesh=false with nil Mtls for those workloads, NsPartial bumped exactly
// once per ns regardless of how many opt out. nil PA lists throughout also
// guard the resolveMtls-returns-nil path against panics.
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
			models.WorkloadNode{ID: "wl-out2", Namespace: "ns-a",
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
	for _, id := range []string{"wl-out", "wl-out2"} {
		m := result.Memberships[id]
		if m.InMesh || m.Mtls != nil {
			t.Errorf("%s label opt-out ignored: %+v", id, m)
		}
	}
	if result.Metrics.NsPartial != 1 {
		t.Errorf("NsPartial: want 1 (counted once per ns, not per workload), got %d", result.Metrics.NsPartial)
	}
	if result.Metrics.WorkloadsEnrolled != 1 {
		t.Errorf("WorkloadsEnrolled: want 1, got %d", result.Metrics.WorkloadsEnrolled)
	}
}

// Ns-node (Type=NodeTypeNamespace) in the Workloads slice gets its own ns-level
// membership so the mesh-membership filter can select enrolled namespaces. Its
// mТLS is the namespace default — the ns-scoped → root PA precedence walk a
// label-less workload would inherit (STRICT ns PA wins here; no PA → PERMISSIVE
// default). Must NOT inflate WorkloadsEnrolled.
func TestBuildMeshMembership_NsNodeMembership(t *testing.T) {
	nsStrictPA := makePA("ns-default", "ns-a", istioapi.PeerAuthentication_MutualTLS_STRICT, nil, time.Unix(0, 0))
	client := &fakePAClient{
		paByNs: map[string][]*istiosec.PeerAuthentication{
			RootNamespace: nil, "ns-a": {nsStrictPA}, "ns-def": nil, "ns-plain": nil, IngressNamespace: nil,
		},
	}
	nsNode := func(ns string, labels map[string]string) models.WorkloadNode {
		return models.WorkloadNode{ID: "ns/" + ns, Namespace: ns, Type: models.NodeTypeNamespace, Labels: labels}
	}
	ambientLabels := map[string]string{AmbientEnrollmentKey: AmbientEnrollmentValue}
	idx := map[string]models.NSIndex{
		"ns-a": {
			NSNode:    ptr(nsNode("ns-a", ambientLabels)),
			Workloads: []models.WorkloadNode{{ID: "wl1", Namespace: "ns-a"}, nsNode("ns-a", ambientLabels)},
		},
		// Ambient ns with no PA → ns default is PERMISSIVE (install default).
		"ns-def": {
			NSNode:    ptr(nsNode("ns-def", ambientLabels)),
			Workloads: []models.WorkloadNode{nsNode("ns-def", ambientLabels)},
		},
		"ns-plain": {
			NSNode:    ptr(nsNode("ns-plain", nil)),
			Workloads: []models.WorkloadNode{nsNode("ns-plain", nil)},
		},
		// System ns carries no ambient label but must still read in-mesh.
		IngressNamespace: {
			NSNode:    ptr(nsNode(IngressNamespace, nil)),
			Workloads: []models.WorkloadNode{nsNode(IngressNamespace, nil)},
		},
	}

	result, err := buildSource(client).BuildMeshMembership(idx)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	// Ambient ns with a STRICT ns-scoped PA → in mesh, provider stamped, verdict
	// resolved to STRICT (the default an unselected workload inherits).
	enrolled := result.Memberships["ns/ns-a"]
	if !enrolled.InMesh || enrolled.Provider != SourceName || enrolled.Mtls == nil || enrolled.Mtls.Verdict != models.MeshStrict {
		t.Errorf("ns/ns-a: want in-mesh, provider set, STRICT mТLS; got %+v", enrolled)
	}
	// Ambient ns with no PA → PERMISSIVE default, never UNSET/nil.
	if def := result.Memberships["ns/ns-def"]; !def.InMesh || def.Mtls == nil || def.Mtls.Verdict != models.MeshPermissive {
		t.Errorf("ns/ns-def: want in-mesh with PERMISSIVE default; got %+v", def)
	}
	// System ns → in mesh even without the ambient label.
	if !result.Memberships["ns/"+IngressNamespace].InMesh {
		t.Errorf("ns/%s: system namespace must read in-mesh, got %+v", IngressNamespace, result.Memberships["ns/"+IngressNamespace])
	}
	// Out-of-mesh ns → no verdict invented.
	if plain := result.Memberships["ns/ns-plain"]; plain.InMesh || plain.Mtls != nil {
		t.Errorf("ns/ns-plain: unlabeled ns must be out of mesh with nil mТLS, got %+v", plain)
	}
	// Ns nodes never count as workloads.
	if result.Metrics.WorkloadsEnrolled != 1 {
		t.Errorf("WorkloadsEnrolled: want 1 (wl1 only), got %d", result.Metrics.WorkloadsEnrolled)
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
	var sawDuplicateCulprits bool
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
		// dupDetail's refs (pa1 = winner, pa2 = ignored) must survive the
		// mtls.Issues → models.Issue promotion — CulpritActions.tsx renders
		// them as clickable "in use"/"ignored" chips, an empty Culprits list
		// silently drops that back to a message-only row.
		if strings.Contains(issue.Message, IssueDuplicateAtScope) {
			sawDuplicateCulprits = true
			want := []models.PolicyRef{
				{Source: "pa", Namespace: "ns-a", Name: "dup-a"},
				{Source: "pa", Namespace: "ns-a", Name: "dup-b"},
			}
			if len(issue.Culprits) != 2 || issue.Culprits[0] != want[0] || issue.Culprits[1] != want[1] {
				t.Errorf("duplicate issue culprits: want %+v, got %+v", want, issue.Culprits)
			}
		}
	}
	if !sawDuplicateCulprits {
		t.Fatalf("expected a duplicate-at-scope issue in %+v", result.Issues)
	}
}

// Per-workload enrollment — regression test for the review finding "workload
// opt-in on non-ambient ns silently dropped". A workload carrying the ambient
// label in an unlabeled ns must get full membership including the PA
// precedence walk (root PA verdict must land on it), while its unlabeled
// neighbor stays out. Root/ingress system namespaces participate without any
// label — ztunnel/gateway always speak HBONE.
func TestBuildMeshMembership_PerWorkloadEnrollment(t *testing.T) {
	rootStrict := makePA("mesh-default", RootNamespace, istioapi.PeerAuthentication_MutualTLS_STRICT, nil, time.Unix(100, 0))
	client := &fakePAClient{
		paByNs: map[string][]*istiosec.PeerAuthentication{
			RootNamespace: {rootStrict},
		},
	}
	idx := map[string]models.NSIndex{
		"ns-plain": plainNs("ns-plain",
			models.WorkloadNode{ID: "wl-optin", Namespace: "ns-plain",
				Labels: map[string]string{AmbientEnrollmentKey: AmbientEnrollmentValue}},
			models.WorkloadNode{ID: "wl-plain", Namespace: "ns-plain"},
		),
		RootNamespace:    plainNs(RootNamespace, models.WorkloadNode{ID: "wl-ztunnel", Namespace: RootNamespace}),
		IngressNamespace: plainNs(IngressNamespace, models.WorkloadNode{ID: "wl-gw", Namespace: IngressNamespace}),
	}

	result, err := buildSource(client).BuildMeshMembership(idx)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}

	// The review-finding case: opt-in label in an unlabeled ns. Graph and
	// detail panel must agree — full membership, PA walk included.
	optin := result.Memberships["wl-optin"]
	if !optin.InMesh || optin.Provider != SourceName || optin.Mode != "ambient" {
		t.Errorf("wl-optin: workload label must win over unlabeled ns, got %+v", optin)
	}
	if optin.Mtls == nil || optin.Mtls.Verdict != models.MeshStrict {
		t.Errorf("wl-optin: root PA precedence walk must run for opt-in workload, got %+v", optin.Mtls)
	}
	// Unlabeled neighbor in the same ns stays out.
	if result.Memberships["wl-plain"].InMesh {
		t.Errorf("wl-plain: unlabeled workload in unlabeled ns must stay out, got %+v",
			result.Memberships["wl-plain"])
	}
	// System namespaces participate without labels.
	for _, id := range []string{"wl-ztunnel", "wl-gw"} {
		if !result.Memberships[id].InMesh {
			t.Errorf("%s: root/ingress workloads always participate, got %+v", id, result.Memberships[id])
		}
	}
	if result.Metrics.WorkloadsEnrolled != 3 {
		t.Errorf("WorkloadsEnrolled: want 3 (optin + ztunnel + gw), got %d", result.Metrics.WorkloadsEnrolled)
	}
	if result.Metrics.NsEnrolled != 2 {
		t.Errorf("NsEnrolled: want 2 (root + ingress only), got %d", result.Metrics.NsEnrolled)
	}
}

// Real ambient system-ns names. A typo'd constant (P0#2: "istio ingress",
// space for hyphen) passes every constant-vs-constant test while missing
// real clusters — literals here pin the contract.
func TestSystemNamespaceConstants(t *testing.T) {
	if RootNamespace != "istio-system" {
		t.Errorf("RootNamespace: want istio-system, got %q", RootNamespace)
	}
	if IngressNamespace != "istio-ingress" {
		t.Errorf("IngressNamespace: want istio-ingress, got %q", IngressNamespace)
	}
}

// NSNode can be nil when the ns object fetch raced a deletion. Must not
// panic; enrollment resolves from workload labels alone.
func TestBuildMeshMembership_NilNSNode(t *testing.T) {
	client := &fakePAClient{
		paByNs: map[string][]*istiosec.PeerAuthentication{RootNamespace: nil},
	}
	idx := map[string]models.NSIndex{
		"ns-gone": {
			Workloads: []models.WorkloadNode{
				{ID: "wl-optin", Namespace: "ns-gone",
					Labels: map[string]string{AmbientEnrollmentKey: AmbientEnrollmentValue}},
				{ID: "wl-plain", Namespace: "ns-gone"},
			},
		},
	}

	result, err := buildSource(client).BuildMeshMembership(idx)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !result.Memberships["wl-optin"].InMesh {
		t.Errorf("wl-optin: workload label must still enroll under nil NSNode, got %+v",
			result.Memberships["wl-optin"])
	}
	if result.Memberships["wl-plain"].InMesh {
		t.Errorf("wl-plain: want InMesh=false, got %+v", result.Memberships["wl-plain"])
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
	if result.Metrics.NsEnrolled != 2 {
		t.Errorf("NsEnrolled: want 2, got %d", result.Metrics.NsEnrolled)
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
