package istio

import (
	"testing"

	"alatyr/internal/models"
)

// testDstNs is the ambient-enrolled namespace every test dst lives in, so the
// dst-in-mesh gate resolves them to mesh peers (HBONE applies → rules flaggable).
const testDstNs = "ns-dst"

func ambientMembership() *models.MeshMembership {
	return &models.MeshMembership{InMesh: true, Provider: SourceName, Mode: "ambient"}
}

// testWorkload is the ambient src workload the ValidateExternalRules suite runs
// against. Node pointer stamped onto each emitted Issue so assertions can
// still reach the source identity if a test starts checking Node.
func testWorkload() models.WorkloadNode {
	return models.WorkloadNode{ID: "src-workload", Namespace: "ns-src", Type: models.NodeTypeDeployment}
}

// issueMessages flattens []Issue to []string so the existing hasIssue helper
// (substring match over string slice) keeps working after the refactor.
func issueMessages(issues []models.Issue) []string {
	out := make([]string, 0, len(issues))
	for _, i := range issues {
		out = append(out, i.Message)
	}
	return out
}

// culpritNames flattens both-direction culprit refs to "source/name/namespace"
// strings — attribution asserts go through culprits now, not the message text.
func culpritNames(issues []models.Issue) []string {
	var out []string
	for _, issue := range issues {
		for _, ref := range issue.IngressCulprits {
			out = append(out, ref.Source+"/"+ref.Name+"/"+ref.Namespace)
		}
		for _, ref := range issue.EgressCulprits {
			out = append(out, ref.Source+"/"+ref.Name+"/"+ref.Namespace)
		}
	}
	return out
}

// meshCache returns a cache whose dsts (dst1, dst2) are ambient-enrolled via the
// namespace label, so dstInAmbientMesh reports them in-mesh. Shared by the suite.
func meshCache() *models.Cache {
	ambientLabels := map[string]string{AmbientEnrollmentKey: AmbientEnrollmentValue}
	return &models.Cache{
		NsIndex: map[string]models.NSIndex{
			testDstNs: {
				NSNode: &models.WorkloadNode{
					ID:        testDstNs,
					Namespace: testDstNs,
					Type:      models.NodeTypeNamespace,
					Labels:    ambientLabels,
				},
				Workloads: []models.WorkloadNode{
					{ID: "dst1", Namespace: testDstNs},
					{ID: "dst2", Namespace: testDstNs},
					// in-cluster but opted out — workload label beats the ns label.
					{ID: "dst-plain", Namespace: testDstNs, Labels: map[string]string{AmbientEnrollmentKey: "none"}},
				},
			},
			// System ns: components speak HBONE without the enrollment label.
			RootNamespace: {
				NSNode: &models.WorkloadNode{ID: RootNamespace, Namespace: RootNamespace, Type: models.NodeTypeNamespace},
				Workloads: []models.WorkloadNode{
					{ID: "dst-root", Namespace: RootNamespace},
				},
			},
			// The protected workload's own ambient ns. A regressed gate that
			// resolves the peer via DstID on ingress rules lands here (always
			// in-mesh) and false-flags — keeps the P1#3 swap tests sensitive.
			"ns-src": {
				NSNode: &models.WorkloadNode{
					ID:        "ns-src",
					Namespace: "ns-src",
					Type:      models.NodeTypeNamespace,
					Labels:    ambientLabels,
				},
				Workloads: []models.WorkloadNode{
					{ID: "src-workload", Namespace: "ns-src"},
				},
			},
		},
	}
}

// allowRule builds an ALLOW NodeRule with one contributing policy. peerID is
// the far end of the rule: DstID for egress, SrcID for ingress — k8s ingress
// rules swap, DstID holds the protected workload itself (see
// k8spolicy/buildRules.go), and these fixtures mirror that prod shape.
func allowRule(direction models.Direction, peerID string, ports []int) models.NodeRule {
	rulePorts := make([]models.Port, 0, len(ports))
	for _, port := range ports {
		rulePorts = append(rulePorts, models.Port{Port: port, Protocol: "TCP"})
	}
	rule := models.NodeRule{
		Direction:   direction,
		Action:      models.ActionAllow,
		Ports:       rulePorts,
		Contributor: models.PolicyRef{Name: "np1", Namespace: "ns-a"},
	}
	if direction == models.DirectionIngress {
		rule.SrcID, rule.SrcNamespace = peerID, testDstNs
		rule.DstID, rule.DstNamespace = "src-workload", "ns-src"
	} else {
		rule.DstID, rule.DstNamespace = peerID, testDstNs
	}
	return rule
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
	errors := ValidateExternalRules(testWorkload(), meshCache(), membership, policies(allowRule(models.DirectionEgress, "dst1", []int{8080})))
	if len(errors) != 0 {
		t.Fatalf("expected no errors for non-mesh workload, got %v", errors)
	}
}

// Egress rule restricting to a non-HBONE port → flagged with the ztunnel port.
func TestValidateExternalRules_EgressMissingHBONE(t *testing.T) {
	errors := ValidateExternalRules(testWorkload(), meshCache(), ambientMembership(), policies(allowRule(models.DirectionEgress, "dst1", []int{8080})))
	if len(errors) == 0 {
		t.Fatal("expected an egress error, got none")
	}
	if !hasIssue(issueMessages(errors), "egress") || !hasIssue(issueMessages(errors), "15008") {
		t.Fatalf("error should mention egress + 15008: %v", errors)
	}
}

// Ingress rule restricting to a non-HBONE port → flagged on the ingress side.
func TestValidateExternalRules_IngressMissingHBONE(t *testing.T) {
	errors := ValidateExternalRules(testWorkload(), meshCache(), ambientMembership(), policies(allowRule(models.DirectionIngress, "dst1", []int{8080})))
	if !hasIssue(issueMessages(errors), "ingress") || !hasIssue(issueMessages(errors), "15008") {
		t.Fatalf("error should mention ingress + 15008: %v", errors)
	}
}

// Port 0 = all ports open → ztunnel HBONE port is covered, no error.
func TestValidateExternalRules_PortZeroAllowsAll(t *testing.T) {
	errors := ValidateExternalRules(testWorkload(), meshCache(), ambientMembership(), policies(allowRule(models.DirectionEgress, "dst1", []int{})))
	if len(errors) != 0 {
		t.Fatalf("port 0 opens all ports; expected no error, got %v", errors)
	}
}

// A rule explicitly allowing the HBONE port → no error.
func TestValidateExternalRules_HBONEPortAllowed(t *testing.T) {
	errors := ValidateExternalRules(testWorkload(), meshCache(), ambientMembership(), policies(allowRule(models.DirectionEgress, "dst1", []int{ZtunnelHBONEPort})))
	if len(errors) != 0 {
		t.Fatalf("HBONE port allowed; expected no error, got %v", errors)
	}
}

// Ingress rule listing both 8080 and HBONE → no error (HBONE covered within the rule).
func TestValidateExternalRules_MultiplePortsIngress(t *testing.T) {
	errors := ValidateExternalRules(testWorkload(), meshCache(), ambientMembership(),
		policies(allowRule(models.DirectionIngress, "dst1", []int{8080, ZtunnelHBONEPort})),
	)
	if len(errors) != 0 {
		t.Fatalf("HBONE port present in port list, got %v", errors)
	}
}

// Egress rule listing both 8080 and HBONE → no error.
func TestValidateExternalRules_MultiplePortsEgress(t *testing.T) {
	errors := ValidateExternalRules(testWorkload(), meshCache(), ambientMembership(),
		policies(allowRule(models.DirectionEgress, "dst1", []int{8080, ZtunnelHBONEPort})),
	)
	if len(errors) != 0 {
		t.Fatalf("HBONE port present in port list, got %v", errors)
	}
}

// Same policy, both directions, HBONE present on each → no error.
func TestValidateExternalRules_MultiplePortsMixedPass1(t *testing.T) {
	errors := ValidateExternalRules(testWorkload(), meshCache(), ambientMembership(),
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
	errors := ValidateExternalRules(testWorkload(), meshCache(), ambientMembership(),
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
	errors := ValidateExternalRules(testWorkload(), meshCache(), ambientMembership(),
		policies(allowRule(models.DirectionEgress, "dst1", []int{8080, ZtunnelHBONEPort, 8443})),
	)
	if len(errors) != 0 {
		t.Fatalf("HBONE port present in port list, got %v", errors)
	}
}

// Single ingress rule listing many ports including HBONE → no error.
func TestValidateExternalRules_MultipleIngress(t *testing.T) {
	errors := ValidateExternalRules(testWorkload(), meshCache(), ambientMembership(),
		policies(allowRule(models.DirectionIngress, "dst1", []int{8080, ZtunnelHBONEPort, 8443})),
	)
	if len(errors) != 0 {
		t.Fatalf("HBONE port present in port list, got %v", errors)
	}
}

// Single ingress rule, no HBONE in port list → exactly 1 error.
func TestValidateExternalRules_DedupErrorsIngress(t *testing.T) {
	errors := ValidateExternalRules(testWorkload(), meshCache(), ambientMembership(),
		policies(allowRule(models.DirectionIngress, "dst1", []int{8080, 8443})),
	)
	if len(errors) != 1 {
		t.Fatalf("expected 1 error, got %v", errors)
	}
}

// Single egress rule, no HBONE in port list → exactly 1 error.
func TestValidateExternalRules_DedupErrorsEgress(t *testing.T) {
	errors := ValidateExternalRules(testWorkload(), meshCache(), ambientMembership(),
		policies(allowRule(models.DirectionEgress, "dst1", []int{8080, 8443})),
	)
	if len(errors) != 1 {
		t.Fatalf("expected 1 error, got %v", errors)
	}
}

// Two ingress rules to different dsts, both missing HBONE → 2 errors.
// Guards against bucket collapse across destinations.
func TestValidateExternalRules_MultipleDestinations(t *testing.T) {
	errors := ValidateExternalRules(testWorkload(), meshCache(), ambientMembership(),
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
	errors := ValidateExternalRules(testWorkload(), meshCache(), ambientMembership(),
		policies(
			allowRuleWithPolicy(models.DirectionIngress, "dst1", []int{8080}, "k8s", "np-a", "ns-a"),
			allowRuleWithPolicy(models.DirectionIngress, "dst1", []int{8080}, "k8s", "np-b", "ns-a"),
		),
	)
	if len(errors) != 2 {
		t.Fatalf("expected 2 errors (one per policy), got %v", errors)
	}
	if !hasIssue(culpritNames(errors), "np-a") || !hasIssue(culpritNames(errors), "np-b") {
		t.Fatalf("each policy name should appear in culprits: %v", errors)
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
	errors := ValidateExternalRules(testWorkload(), meshCache(), ambientMembership(), nodePolicies)
	if len(errors) != 2 {
		t.Fatalf("expected 2 errors (one per engine), got %v", errors)
	}
	if !hasIssue(culpritNames(errors), "np-k8s") || !hasIssue(culpritNames(errors), "ap-istio") {
		t.Fatalf("each engine's policy should appear in culprits: %v", errors)
	}
}

// Culprit ref must carry the contributor's source, name, and namespace verbatim,
// on the side matching the rule's direction; message only keeps direction + HBONE
// port. Guards against field swaps and wrong-side culprit stamping.
func TestValidateExternalRules_ErrorAttribution(t *testing.T) {
	errors := ValidateExternalRules(testWorkload(), meshCache(), ambientMembership(),
		policies(allowRuleWithPolicy(models.DirectionEgress, "dst1", []int{8080}, "k8s", "np-attr", "ns-attr")),
	)
	if len(errors) != 1 {
		t.Fatalf("expected 1 error, got %v", errors)
	}
	culprits := errors[0].EgressCulprits
	if len(culprits) != 1 || culprits[0].Source != "k8s" || culprits[0].Name != "np-attr" || culprits[0].Namespace != "ns-attr" {
		t.Fatalf("egress culprit should carry contributor verbatim: %+v", errors[0])
	}
	if len(errors[0].IngressCulprits) != 0 {
		t.Fatalf("egress rule must not stamp ingress culprits: %+v", errors[0])
	}
	for _, want := range []string{"egress", "15008"} {
		if !hasIssue([]string{errors[0].Message}, want) {
			t.Fatalf("message missing %q: %s", want, errors[0].Message)
		}
	}
}

// Peers that never tunnel HBONE must not be flagged for missing 15008,
// regardless of direction or restricted ports:
//   - external CIDR (0.0.0.0/0) — not a workload, leaves the mesh
//   - in-cluster workload opted out of ambient (dst-plain)
//   - workload in an unloaded/uncached namespace
//
// The ingress cases are the P1#3 swap regression: peer sits in SrcID with a
// non-ambient identity while DstID is the ambient workload itself — a gate
// resolving DstID for ingress false-flags every one of these.
func TestValidateExternalRules_OutOfMeshPeerNotFlagged(t *testing.T) {
	cases := []struct {
		name      string
		direction models.Direction
		peerID    string
		peerNs    string
	}{
		{"external CIDR egress", models.DirectionEgress, "0.0.0.0/0", testDstNs},
		{"external CIDR ingress", models.DirectionIngress, "0.0.0.0/0", testDstNs},
		{"non-ambient peer egress", models.DirectionEgress, "dst-plain", testDstNs},
		{"non-ambient peer ingress", models.DirectionIngress, "dst-plain", testDstNs},
		{"uncached ns peer", models.DirectionEgress, "dst1", "ns-not-loaded"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rule := allowRule(tc.direction, tc.peerID, []int{8080})
			if tc.direction == models.DirectionIngress {
				rule.SrcNamespace = tc.peerNs
			} else {
				rule.DstNamespace = tc.peerNs
			}
			errors := ValidateExternalRules(testWorkload(), meshCache(), ambientMembership(), policies(rule))
			if len(errors) != 0 {
				t.Fatalf("out-of-mesh peer must not be flagged, got %v", errors)
			}
		})
	}
}

// Root/ingress-ns dsts speak HBONE without the enrollment label, so a rule to
// them missing 15008 must still be flagged. Guards the gate's namespace rule.
func TestValidateExternalRules_SystemNsDstFlagged(t *testing.T) {
	rule := allowRule(models.DirectionEgress, "dst-root", []int{8080})
	rule.DstNamespace = RootNamespace
	errors := ValidateExternalRules(testWorkload(), meshCache(), ambientMembership(), policies(rule))
	if !hasIssue(issueMessages(errors), "egress") || !hasIssue(issueMessages(errors), "15008") {
		t.Fatalf("system-ns dst missing HBONE should be flagged: %v", errors)
	}
}

// Mixed batch: only the in-mesh dst missing HBONE is flagged; external and
// non-ambient dsts in the same engine are skipped. Guards the gate against
// over- or under-counting when dsts of both kinds share one rule set.
func TestValidateExternalRules_OnlyInMeshDstFlagged(t *testing.T) {
	external := allowRule(models.DirectionEgress, "0.0.0.0/0", []int{443})
	plain := allowRule(models.DirectionEgress, "dst-plain", []int{8080})
	errors := ValidateExternalRules(testWorkload(), meshCache(), ambientMembership(),
		policies(
			allowRule(models.DirectionEgress, "dst1", []int{8080}), // in mesh, no HBONE → flagged
			external,
			plain,
		),
	)
	if len(errors) != 1 {
		t.Fatalf("expected exactly 1 error (in-mesh dst only), got %v", errors)
	}
}
