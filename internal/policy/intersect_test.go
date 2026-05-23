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

// Access fields AND: both must permit for traffic to flow.
func TestIntersectPolicyStatus_AccessFieldsAND(t *testing.T) {
	first := models.PolicyStatus{InternetEgress: true, LanEgress: true}
	second := models.PolicyStatus{InternetEgress: true, LanEgress: false}
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

// k8s LAN-only + Istio default-open (all access true) → effective = LAN-only.
// The "doesn't constrain = true" convention lets Istio not subtract anything.
func TestIntersectPolicyStatus_DefaultOpenEngineDoesNotSubtract(t *testing.T) {
	k8sStatus := models.PolicyStatus{
		EgressLocked:   true,
		LanEgress:      true,
		InnerNsEgress:  true,
		InternetEgress: false,
	}
	istioOpen := models.PolicyStatus{
		InternetEgress: true,
		LanEgress:      true,
		InnerNsEgress:  true,
	}
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

// Air-gapped engine (no access, both locked) intersected with open engine
// → air-gapped wins. AND drops every access field; OR keeps locks.
func TestIntersectPolicyStatus_AirGappedWinsOverOpen(t *testing.T) {
	airGapped := models.PolicyStatus{EgressLocked: true, IngressLocked: true}
	open := models.PolicyStatus{
		InternetEgress:  true,
		InternetIngress: true,
		LanEgress:       true,
		LanIngress:      true,
		ApiServerEgress: true,
	}
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
func TestIntersectPolicyStatus_OrthogonalFieldsAND(t *testing.T) {
	first := models.PolicyStatus{CrossNS: true, ApiServerEgress: true}
	second := models.PolicyStatus{CrossNS: true, ApiServerEgress: false}
	got := IntersectPolicyStatus([]models.PolicyStatus{first, second})
	if !got.CrossNS {
		t.Error("CrossNS: both true → expected true")
	}
	if got.ApiServerEgress {
		t.Error("ApiServerEgress: one false → expected false")
	}
}
