package policy

import (
	"testing"

	"graph/internal/models"
)

// Empty input → zero-value PolicyStatus.
func TestIntersectPolicyStatus_Empty(t *testing.T) {
	got := IntersectPolicyStatus(nil)
	if got != (models.PolicyStatus{}) {
		t.Errorf("empty input: want zero PolicyStatus, got %+v", got)
	}
}

// Single engine → pass through unchanged (no intersection needed).
func TestIntersectPolicyStatus_SingleEngine(t *testing.T) {
	input := models.PolicyStatus{
		InternetEgress: true,
		LanEgress:      true,
		EgressLocked:   true,
		CrossNS:        true,
	}
	got := IntersectPolicyStatus([]models.PolicyStatus{input})
	if got != input {
		t.Errorf("single engine: want pass-through, got %+v", got)
	}
}

// Access fields AND: both must permit for traffic to flow. Both engines
// lock egress so "X=false" expresses explicit deny rather than transparency.
func TestIntersectPolicyStatus_AccessFieldsAND(t *testing.T) {
	first := models.PolicyStatus{EgressLocked: true, InternetEgress: true, LanEgress: true}
	second := models.PolicyStatus{EgressLocked: true, InternetEgress: true, LanEgress: false}
	got := IntersectPolicyStatus([]models.PolicyStatus{first, second})
	if !got.InternetEgress {
		t.Error("InternetEgress: both true → expected true")
	}
	if got.LanEgress {
		t.Error("LanEgress: one false → expected false (AND)")
	}
}

// Lock fields OR: any engine locking the direction restricts effective state.
func TestIntersectPolicyStatus_LockFieldsOR(t *testing.T) {
	first := models.PolicyStatus{EgressLocked: false, IngressLocked: true}
	second := models.PolicyStatus{EgressLocked: true, IngressLocked: false}
	got := IntersectPolicyStatus([]models.PolicyStatus{first, second})
	if !got.EgressLocked {
		t.Error("EgressLocked: one engine locked → expected true (OR)")
	}
	if !got.IngressLocked {
		t.Error("IngressLocked: one engine locked → expected true (OR)")
	}
}

// k8s LAN-only + Istio with no egress policy → effective = LAN-only.
// Istio expresses "doesn't constrain egress" by leaving EgressLocked=false;
// the intersection treats that engine as transparent for every egress
// dimension (it can't subtract access in the AND).
func TestIntersectPolicyStatus_DefaultOpenEngineDoesNotSubtract(t *testing.T) {
	k8sStatus := models.PolicyStatus{
		EgressLocked:   true,
		LanEgress:      true,
		InnerNsEgress:  true,
		InternetEgress: false,
	}
	istioOpen := models.PolicyStatus{} // no egress policy → transparent per intent
	got := IntersectPolicyStatus([]models.PolicyStatus{k8sStatus, istioOpen})
	if got.InternetEgress {
		t.Error("InternetEgress: k8s denies → expected false even though Istio open")
	}
	if !got.LanEgress {
		t.Error("LanEgress: both permit → expected true")
	}
	if !got.InnerNsEgress {
		t.Error("InnerNsEgress: both permit → expected true")
	}
}

// Air-gapped engine (no access, both locked) intersected with an engine
// that has no policy at all → air-gapped wins. Transparency from the
// no-policy engine doesn't open access the locked engine has closed.
func TestIntersectPolicyStatus_AirGappedWinsOverOpen(t *testing.T) {
	airGapped := models.PolicyStatus{EgressLocked: true, IngressLocked: true}
	open := models.PolicyStatus{} // no policy → transparent on every dimension
	got := IntersectPolicyStatus([]models.PolicyStatus{airGapped, open})
	if got.InternetEgress || got.InternetIngress || got.LanEgress || got.LanIngress || got.ApiServerEgress {
		t.Errorf("air-gapped engine: expected all access false, got %+v", got)
	}
	if !got.EgressLocked || !got.IngressLocked {
		t.Error("locks: at least one engine locked each direction → expected both true")
	}
}

// Asymmetric directions: k8s allows LAN out + internet in,
// Istio allows internet out + LAN in. Intersection per direction = LAN both ways.
func TestIntersectPolicyStatus_AsymmetricDirections(t *testing.T) {
	k8sStatus := models.PolicyStatus{
		EgressLocked:    true,
		IngressLocked:   true,
		LanEgress:       true,
		InnerNsEgress:   true,
		InternetIngress: true,
		LanIngress:      true,
		InnerNsIngress:  true,
	}
	istioStatus := models.PolicyStatus{
		EgressLocked:   true,
		IngressLocked:  true,
		InternetEgress: true,
		LanEgress:      true,
		InnerNsEgress:  true,
		LanIngress:     true,
		InnerNsIngress: true,
	}
	got := IntersectPolicyStatus([]models.PolicyStatus{k8sStatus, istioStatus})
	if got.InternetEgress {
		t.Error("InternetEgress: k8s denies → expected false")
	}
	if got.InternetIngress {
		t.Error("InternetIngress: Istio denies → expected false")
	}
	if !got.LanEgress || !got.LanIngress {
		t.Error("LAN both directions: both permit → expected both true")
	}
}

// CrossNS and ApiServerEgress: orthogonal flags, AND'd across engines.
// Both engines lock both directions so "X=false" means explicit deny.
// (Unlocked engines would be transparent and contribute true regardless of
// raw values — covered by the DefaultOpen test above.)
func TestIntersectPolicyStatus_OrthogonalFieldsAND(t *testing.T) {
	first := models.PolicyStatus{EgressLocked: true, IngressLocked: true, CrossNS: true, ApiServerEgress: true}
	second := models.PolicyStatus{EgressLocked: true, IngressLocked: true, CrossNS: true, ApiServerEgress: false}
	got := IntersectPolicyStatus([]models.PolicyStatus{first, second})
	if !got.CrossNS {
		t.Error("CrossNS: both true → expected true")
	}
	if got.ApiServerEgress {
		t.Error("ApiServerEgress: one false → expected false")
	}
}

// Regression: alert-digest scenario. Istio asserts "internet full" (no egress
// policy → transparent egress + explicit internet ingress allowed); k8s
// asserts "internet egress only" (egress + ingress locked, only InternetEgress
// granted). Effective view should be "internet-egress" — k8s blocks ingress
// from internet, istio is transparent for egress so doesn't subtract.
// Before the intersection rewrite this produced "air-gapped".
func TestIntersectPolicyStatus_IstioInternetFullPlusK8sInternetEgress(t *testing.T) {
	istioFullInternet := models.PolicyStatus{
		IngressLocked:   true, // istio has an ingress policy
		InternetIngress: true, // policy explicitly allows from 0.0.0.0/0
		// EgressLocked stays false: istio doesn't gate egress → transparent
	}
	k8sInternetEgress := models.PolicyStatus{
		EgressLocked:   true,
		IngressLocked:  true, // k8s policy with no ingress rules still defaults Ingress locked
		InternetEgress: true,
	}

	got := IntersectPolicyStatus([]models.PolicyStatus{istioFullInternet, k8sInternetEgress})
	keys := DeriveStatusKeys(got)

	hasInternetEgress := false
	for _, key := range keys {
		if key == models.StatusInternetEgress {
			hasInternetEgress = true
		}
		if key == models.StatusIsolated {
			t.Errorf("StatusIsolated set on effective status — should be internet-egress only; got %v", keys)
		}
		if key == models.StatusInternetIngress || key == models.StatusInternetFull {
			t.Errorf("unexpected internet ingress key %q in effective status; got %v", key, keys)
		}
	}
	if !hasInternetEgress {
		t.Errorf("StatusInternetEgress missing from effective status; got %v", keys)
	}
}
