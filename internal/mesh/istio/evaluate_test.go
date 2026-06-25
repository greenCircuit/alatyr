package istio

import (
	"testing"

	"graph/internal/models"
)

func ambientMembership() *models.MeshMembership {
	return &models.MeshMembership{InMesh: true, Provider: SourceName, Mode: "ambient"}
}

// allowRule builds an ALLOW NodeRule with one contributing policy.
func allowRule(direction models.Direction, dstID string, ports []int) models.NodeRule {
	rulePorts := make([]models.Port, 0, len(ports))
	for _, p := range ports {
		rulePorts = append(rulePorts, models.Port{Port: p, Protocol: "TCP"})
	}
	return models.NodeRule{
		Direction:   direction,
		Action:      models.ActionAllow,
		Ports:       rulePorts,
		DstID:       dstID,
		Contributor: models.PolicyRef{Name: "np1", Namespace: "ns-a"},
	}
}

func policies(rules ...models.NodeRule) map[string]models.NodeInfo {
	return map[string]models.NodeInfo{"k8s": {Rules: rules}}
}

// allowRuleWithPolicy builds an ALLOW NodeRule with a custom contributing policy.
// Used by tests that need to distinguish multiple policies / engines on the same dst.
func allowRuleWithPolicy(direction models.Direction, dstID string, ports []int, source, name, namespace string) models.NodeRule {
	r := allowRule(direction, dstID, ports)
	r.Contributor = models.PolicyRef{Source: source, Name: name, Namespace: namespace}
	return r
}

// Non-ambient workload: ValidateExternalRules has no opinion.
func TestValidateExternalRules_NotInMesh(t *testing.T) {
	membership := &models.MeshMembership{InMesh: false}
	errors := ValidateExternalRules(membership, policies(allowRule(models.DirectionEgress, "dst1", []int{8080})))
	if len(errors) != 0 {
		t.Fatalf("expected no errors for non-mesh workload, got %v", errors)
	}
}

// Egress rule restricting to a non-HBONE port → flagged with the ztunnel port.
func TestValidateExternalRules_EgressMissingHBONE(t *testing.T) {
	errors := ValidateExternalRules(ambientMembership(), policies(allowRule(models.DirectionEgress, "dst1", []int{8080})))
	if len(errors) == 0 {
		t.Fatal("expected an egress error, got none")
	}
	if !hasIssue(errors, "egress") || !hasIssue(errors, "15008") {
		t.Fatalf("error should mention egress + 15008: %v", errors)
	}
}

// Ingress rule restricting to a non-HBONE port → flagged on the ingress side.
func TestValidateExternalRules_IngressMissingHBONE(t *testing.T) {
	errors := ValidateExternalRules(ambientMembership(), policies(allowRule(models.DirectionIngress, "dst1", []int{8080})))
	if !hasIssue(errors, "ingress") || !hasIssue(errors, "15008") {
		t.Fatalf("error should mention ingress + 15008: %v", errors)
	}
}

// Port 0 = all ports open → ztunnel HBONE port is covered, no error.
func TestValidateExternalRules_PortZeroAllowsAll(t *testing.T) {
	errors := ValidateExternalRules(ambientMembership(), policies(allowRule(models.DirectionEgress, "dst1", []int{})))
	if len(errors) != 0 {
		t.Fatalf("port 0 opens all ports; expected no error, got %v", errors)
	}
}

// A rule explicitly allowing the HBONE port → no error.
func TestValidateExternalRules_HBONEPortAllowed(t *testing.T) {
	errors := ValidateExternalRules(ambientMembership(), policies(allowRule(models.DirectionEgress, "dst1", []int{ZtunnelHBONEPort})))
	if len(errors) != 0 {
		t.Fatalf("HBONE port allowed; expected no error, got %v", errors)
	}
}

// Ingress rule listing both 8080 and HBONE → no error (HBONE covered within the rule).
func TestValidateExternalRules_MultiplePortsIngress(t *testing.T) {
	errors := ValidateExternalRules(ambientMembership(),
		policies(allowRule(models.DirectionIngress, "dst1", []int{8080, ZtunnelHBONEPort})),
	)
	if len(errors) != 0 {
		t.Fatalf("HBONE port present in port list, got %v", errors)
	}
}

// Egress rule listing both 8080 and HBONE → no error.
func TestValidateExternalRules_MultiplePortsEgress(t *testing.T) {
	errors := ValidateExternalRules(ambientMembership(),
		policies(allowRule(models.DirectionEgress, "dst1", []int{8080, ZtunnelHBONEPort})),
	)
	if len(errors) != 0 {
		t.Fatalf("HBONE port present in port list, got %v", errors)
	}
}

// Same policy, both directions, HBONE present on each → no error.
func TestValidateExternalRules_MultiplePortsMixedPass1(t *testing.T) {
	errors := ValidateExternalRules(ambientMembership(),
		policies(
			allowRule(models.DirectionEgress, "dst1", []int{8080, ZtunnelHBONEPort}),
			allowRule(models.DirectionIngress, "dst1", []int{ZtunnelHBONEPort}),
		),
	)
	if len(errors) != 0 {
		t.Fatalf("HBONE port present in each direction, got %v", errors)
	}
}

// Egress missing HBONE, ingress has HBONE → exactly 1 error (egress only).
func TestValidateExternalRules_MultiplePortsMixedPass2(t *testing.T) {
	errors := ValidateExternalRules(ambientMembership(),
		policies(
			allowRule(models.DirectionEgress, "dst1", []int{8080}),
			allowRule(models.DirectionIngress, "dst1", []int{ZtunnelHBONEPort, 8080, 8443}),
		),
	)
	if len(errors) != 1 {
		t.Fatalf("expected 1 error (egress only), got %v", errors)
	}
}

// Single egress rule listing many ports including HBONE → no error.
func TestValidateExternalRules_MultipleEgress(t *testing.T) {
	errors := ValidateExternalRules(ambientMembership(),
		policies(allowRule(models.DirectionEgress, "dst1", []int{8080, ZtunnelHBONEPort, 8443})),
	)
	if len(errors) != 0 {
		t.Fatalf("HBONE port present in port list, got %v", errors)
	}
}

// Single ingress rule listing many ports including HBONE → no error.
func TestValidateExternalRules_MultipleIngress(t *testing.T) {
	errors := ValidateExternalRules(ambientMembership(),
		policies(allowRule(models.DirectionIngress, "dst1", []int{8080, ZtunnelHBONEPort, 8443})),
	)
	if len(errors) != 0 {
		t.Fatalf("HBONE port present in port list, got %v", errors)
	}
}

// Single ingress rule, no HBONE in port list → exactly 1 error.
func TestValidateExternalRules_DedupErrorsIngress(t *testing.T) {
	errors := ValidateExternalRules(ambientMembership(),
		policies(allowRule(models.DirectionIngress, "dst1", []int{8080, 8443})),
	)
	if len(errors) != 1 {
		t.Fatalf("expected 1 error, got %v", errors)
	}
}

// Single egress rule, no HBONE in port list → exactly 1 error.
func TestValidateExternalRules_DedupErrorsEgress(t *testing.T) {
	errors := ValidateExternalRules(ambientMembership(),
		policies(allowRule(models.DirectionEgress, "dst1", []int{8080, 8443})),
	)
	if len(errors) != 1 {
		t.Fatalf("expected 1 error, got %v", errors)
	}
}

// Two ingress rules to different dsts, both missing HBONE → 2 errors.
// Guards against bucket collapse across destinations.
func TestValidateExternalRules_MultipleDestinations(t *testing.T) {
	errors := ValidateExternalRules(ambientMembership(),
		policies(
			allowRule(models.DirectionIngress, "dst1", []int{8080}),
			allowRule(models.DirectionIngress, "dst2", []int{8080}),
		),
	)
	if len(errors) != 2 {
		t.Fatalf("expected 2 errors (one per dst), got %v", errors)
	}
}

// Two ingress rules from different policies, same dst, both missing HBONE → 2 errors.
// Guards against collapse by dst alone (would lose per-policy attribution).
func TestValidateExternalRules_MultiplePoliciesSameDst(t *testing.T) {
	errors := ValidateExternalRules(ambientMembership(),
		policies(
			allowRuleWithPolicy(models.DirectionIngress, "dst1", []int{8080}, "k8s", "np-a", "ns-a"),
			allowRuleWithPolicy(models.DirectionIngress, "dst1", []int{8080}, "k8s", "np-b", "ns-a"),
		),
	)
	if len(errors) != 2 {
		t.Fatalf("expected 2 errors (one per policy), got %v", errors)
	}
	if !hasIssue(errors, "np-a") || !hasIssue(errors, "np-b") {
		t.Fatalf("each policy name should appear: %v", errors)
	}
}

// Offending rules under two engine keys → both engines surface errors.
// Guards against the outer-loop engine collapse.
func TestValidateExternalRules_MultipleEngines(t *testing.T) {
	nodePolicies := map[string]models.NodeInfo{
		"k8s": {Rules: []models.NodeRule{
			allowRuleWithPolicy(models.DirectionIngress, "dst1", []int{8080}, "k8s", "np-k8s", "ns-a"),
		}},
		"istio": {Rules: []models.NodeRule{
			allowRuleWithPolicy(models.DirectionIngress, "dst1", []int{8080}, "istio", "ap-istio", "ns-a"),
		}},
	}
	errors := ValidateExternalRules(ambientMembership(), nodePolicies)
	if len(errors) != 2 {
		t.Fatalf("expected 2 errors (one per engine), got %v", errors)
	}
	if !hasIssue(errors, "np-k8s") || !hasIssue(errors, "ap-istio") {
		t.Fatalf("each engine's policy should appear: %v", errors)
	}
}

// Error message must carry the contributor's source, name, and namespace verbatim.
// Guards against argument-order swaps in the Sprintf and missing-field regressions.
func TestValidateExternalRules_ErrorAttribution(t *testing.T) {
	errors := ValidateExternalRules(ambientMembership(),
		policies(allowRuleWithPolicy(models.DirectionEgress, "dst1", []int{8080}, "k8s", "np-attr", "ns-attr")),
	)
	if len(errors) != 1 {
		t.Fatalf("expected 1 error, got %v", errors)
	}
	got := errors[0]
	for _, want := range []string{"name: np-attr", "ns: ns-attr", "egress", "15008"} {
		if !hasIssue([]string{got}, want) {
			t.Fatalf("error missing %q: %s", want, got)
		}
	}
}