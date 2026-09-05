package istio

import (
	"testing"

	"alatyr/internal/models"
)

// TestIsIstioComponent pins the control-plane / gateway detection contract:
// a workload counts only when BOTH its identifying label and its namespace
// match — the ingress gateway lives in IngressNamespace, everything else
// (istiod, ztunnel, istio-cni) in RootNamespace. The namespace guard stops an
// app in a user namespace that happens to reuse the label (app=istiod) from
// being mistaken for the control plane.
func TestIsIstioComponent(t *testing.T) {
	cases := []struct {
		name   string
		ns     string
		labels map[string]string
		want   bool
	}{
		{"ingress gateway", IngressNamespace, map[string]string{istioSelector: gatewayVal}, true},
		{"istiod", RootNamespace, map[string]string{istioSelector: istiodVal}, true},
		{"ztunnel", RootNamespace, map[string]string{istioSelector: ztunelVal}, true},
		{"istio-cni", RootNamespace, map[string]string{istioCniKey: istioCniVal}, true},

		// gateway carries dataplane-mode:none in the real cluster — detection
		// must not care about the ambient opt-out label.
		{"ingress gateway with opt-out", IngressNamespace,
			map[string]string{istioSelector: gatewayVal, AmbientEnrollmentKey: AmbientSkipValue}, true},

		// namespace guard: right label, wrong ns.
		{"gateway label outside ingress ns", RootNamespace, map[string]string{istioSelector: gatewayVal}, false},
		{"istiod label in user ns", "team-a", map[string]string{istioSelector: istiodVal}, false},
		{"cni label outside root ns", "team-a", map[string]string{istioCniKey: istioCniVal}, false},

		// ordinary workloads.
		{"plain app in user ns", "team-a", map[string]string{istioSelector: "checkout"}, false},
		{"no labels", RootNamespace, nil, false},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			workload := models.WorkloadNode{Namespace: testCase.ns, Labels: testCase.labels}
			if got := isIstioComponent(workload); got != testCase.want {
				t.Errorf("isIstioComponent(%s) = %v, want %v", testCase.name, got, testCase.want)
			}
		})
	}
}

// TestBuildMeshMembership_IngressGatewayInMesh is the end-to-end guard: the
// ingress gateway (app=istio-ingress + dataplane-mode:none) must land InMesh in
// cache.MeshMembership so tables and node-info both read it as a mesh member.
func TestBuildMeshMembership_IngressGatewayInMesh(t *testing.T) {
	gateway := models.WorkloadNode{
		ID:        "wl-gw",
		Namespace: IngressNamespace,
		Labels:    map[string]string{istioSelector: gatewayVal, AmbientEnrollmentKey: AmbientSkipValue},
	}
	idx := map[string]models.NSIndex{
		IngressNamespace: plainNs(IngressNamespace, gateway),
	}

	result, err := buildSource(&fakePAClient{}).BuildMeshMembership(idx)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !result.Memberships["wl-gw"].InMesh {
		t.Errorf("ingress gateway must be InMesh despite opt-out label, got %+v", result.Memberships["wl-gw"])
	}
}
