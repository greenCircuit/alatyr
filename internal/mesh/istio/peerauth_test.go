package istio

import (
	"strings"
	"testing"
	"time"

	"graph/internal/models"

	istioapi "istio.io/api/security/v1beta1"
	istioapitype "istio.io/api/type/v1beta1"
	istiosec "istio.io/client-go/pkg/apis/security/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// makePA builds a PeerAuthentication. nil selector → ns-scoped (or mesh-scoped
// in the root ns). created sets creationTimestamp so oldest-wins tie-breaks are
// deterministic in tests.
func makePA(name, namespace string, mode istioapi.PeerAuthentication_MutualTLS_Mode, selector map[string]string, created time.Time) *istiosec.PeerAuthentication {
	var workloadSelector *istioapitype.WorkloadSelector
	if selector != nil {
		workloadSelector = &istioapitype.WorkloadSelector{MatchLabels: selector}
	}
	return &istiosec.PeerAuthentication{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace, CreationTimestamp: metav1.NewTime(created)},
		Spec: istioapi.PeerAuthentication{
			Selector: workloadSelector,
			Mtls:     &istioapi.PeerAuthentication_MutualTLS{Mode: mode},
		},
	}
}

func backendWorkload() models.WorkloadNode {
	return models.WorkloadNode{
		ID:        "backend",
		Label:     "backend",
		Namespace: "ns-a",
		Type:      models.NodeTypeDeployment,
		Labels:    map[string]string{"app": "backend"},
	}
}

func hasIssue(issues []string, code string) bool {
	for _, issue := range issues {
		if strings.Contains(issue, code) {
			return true
		}
	}
	return false
}

// ns-scoped STRICT PA → workload verdict STRICT, effective source points at it.
func TestResolveMtls_NsScopeStrict(t *testing.T) {
	nsPAs := []*istiosec.PeerAuthentication{
		makePA("ns-strict", "ns-a", istioapi.PeerAuthentication_MutualTLS_STRICT, nil, time.Unix(100, 0)),
	}
	state := resolveMtls(backendWorkload(), nsPAs, nil)

	if state.Verdict != models.MeshStrict {
		t.Fatalf("verdict = %q, want strict", state.Verdict)
	}
	if state.EffectiveSource.Name != "ns-strict" {
		t.Fatalf("effective source = %q, want ns-strict", state.EffectiveSource.Name)
	}
	if len(state.Issues) != 0 {
		t.Fatalf("unexpected issues: %v", state.Issues)
	}
}

// Workload-selector PA outranks ns-scoped PA regardless of mode.
func TestResolveMtls_WorkloadBeatsNs(t *testing.T) {
	nsPAs := []*istiosec.PeerAuthentication{
		makePA("ns-permissive", "ns-a", istioapi.PeerAuthentication_MutualTLS_PERMISSIVE, nil, time.Unix(100, 0)),
		makePA("wl-strict", "ns-a", istioapi.PeerAuthentication_MutualTLS_STRICT, map[string]string{"app": "backend"}, time.Unix(200, 0)),
	}
	state := resolveMtls(backendWorkload(), nsPAs, nil)

	if state.Verdict != models.MeshStrict {
		t.Fatalf("verdict = %q, want strict (workload precedence)", state.Verdict)
	}
	if state.EffectiveSource.Name != "wl-strict" {
		t.Fatalf("effective source = %q, want wl-strict", state.EffectiveSource.Name)
	}
}

// Every matching PA UNSET → fall back to install default (permissive) + issue.
func TestResolveMtls_AllUnsetFallsBackToPermissive(t *testing.T) {
	nsPAs := []*istiosec.PeerAuthentication{
		makePA("ns-unset", "ns-a", istioapi.PeerAuthentication_MutualTLS_UNSET, nil, time.Unix(100, 0)),
	}
	state := resolveMtls(backendWorkload(), nsPAs, nil)

	if state.Verdict != models.MeshPermissive {
		t.Fatalf("verdict = %q, want permissive fallback", state.Verdict)
	}
	if !hasIssue(state.Issues, IssueAllUnset) {
		t.Fatalf("missing all-unset issue: %v", state.Issues)
	}
}

// Two PAs at ns scope → oldest wins; the other is flagged as a duplicate.
func TestResolveMtls_DuplicateAtNsScope(t *testing.T) {
	nsPAs := []*istiosec.PeerAuthentication{
		makePA("ns-newer", "ns-a", istioapi.PeerAuthentication_MutualTLS_PERMISSIVE, nil, time.Unix(200, 0)),
		makePA("ns-older", "ns-a", istioapi.PeerAuthentication_MutualTLS_STRICT, nil, time.Unix(100, 0)),
	}
	state := resolveMtls(backendWorkload(), nsPAs, nil)

	if state.Verdict != models.MeshStrict {
		t.Fatalf("verdict = %q, want strict (older PA wins)", state.Verdict)
	}
	if !hasIssue(state.Issues, IssueDuplicateAtScope) {
		t.Fatalf("missing duplicate-at-scope issue: %v", state.Issues)
	}
}

// Root-ns PA with a selector only matches root-ns workloads → for a workload in
// another ns it is ignored and flagged.
func TestResolveMtls_RootSelectorIgnored(t *testing.T) {
	rootPAs := []*istiosec.PeerAuthentication{
		makePA("root-sel", RootNamespace, istioapi.PeerAuthentication_MutualTLS_STRICT, map[string]string{"app": "backend"}, time.Unix(100, 0)),
	}
	state := resolveMtls(backendWorkload(), nil, rootPAs)

	if !hasIssue(state.Issues, IssueRootSelectorIgnored) {
		t.Fatalf("missing root-selector-ignored issue: %v", state.Issues)
	}
	if state.Verdict != models.MeshPermissive {
		t.Fatalf("verdict = %q, want permissive (root selector ignored, nothing applies)", state.Verdict)
	}
}

// Workload STRICT with a port-level DISABLE override surfaces in PortOverrides.
func TestResolveMtls_PortOverride(t *testing.T) {
	pa := makePA("wl-strict", "ns-a", istioapi.PeerAuthentication_MutualTLS_STRICT, map[string]string{"app": "backend"}, time.Unix(100, 0))
	pa.Spec.PortLevelMtls = map[uint32]*istioapi.PeerAuthentication_MutualTLS{
		8080: {Mode: istioapi.PeerAuthentication_MutualTLS_DISABLE},
	}
	state := resolveMtls(backendWorkload(), []*istiosec.PeerAuthentication{pa}, nil)

	if state.Verdict != models.MeshStrict {
		t.Fatalf("verdict = %q, want strict", state.Verdict)
	}
	if got := state.PortOverrides[8080]; got != models.MeshDisable {
		t.Fatalf("port 8080 override = %q, want disable", got)
	}
}
