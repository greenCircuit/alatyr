package k8spolicy

import (
	"testing"

	"graph/internal/models"

	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

var (
	nodeFrontend = models.WorkloadNode{ID: "frontend-uid", Labels: map[string]string{"app": "frontend"}, Namespace: "default"}
	nodeBackend  = models.WorkloadNode{ID: "backend-uid", Labels: map[string]string{"app": "backend"}, Namespace: "default"}
	nodeNS       = models.WorkloadNode{ID: "ns-default", Type: models.NodeTypeNamespace, Namespace: "default", Labels: map[string]string{"kubernetes.io/metadata.name": "default"}}
)

func buildTestIndex(nodes []models.WorkloadNode) map[string]map[string][]*models.WorkloadNode {
	return map[string]map[string][]*models.WorkloadNode{
		"default": buildWorkloadIndex(nodes),
	}
}

func buildMultiNSIndex(nodesByNS map[string][]models.WorkloadNode) map[string]map[string][]*models.WorkloadNode {
	result := map[string]map[string][]*models.WorkloadNode{}
	for ns, nodes := range nodesByNS {
		result[ns] = buildWorkloadIndex(nodes)
	}
	return result
}

func defaultTestNodes() []models.WorkloadNode {
	return []models.WorkloadNode{nodeFrontend, nodeBackend}
}

func defaultTestNodesWithNS() []models.WorkloadNode {
	return []models.WorkloadNode{nodeFrontend, nodeBackend, nodeNS}
}

func policyWithEgress(srcLabels, dstLabels map[string]string) networkingv1.NetworkPolicy {
	return networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{Namespace: "default"},
		Spec: networkingv1.NetworkPolicySpec{
			PodSelector: metav1.LabelSelector{MatchLabels: srcLabels},
			Egress: []networkingv1.NetworkPolicyEgressRule{
				{To: []networkingv1.NetworkPolicyPeer{
					{PodSelector: &metav1.LabelSelector{MatchLabels: dstLabels}},
				}},
			},
		},
	}
}

func policyWithIngress(dstLabels, srcLabels map[string]string) networkingv1.NetworkPolicy {
	return networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{Namespace: "default"},
		Spec: networkingv1.NetworkPolicySpec{
			PodSelector: metav1.LabelSelector{MatchLabels: dstLabels},
			Ingress: []networkingv1.NetworkPolicyIngressRule{
				{From: []networkingv1.NetworkPolicyPeer{
					{PodSelector: &metav1.LabelSelector{MatchLabels: srcLabels}},
				}},
			},
		},
	}
}

// BuildAllowTuples

func TestBuildAllowTuples_EgressDirection(t *testing.T) {
	nodes := defaultTestNodes()
	networkPolicy := policyWithEgress(
		map[string]string{"app": "frontend"},
		map[string]string{"app": "backend"},
	)
	tuples := BuildAllowTuples(buildTestIndex(nodes), []networkingv1.NetworkPolicy{networkPolicy})

	if len(tuples) != 1 {
		t.Fatalf("expected 1 tuple, got %d", len(tuples))
	}
	tuple := tuples[0]
	if tuple.SrcID != nodeFrontend.ID {
		t.Errorf("egress source: want %s, got %s", nodeFrontend.ID, tuple.SrcID)
	}
	if tuple.DstID != nodeBackend.ID {
		t.Errorf("egress target: want %s, got %s", nodeBackend.ID, tuple.DstID)
	}
	if tuple.Direction != models.DirectionEgress {
		t.Errorf("expected egress direction, got %s", tuple.Direction)
	}
}

func TestBuildAllowTuples_IngressDirection(t *testing.T) {
	nodes := defaultTestNodes()
	networkPolicy := policyWithIngress(
		map[string]string{"app": "backend"},
		map[string]string{"app": "frontend"},
	)
	tuples := BuildAllowTuples(buildTestIndex(nodes), []networkingv1.NetworkPolicy{networkPolicy})

	if len(tuples) != 1 {
		t.Fatalf("expected 1 tuple, got %d", len(tuples))
	}
	tuple := tuples[0]
	if tuple.SrcID != nodeFrontend.ID {
		t.Errorf("ingress source: want %s (sender), got %s", nodeFrontend.ID, tuple.SrcID)
	}
	if tuple.DstID != nodeBackend.ID {
		t.Errorf("ingress target: want %s (receiver), got %s", nodeBackend.ID, tuple.DstID)
	}
	if tuple.Direction != models.DirectionIngress {
		t.Errorf("expected ingress direction, got %s", tuple.Direction)
	}
}

func TestBuildAllowTuples_NoPolicies(t *testing.T) {
	tuples := BuildAllowTuples(buildTestIndex(defaultTestNodes()), nil)
	if len(tuples) != 0 {
		t.Errorf("expected no tuples, got %d", len(tuples))
	}
}

func TestBuildAllowTuples_NoMatchingNodes(t *testing.T) {
	nodes := defaultTestNodes()
	networkPolicy := policyWithEgress(
		map[string]string{"app": "unknown"},
		map[string]string{"app": "backend"},
	)
	tuples := BuildAllowTuples(buildTestIndex(nodes), []networkingv1.NetworkPolicy{networkPolicy})
	if len(tuples) != 0 {
		t.Errorf("expected no tuples when source selector matches nothing, got %d", len(tuples))
	}
}

// getTargetEgressTuples

func TestGetTargetEgressTuples_Match(t *testing.T) {
	nodes := defaultTestNodes()
	networkPolicy := policyWithEgress(
		map[string]string{"app": "frontend"},
		map[string]string{"app": "backend"},
	)
	tuples := getTargetEgressTuples(networkPolicy, buildTestIndex(nodes))
	if len(tuples) != 1 {
		t.Fatalf("expected 1 egress tuple, got %d", len(tuples))
	}
	if tuples[0].DstID != nodeBackend.ID {
		t.Errorf("want target %s, got %s", nodeBackend.ID, tuples[0].DstID)
	}
	if tuples[0].Direction != models.DirectionEgress {
		t.Errorf("expected egress direction, got %s", tuples[0].Direction)
	}
}

func TestGetTargetEgressTuples_NilPodSelector(t *testing.T) {
	networkPolicy := networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{Namespace: "default"},
		Spec: networkingv1.NetworkPolicySpec{
			Egress: []networkingv1.NetworkPolicyEgressRule{
				{To: []networkingv1.NetworkPolicyPeer{{PodSelector: nil}}},
			},
		},
	}
	tuples := getTargetEgressTuples(networkPolicy, buildTestIndex(defaultTestNodes()))
	if len(tuples) != 0 {
		t.Errorf("nil PodSelector should produce no tuples, got %d", len(tuples))
	}
}

func TestGetTargetEgressTuples_NoRules(t *testing.T) {
	networkPolicy := networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{Namespace: "default"},
	}
	tuples := getTargetEgressTuples(networkPolicy, buildTestIndex(defaultTestNodes()))
	if len(tuples) != 0 {
		t.Errorf("expected no tuples, got %d", len(tuples))
	}
}

// getTargetIngressTuples

func TestGetTargetIngressTuples_Match(t *testing.T) {
	nodes := defaultTestNodes()
	networkPolicy := policyWithIngress(
		map[string]string{"app": "backend"},
		map[string]string{"app": "frontend"},
	)
	tuples := getTargetIngressTuples(networkPolicy, buildTestIndex(nodes))
	if len(tuples) != 1 {
		t.Fatalf("expected 1 ingress tuple, got %d", len(tuples))
	}
	if tuples[0].DstID != nodeFrontend.ID {
		t.Errorf("want target %s, got %s", nodeFrontend.ID, tuples[0].DstID)
	}
	if tuples[0].Direction != models.DirectionIngress {
		t.Errorf("expected ingress direction, got %s", tuples[0].Direction)
	}
}

func TestGetTargetIngressTuples_NilPodSelector(t *testing.T) {
	networkPolicy := networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{Namespace: "default"},
		Spec: networkingv1.NetworkPolicySpec{
			Ingress: []networkingv1.NetworkPolicyIngressRule{
				{From: []networkingv1.NetworkPolicyPeer{{PodSelector: nil}}},
			},
		},
	}
	tuples := getTargetIngressTuples(networkPolicy, buildTestIndex(defaultTestNodes()))
	if len(tuples) != 0 {
		t.Errorf("nil PodSelector should produce no tuples, got %d", len(tuples))
	}
}

func TestGetTargetIngressTuples_NoRules(t *testing.T) {
	networkPolicy := networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{Namespace: "default"},
	}
	tuples := getTargetIngressTuples(networkPolicy, buildTestIndex(defaultTestNodes()))
	if len(tuples) != 0 {
		t.Errorf("expected no tuples, got %d", len(tuples))
	}
}

// generateTuples — namespace-only selector

func TestGenerateTuples_NSOnly_CatchAll(t *testing.T) {
	peer := networkingv1.NetworkPolicyPeer{
		NamespaceSelector: &metav1.LabelSelector{}, // empty = catch-all → skip
	}
	tuples := generateTuples("pol", "default", 0, models.DirectionEgress, peer, nil, buildTestIndex(defaultTestNodesWithNS()))
	if tuples != nil {
		t.Errorf("catch-all namespace selector should return nil, got %v", tuples)
	}
}

func TestGenerateTuples_NSOnly_Match(t *testing.T) {
	peer := networkingv1.NetworkPolicyPeer{
		NamespaceSelector: &metav1.LabelSelector{
			MatchLabels: map[string]string{"kubernetes.io/metadata.name": "default"},
		},
	}
	tuples := generateTuples("pol", "src", 0, models.DirectionEgress, peer, nil, buildTestIndex(defaultTestNodesWithNS()))
	if len(tuples) != 1 {
		t.Fatalf("expected 1 tuple, got %d", len(tuples))
	}
	if tuples[0].DstID != nodeNS.ID {
		t.Errorf("want target %s (NS node), got %s", nodeNS.ID, tuples[0].DstID)
	}
}

func TestGenerateTuples_NSOnly_NoMatch(t *testing.T) {
	peer := networkingv1.NetworkPolicyPeer{
		NamespaceSelector: &metav1.LabelSelector{
			MatchLabels: map[string]string{"kubernetes.io/metadata.name": "other"},
		},
	}
	tuples := generateTuples("pol", "src", 0, models.DirectionEgress, peer, nil, buildTestIndex(defaultTestNodesWithNS()))
	if len(tuples) != 0 {
		t.Errorf("non-matching namespace selector should produce no tuples, got %d", len(tuples))
	}
}

// generateTuples — pod-only selector

func TestGenerateTuples_PodOnly_CatchAll(t *testing.T) {
	peer := networkingv1.NetworkPolicyPeer{
		PodSelector: &metav1.LabelSelector{}, // empty = catch-all → collapse to NS node
	}
	tuples := generateTuples("pol", "default", 0, models.DirectionEgress, peer, nil, buildTestIndex(defaultTestNodesWithNS()))
	if len(tuples) != 1 {
		t.Fatalf("expected 1 tuple to NS node, got %d", len(tuples))
	}
	if tuples[0].DstID != nodeNS.ID {
		t.Errorf("want NS node target %s, got %s", nodeNS.ID, tuples[0].DstID)
	}
}

// generateTuples — pod + namespace selector

func TestGenerateTuples_BothSelectors_SpecificNSSpecificPod(t *testing.T) {
	otherNSNode := models.WorkloadNode{ID: "ns-other", Type: models.NodeTypeNamespace, Namespace: "other", Labels: map[string]string{"kubernetes.io/metadata.name": "other"}}
	otherPod := models.WorkloadNode{ID: "other-backend", Labels: map[string]string{"app": "backend"}, Namespace: "other"}
	index := buildMultiNSIndex(map[string][]models.WorkloadNode{
		"default": defaultTestNodesWithNS(),
		"other":   {otherPod, otherNSNode},
	})
	peer := networkingv1.NetworkPolicyPeer{
		PodSelector:       &metav1.LabelSelector{MatchLabels: map[string]string{"app": "backend"}},
		NamespaceSelector: &metav1.LabelSelector{MatchLabels: map[string]string{"kubernetes.io/metadata.name": "other"}},
	}
	tuples := generateTuples("pol", "default", 0, models.DirectionEgress, peer, nil, index)
	if len(tuples) != 1 {
		t.Fatalf("expected 1 tuple, got %d", len(tuples))
	}
	if tuples[0].DstID != otherPod.ID {
		t.Errorf("want target %s, got %s", otherPod.ID, tuples[0].DstID)
	}
}

func TestGenerateTuples_BothSelectors_CatchAllNSSpecificPod(t *testing.T) {
	otherNSNode := models.WorkloadNode{ID: "ns-other", Type: models.NodeTypeNamespace, Namespace: "other", Labels: map[string]string{"kubernetes.io/metadata.name": "other"}}
	otherPod := models.WorkloadNode{ID: "other-backend", Labels: map[string]string{"app": "backend"}, Namespace: "other"}
	index := buildMultiNSIndex(map[string][]models.WorkloadNode{
		"default": defaultTestNodesWithNS(),
		"other":   {otherPod, otherNSNode},
	})
	peer := networkingv1.NetworkPolicyPeer{
		PodSelector:       &metav1.LabelSelector{MatchLabels: map[string]string{"app": "backend"}},
		NamespaceSelector: &metav1.LabelSelector{}, // catch-all NS
	}
	tuples := generateTuples("pol", "default", 0, models.DirectionEgress, peer, nil, index)
	if len(tuples) != 2 {
		t.Fatalf("expected 1 tuple per NS with matching pod (2 total), got %d", len(tuples))
	}
}

func TestGenerateTuples_BothSelectors_SpecificNSCatchAllPod(t *testing.T) {
	otherNSNode := models.WorkloadNode{ID: "ns-other", Type: models.NodeTypeNamespace, Namespace: "other", Labels: map[string]string{"kubernetes.io/metadata.name": "other"}}
	otherPod := models.WorkloadNode{ID: "other-backend", Labels: map[string]string{"app": "backend"}, Namespace: "other"}
	index := buildMultiNSIndex(map[string][]models.WorkloadNode{
		"default": defaultTestNodesWithNS(),
		"other":   {otherPod, otherNSNode},
	})
	peer := networkingv1.NetworkPolicyPeer{
		PodSelector:       &metav1.LabelSelector{}, // catch-all pod → use NS node
		NamespaceSelector: &metav1.LabelSelector{MatchLabels: map[string]string{"kubernetes.io/metadata.name": "other"}},
	}
	tuples := generateTuples("pol", "default", 0, models.DirectionEgress, peer, nil, index)
	if len(tuples) != 1 {
		t.Fatalf("expected 1 NS-level tuple, got %d", len(tuples))
	}
	if tuples[0].DstID != otherNSNode.ID {
		t.Errorf("want NS node target %s, got %s", otherNSNode.ID, tuples[0].DstID)
	}
}

// generateTuples — IP block

func TestGenerateTuples_IPBlock(t *testing.T) {
	peer := networkingv1.NetworkPolicyPeer{
		IPBlock: &networkingv1.IPBlock{CIDR: "10.0.0.0/8"},
	}
	tuples := generateTuples("pol", "default", 0, models.DirectionEgress, peer, nil, buildTestIndex(defaultTestNodes()))
	if len(tuples) != 1 {
		t.Fatalf("expected 1 tuple, got %d", len(tuples))
	}
	if tuples[0].DstID != "10.0.0.0/8" {
		t.Errorf("want CIDR as target, got %s", tuples[0].DstID)
	}
}

// ── port conversion ─────────────────────────────────────────────────────────

func intPort(number int32) *intstr.IntOrString {
	value := intstr.FromInt(int(number))
	return &value
}

func namedPort(name string) *intstr.IntOrString {
	value := intstr.FromString(name)
	return &value
}

func proto(protocol corev1.Protocol) *corev1.Protocol { return &protocol }

func TestConvertPorts_EmptyReturnsNil(t *testing.T) {
	if got := convertPorts(nil); got != nil {
		t.Errorf("want nil for empty input, got %v", got)
	}
	if got := convertPorts([]networkingv1.NetworkPolicyPort{}); got != nil {
		t.Errorf("want nil for empty slice, got %v", got)
	}
}

func TestConvertPorts_NumericTCPDefault(t *testing.T) {
	in := []networkingv1.NetworkPolicyPort{{Port: intPort(8080)}}
	got := convertPorts(in)
	if len(got) != 1 || got[0].Port != 8080 || got[0].Protocol != "TCP" {
		t.Errorf("want [{8080 TCP}], got %v", got)
	}
	if got[0].Name != "" || got[0].EndPort != 0 {
		t.Errorf("unexpected Name/EndPort populated: %+v", got[0])
	}
}

func TestConvertPorts_ExplicitProtocol(t *testing.T) {
	in := []networkingv1.NetworkPolicyPort{{Port: intPort(53), Protocol: proto(corev1.ProtocolUDP)}}
	got := convertPorts(in)
	if got[0].Protocol != "UDP" {
		t.Errorf("want UDP, got %s", got[0].Protocol)
	}
}

func TestConvertPorts_NamedPort(t *testing.T) {
	in := []networkingv1.NetworkPolicyPort{{Port: namedPort("http")}}
	got := convertPorts(in)
	if got[0].Name != "http" {
		t.Errorf("want Name=http, got %q", got[0].Name)
	}
	if got[0].Port != 0 {
		t.Errorf("want Port=0 for named port, got %d", got[0].Port)
	}
}

func TestConvertPorts_Range(t *testing.T) {
	endPort := int32(8090)
	in := []networkingv1.NetworkPolicyPort{{Port: intPort(8080), EndPort: &endPort}}
	got := convertPorts(in)
	if got[0].Port != 8080 || got[0].EndPort != 8090 {
		t.Errorf("want 8080-8090, got %d-%d", got[0].Port, got[0].EndPort)
	}
}

// Ports from a rule must propagate onto every tuple that rule produces.
func TestGenerateTuples_PortsPropagated(t *testing.T) {
	peer := networkingv1.NetworkPolicyPeer{
		PodSelector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "backend"}},
	}
	ports := []models.Port{{Port: 8080, Protocol: "TCP"}}
	tuples := generateTuples("pol", "default", 0, models.DirectionEgress, peer, ports, buildTestIndex(defaultTestNodes()))
	if len(tuples) == 0 {
		t.Fatal("expected at least 1 tuple")
	}
	for _, tuple := range tuples {
		if tuple.Port.Port != 8080 {
			t.Errorf("tuple missing expected port: %+v", tuple)
		}
	}
}
