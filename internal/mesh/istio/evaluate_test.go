package istio

import (
	"testing"

	"graph/internal/models"
)

func ambientMembership() *models.MeshMembership {
	return &models.MeshMembership{InMesh: true, Provider: SourceName, Mode: "ambient"}
}

// allowRule builds an ALLOW NodeRule with one contributing policy.
func allowRule(direction models.Direction, dstID string, port int) models.NodeRule {
	return models.NodeRule{
		Direction:    direction,
		Action:       models.ActionAllow,
		Port:         models.Port{Port: port, Protocol: "TCP"},
		DstID:        dstID,
		Contributors: []models.PolicyRef{{Name: "np1", Namespace: "ns-a"}},
	}
}

func policies(rules ...models.NodeRule) map[string]models.NodeInfo {
	return map[string]models.NodeInfo{"k8s": {Rules: rules}}
}

// Non-ambient workload: ValidateExternalRules has no opinion.
func TestValidateExternalRules_NotInMesh(t *testing.T) {
	membership := &models.MeshMembership{InMesh: false}
	errors := ValidateExternalRules(membership, policies(allowRule(models.DirectionEgress, "dst1", 8080)))
	if len(errors) != 0 {
		t.Fatalf("expected no errors for non-mesh workload, got %v", errors)
	}
}

// Egress rule restricting to a non-HBONE port → flagged with the ztunnel port.
func TestValidateExternalRules_EgressMissingHBONE(t *testing.T) {
	errors := ValidateExternalRules(ambientMembership(), policies(allowRule(models.DirectionEgress, "dst1", 8080)))
	if len(errors) == 0 {
		t.Fatal("expected an egress error, got none")
	}
	if !hasIssue(errors, "egress") || !hasIssue(errors, "15008") {
		t.Fatalf("error should mention egress + 15008: %v", errors)
	}
}

// Ingress rule restricting to a non-HBONE port → flagged on the ingress side.
func TestValidateExternalRules_IngressMissingHBONE(t *testing.T) {
	errors := ValidateExternalRules(ambientMembership(), policies(allowRule(models.DirectionIngress, "dst1", 8080)))
	if !hasIssue(errors, "ingress") || !hasIssue(errors, "15008") {
		t.Fatalf("error should mention ingress + 15008: %v", errors)
	}
}

// Port 0 = all ports open → ztunnel HBONE port is covered, no error.
func TestValidateExternalRules_PortZeroAllowsAll(t *testing.T) {
	errors := ValidateExternalRules(ambientMembership(), policies(allowRule(models.DirectionEgress, "dst1", 0)))
	if len(errors) != 0 {
		t.Fatalf("port 0 opens all ports; expected no error, got %v", errors)
	}
}

// A rule explicitly allowing the HBONE port → no error.
func TestValidateExternalRules_HBONEPortAllowed(t *testing.T) {
	errors := ValidateExternalRules(ambientMembership(), policies(allowRule(models.DirectionEgress, "dst1", ZtunnelHBONEPort)))
	if len(errors) != 0 {
		t.Fatalf("HBONE port allowed; expected no error, got %v", errors)
	}
}
