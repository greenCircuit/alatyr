package istio

import (
	"testing"

	"alatyr/internal/models"
)

// allowAllPeerHBONE builds a NodeRule shaped like a k8s NetworkPolicy stanza
// with empty peer selector + ports=[15008] — the k8spolicy builder emits
// these with empty DstID/SrcID and Coverage=AllowAll. Represents the stanza
// `- ports: [{15008 TCP}]` (no `to:` / `from:`).
func allowAllPeerHBONE(direction models.Direction) models.NodeRule {
	return models.NodeRule{
		Direction:   direction,
		Action:      models.ActionAllow,
		Ports:       []models.Port{{Port: ZtunnelHBONEPort, Protocol: "TCP"}},
		Contributor: models.PolicyRef{Source: "k8s", Name: "payments-api-netpol", Namespace: "payments"},
	}
}

// allowAllPeerNoPorts is `- {}` on the stanza side — empty peer + empty ports
// = admit everything. Represents a wide-open stanza. Also globally opens
// HBONE by k8s semantics (empty ports list = every port).
func allowAllPeerNoPorts(direction models.Direction) models.NodeRule {
	return models.NodeRule{
		Direction:   direction,
		Action:      models.ActionAllow,
		Contributor: models.PolicyRef{Source: "k8s", Name: "wide-open", Namespace: "payments"},
	}
}

// User-reported footgun. The payments-api NetworkPolicy has one egress stanza
// opening 15008 to any peer, then a second stanza restricting 5432 to
// ledger-db. Current traffic to ledger-db:15008 is allowed by k8s stanza-OR
// semantics, so the second stanza is NOT a mesh transport problem — must not
// be flagged.
func TestValidateExternalRules_GlobalHBONEEgressStanzaCoversRestrictedPeer(t *testing.T) {
	globalHBONE := allowAllPeerHBONE(models.DirectionEgress)
	restrictedToLedger := allowRule(models.DirectionEgress, "dst1", []int{5432}) // ledger-db-shaped

	errors := ValidateExternalRules(testWorkload(), meshCache(), ambientMembership(),
		policies(globalHBONE, restrictedToLedger),
	)

	if len(errors) != 0 {
		t.Fatalf("global 15008 stanza should cover the restricted stanza; got %d issues: %+v", len(errors), errors)
	}
}

// Same shape on the ingress side: one stanza opens 15008 from anyone, another
// restricts 8080 to specific pods. Must not be flagged.
func TestValidateExternalRules_GlobalHBONEIngressStanzaCoversRestrictedPeer(t *testing.T) {
	globalHBONE := allowAllPeerHBONE(models.DirectionIngress)
	restricted := allowRule(models.DirectionIngress, "dst1", []int{8080})

	errors := ValidateExternalRules(testWorkload(), meshCache(), ambientMembership(),
		policies(globalHBONE, restricted),
	)

	if len(errors) != 0 {
		t.Fatalf("global 15008 ingress stanza should cover the restricted stanza; got %d: %+v", len(errors), errors)
	}
}

// Global open is per-direction: an EGRESS stanza opening 15008 does NOT
// cover an INGRESS restriction. Guards against smearing the flag across
// directions.
func TestValidateExternalRules_GlobalHBONEDoesNotCrossDirections(t *testing.T) {
	globalEgressHBONE := allowAllPeerHBONE(models.DirectionEgress)
	restrictedIngress := allowRule(models.DirectionIngress, "dst1", []int{8080})

	errors := ValidateExternalRules(testWorkload(), meshCache(), ambientMembership(),
		policies(globalEgressHBONE, restrictedIngress),
	)

	if len(errors) != 1 {
		t.Fatalf("ingress restriction must still flag when only egress is globally open; got %d: %+v", len(errors), errors)
	}
	if !hasIssue(issueMessages(errors), "ingress") {
		t.Fatalf("issue should be on ingress side: %+v", errors)
	}
}

// Empty ports stanza (`- {}`) = allow-all-peer + allow-all-ports. Every port
// including HBONE is covered, so a sibling restricted stanza must not flag.
func TestValidateExternalRules_GlobalAllPortsStanzaCovers(t *testing.T) {
	wideOpen := allowAllPeerNoPorts(models.DirectionEgress)
	restricted := allowRule(models.DirectionEgress, "dst1", []int{5432})

	errors := ValidateExternalRules(testWorkload(), meshCache(), ambientMembership(),
		policies(wideOpen, restricted),
	)

	if len(errors) != 0 {
		t.Fatalf("wide-open stanza must cover HBONE; got %d issues: %+v", len(errors), errors)
	}
}

// Global open is per-engine: a k8s stanza opening HBONE globally must NOT
// silence an istio rule that restricts ports without HBONE, because istio
// rules AND with k8s (not union). Guards against cross-engine leakage.
func TestValidateExternalRules_GlobalOpenIsPerEngine(t *testing.T) {
	nodePolicies := map[string]models.NodeInfo{
		"k8s": {Rules: []models.NodeRule{
			allowAllPeerHBONE(models.DirectionIngress),
		}},
		"istio": {Rules: []models.NodeRule{
			allowRuleWithPolicy(models.DirectionIngress, "dst1", []int{8080}, "istio", "ap-strict", "payments"),
		}},
	}

	errors := ValidateExternalRules(testWorkload(), meshCache(), ambientMembership(), nodePolicies)

	if len(errors) != 1 {
		t.Fatalf("istio restriction must still flag despite k8s global open (per-engine); got %d: %+v", len(errors), errors)
	}
	if !hasIssue(culpritNames(errors), "ap-strict") {
		t.Fatalf("flagged issue should attribute to istio policy: %+v", errors)
	}
}

// The stanza that opens HBONE globally must itself never be flagged. Guards
// against the detector confusing the source of the coverage with a target.
func TestValidateExternalRules_GlobalOpenStanzaNotFlagged(t *testing.T) {
	errors := ValidateExternalRules(testWorkload(), meshCache(), ambientMembership(),
		policies(allowAllPeerHBONE(models.DirectionEgress)),
	)
	if len(errors) != 0 {
		t.Fatalf("allow-all + 15008 stanza is the fix, not the bug; got %+v", errors)
	}
}
