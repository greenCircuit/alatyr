package graph

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

var (
	nodeFrontend = WorkloadNode{ID: "frontend-uid", Labels: map[string]string{"app": "frontend"}, Namespace: "default"}
	nodeBackend  = WorkloadNode{ID: "backend-uid", Labels: map[string]string{"app": "backend"}, Namespace: "default"}
	nodeNS       = WorkloadNode{ID: "ns-default", Type: NodeTypeNamespace, Namespace: "default", Labels: map[string]string{"kubernetes.io/metadata.name": "default"}}
)

func buildTestIndex(nodes []WorkloadNode) map[string]map[string][]*WorkloadNode {
	return map[string]map[string][]*WorkloadNode{
		"default": buildWorkloadIndex(nodes),
	}
}

func buildMultiNSIndex(nodesByNS map[string][]WorkloadNode) map[string]map[string][]*WorkloadNode {
	result := map[string]map[string][]*WorkloadNode{}
	for ns, nodes := range nodesByNS {
		result[ns] = buildWorkloadIndex(nodes)
	}
	return result
}

func defaultTestNodes() []WorkloadNode {
	return []WorkloadNode{nodeFrontend, nodeBackend}
}

func defaultTestNodesWithNS() []WorkloadNode {
	return []WorkloadNode{nodeFrontend, nodeBackend, nodeNS}
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

// buildEdges

func TestBuildEdges_EgressDirection(t *testing.T) {
	nodes := defaultTestNodes()
	policy := policyWithEgress(
		map[string]string{"app": "frontend"},
		map[string]string{"app": "backend"},
	)
	edges := buildEdges(buildTestIndex(nodes), []networkingv1.NetworkPolicy{policy})

	if len(edges) != 1 {
		t.Fatalf("expected 1 edge, got %d", len(edges))
	}
	e := edges[0]
	if e.Source != nodeFrontend.ID {
		t.Errorf("egress source: want %s, got %s", nodeFrontend.ID, e.Source)
	}
	if e.Target != nodeBackend.ID {
		t.Errorf("egress target: want %s, got %s", nodeBackend.ID, e.Target)
	}
	if e.Direction != DirectionEgress {
		t.Errorf("expected egress direction, got %s", e.Direction)
	}
}

func TestBuildEdges_IngressDirection(t *testing.T) {
	nodes := defaultTestNodes()
	policy := policyWithIngress(
		map[string]string{"app": "backend"},
		map[string]string{"app": "frontend"},
	)
	edges := buildEdges(buildTestIndex(nodes), []networkingv1.NetworkPolicy{policy})

	if len(edges) != 1 {
		t.Fatalf("expected 1 edge, got %d", len(edges))
	}
	e := edges[0]
	if e.Source != nodeFrontend.ID {
		t.Errorf("ingress source: want %s (sender), got %s", nodeFrontend.ID, e.Source)
	}
	if e.Target != nodeBackend.ID {
		t.Errorf("ingress target: want %s (receiver), got %s", nodeBackend.ID, e.Target)
	}
	if e.Direction != DirectionIngress {
		t.Errorf("expected ingress direction, got %s", e.Direction)
	}
}

func TestBuildEdges_NoPolicies(t *testing.T) {
	edges := buildEdges(buildTestIndex(defaultTestNodes()), nil)
	if len(edges) != 0 {
		t.Errorf("expected no edges, got %d", len(edges))
	}
}

func TestBuildEdges_NoMatchingNodes(t *testing.T) {
	nodes := defaultTestNodes()
	policy := policyWithEgress(
		map[string]string{"app": "unknown"},
		map[string]string{"app": "backend"},
	)
	edges := buildEdges(buildTestIndex(nodes), []networkingv1.NetworkPolicy{policy})
	if len(edges) != 0 {
		t.Errorf("expected no edges when source selector matches nothing, got %d", len(edges))
	}
}

// getTargetEgressNodes

func TestGetTargetEgressNodes_Match(t *testing.T) {
	nodes := defaultTestNodes()
	policy := policyWithEgress(
		map[string]string{"app": "frontend"},
		map[string]string{"app": "backend"},
	)
	edges := getTargetEgressNodes(policy, buildTestIndex(nodes))
	if len(edges) != 1 {
		t.Fatalf("expected 1 egress edge, got %d", len(edges))
	}
	if edges[0].Target != nodeBackend.ID {
		t.Errorf("want target %s, got %s", nodeBackend.ID, edges[0].Target)
	}
	if edges[0].Direction != DirectionEgress {
		t.Errorf("expected egress direction, got %s", edges[0].Direction)
	}
}

func TestGetTargetEgressNodes_NilPodSelector(t *testing.T) {
	policy := networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{Namespace: "default"},
		Spec: networkingv1.NetworkPolicySpec{
			Egress: []networkingv1.NetworkPolicyEgressRule{
				{To: []networkingv1.NetworkPolicyPeer{{PodSelector: nil}}},
			},
		},
	}
	edges := getTargetEgressNodes(policy, buildTestIndex(defaultTestNodes()))
	if len(edges) != 0 {
		t.Errorf("nil PodSelector should produce no edges, got %d", len(edges))
	}
}

func TestGetTargetEgressNodes_NoRules(t *testing.T) {
	policy := networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{Namespace: "default"},
	}
	edges := getTargetEgressNodes(policy, buildTestIndex(defaultTestNodes()))
	if len(edges) != 0 {
		t.Errorf("expected no edges, got %d", len(edges))
	}
}

// getTargetIngressNodes

func TestGetTargetIngressNodes_Match(t *testing.T) {
	nodes := defaultTestNodes()
	policy := policyWithIngress(
		map[string]string{"app": "backend"},
		map[string]string{"app": "frontend"},
	)
	edges := getTargetIngressNodes(policy, buildTestIndex(nodes))
	if len(edges) != 1 {
		t.Fatalf("expected 1 ingress edge, got %d", len(edges))
	}
	if edges[0].Target != nodeFrontend.ID {
		t.Errorf("want target %s, got %s", nodeFrontend.ID, edges[0].Target)
	}
	if edges[0].Direction != DirectionIngress {
		t.Errorf("expected ingress direction, got %s", edges[0].Direction)
	}
}

func TestGetTargetIngressNodes_NilPodSelector(t *testing.T) {
	policy := networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{Namespace: "default"},
		Spec: networkingv1.NetworkPolicySpec{
			Ingress: []networkingv1.NetworkPolicyIngressRule{
				{From: []networkingv1.NetworkPolicyPeer{{PodSelector: nil}}},
			},
		},
	}
	edges := getTargetIngressNodes(policy, buildTestIndex(defaultTestNodes()))
	if len(edges) != 0 {
		t.Errorf("nil PodSelector should produce no edges, got %d", len(edges))
	}
}

func TestGetTargetIngressNodes_NoRules(t *testing.T) {
	policy := networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{Namespace: "default"},
	}
	edges := getTargetIngressNodes(policy, buildTestIndex(defaultTestNodes()))
	if len(edges) != 0 {
		t.Errorf("expected no edges, got %d", len(edges))
	}
}

// generateEdges — namespace-only selector

func TestGenerateEdges_NSOnly_CatchAll(t *testing.T) {
	peer := networkingv1.NetworkPolicyPeer{
		NamespaceSelector: &metav1.LabelSelector{}, // empty = catch-all → skip
	}
	edges := generateEdges("pol", "default", DirectionEgress, peer, nil, buildTestIndex(defaultTestNodesWithNS()))
	if edges != nil {
		t.Errorf("catch-all namespace selector should return nil, got %v", edges)
	}
}

func TestGenerateEdges_NSOnly_Match(t *testing.T) {
	peer := networkingv1.NetworkPolicyPeer{
		NamespaceSelector: &metav1.LabelSelector{
			MatchLabels: map[string]string{"kubernetes.io/metadata.name": "default"},
		},
	}
	edges := generateEdges("pol", "src", DirectionEgress, peer, nil, buildTestIndex(defaultTestNodesWithNS()))
	if len(edges) != 1 {
		t.Fatalf("expected 1 edge, got %d", len(edges))
	}
	if edges[0].Target != nodeNS.ID {
		t.Errorf("want target %s, got %s", nodeNS.ID, edges[0].Target)
	}
	if edges[0].Level != EdgeLevelNamespace {
		t.Errorf("want EdgeLevelNamespace, got %s", edges[0].Level)
	}
}

func TestGenerateEdges_NSOnly_NoMatch(t *testing.T) {
	peer := networkingv1.NetworkPolicyPeer{
		NamespaceSelector: &metav1.LabelSelector{
			MatchLabels: map[string]string{"kubernetes.io/metadata.name": "other"},
		},
	}
	edges := generateEdges("pol", "src", DirectionEgress, peer, nil, buildTestIndex(defaultTestNodesWithNS()))
	if len(edges) != 0 {
		t.Errorf("non-matching namespace selector should produce no edges, got %d", len(edges))
	}
}

// generateEdges — pod-only selector

func TestGenerateEdges_PodOnly_CatchAll(t *testing.T) {
	peer := networkingv1.NetworkPolicyPeer{
		PodSelector: &metav1.LabelSelector{}, // empty = catch-all → collapse to NS node
	}
	edges := generateEdges("pol", "default", DirectionEgress, peer, nil, buildTestIndex(defaultTestNodesWithNS()))
	if len(edges) != 1 {
		t.Fatalf("expected 1 edge to NS node, got %d", len(edges))
	}
	if edges[0].Target != nodeNS.ID {
		t.Errorf("want NS node target %s, got %s", nodeNS.ID, edges[0].Target)
	}
	if edges[0].Level != EdgeLevelNamespace {
		t.Errorf("want EdgeLevelNamespace, got %s", edges[0].Level)
	}
}

// generateEdges — pod + namespace selector

func TestGenerateEdges_BothSelectors_SpecificNSSpecificPod(t *testing.T) {
	otherNSNode := WorkloadNode{ID: "ns-other", Type: NodeTypeNamespace, Namespace: "other", Labels: map[string]string{"kubernetes.io/metadata.name": "other"}}
	otherPod := WorkloadNode{ID: "other-backend", Labels: map[string]string{"app": "backend"}, Namespace: "other"}
	index := buildMultiNSIndex(map[string][]WorkloadNode{
		"default": defaultTestNodesWithNS(),
		"other":   {otherPod, otherNSNode},
	})
	peer := networkingv1.NetworkPolicyPeer{
		PodSelector:       &metav1.LabelSelector{MatchLabels: map[string]string{"app": "backend"}},
		NamespaceSelector: &metav1.LabelSelector{MatchLabels: map[string]string{"kubernetes.io/metadata.name": "other"}},
	}
	edges := generateEdges("pol", "default", DirectionEgress, peer, nil, index)
	if len(edges) != 1 {
		t.Fatalf("expected 1 edge, got %d", len(edges))
	}
	if edges[0].Target != otherPod.ID {
		t.Errorf("want target %s, got %s", otherPod.ID, edges[0].Target)
	}
	if edges[0].Level != EdgeLevelWorkload {
		t.Errorf("want EdgeLevelWorkload, got %s", edges[0].Level)
	}
}

func TestGenerateEdges_BothSelectors_CatchAllNSSpecificPod(t *testing.T) {
	otherNSNode := WorkloadNode{ID: "ns-other", Type: NodeTypeNamespace, Namespace: "other", Labels: map[string]string{"kubernetes.io/metadata.name": "other"}}
	otherPod := WorkloadNode{ID: "other-backend", Labels: map[string]string{"app": "backend"}, Namespace: "other"}
	index := buildMultiNSIndex(map[string][]WorkloadNode{
		"default": defaultTestNodesWithNS(),               // has nodeBackend with app=backend
		"other":   {otherPod, otherNSNode},
	})
	peer := networkingv1.NetworkPolicyPeer{
		PodSelector:       &metav1.LabelSelector{MatchLabels: map[string]string{"app": "backend"}},
		NamespaceSelector: &metav1.LabelSelector{}, // catch-all NS
	}
	edges := generateEdges("pol", "default", DirectionEgress, peer, nil, index)
	if len(edges) != 2 {
		t.Fatalf("expected 1 edge per NS with matching pod (2 total), got %d", len(edges))
	}
}

func TestGenerateEdges_BothSelectors_SpecificNSCatchAllPod(t *testing.T) {
	otherNSNode := WorkloadNode{ID: "ns-other", Type: NodeTypeNamespace, Namespace: "other", Labels: map[string]string{"kubernetes.io/metadata.name": "other"}}
	otherPod := WorkloadNode{ID: "other-backend", Labels: map[string]string{"app": "backend"}, Namespace: "other"}
	index := buildMultiNSIndex(map[string][]WorkloadNode{
		"default": defaultTestNodesWithNS(),
		"other":   {otherPod, otherNSNode},
	})
	peer := networkingv1.NetworkPolicyPeer{
		PodSelector:       &metav1.LabelSelector{}, // catch-all pod → use NS node
		NamespaceSelector: &metav1.LabelSelector{MatchLabels: map[string]string{"kubernetes.io/metadata.name": "other"}},
	}
	edges := generateEdges("pol", "default", DirectionEgress, peer, nil, index)
	if len(edges) != 1 {
		t.Fatalf("expected 1 NS-level edge, got %d", len(edges))
	}
	if edges[0].Target != otherNSNode.ID {
		t.Errorf("want NS node target %s, got %s", otherNSNode.ID, edges[0].Target)
	}
	if edges[0].Level != EdgeLevelNamespace {
		t.Errorf("want EdgeLevelNamespace, got %s", edges[0].Level)
	}
}

// generateEdges — IP block

func TestGenerateEdges_IPBlock(t *testing.T) {
	peer := networkingv1.NetworkPolicyPeer{
		IPBlock: &networkingv1.IPBlock{CIDR: "10.0.0.0/8"},
	}
	edges := generateEdges("pol", "default", DirectionEgress, peer, nil, buildTestIndex(defaultTestNodes()))
	if len(edges) != 1 {
		t.Fatalf("expected 1 edge, got %d", len(edges))
	}
	if edges[0].Target != "10.0.0.0/8" {
		t.Errorf("want CIDR as target, got %s", edges[0].Target)
	}
	if edges[0].Level != EdgeLevelWorkload {
		t.Errorf("want EdgeLevelWorkload, got %s", edges[0].Level)
	}
}

// ── port conversion ─────────────────────────────────────────────────────────

func intPort(n int32) *intstr.IntOrString {
	v := intstr.FromInt(int(n))
	return &v
}

func namedPort(name string) *intstr.IntOrString {
	v := intstr.FromString(name)
	return &v
}

func proto(p corev1.Protocol) *corev1.Protocol { return &p }

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

// Ports from a rule must propagate onto every PolicyEdge that rule produces.
func TestGenerateEdges_PortsPropagated(t *testing.T) {
	peer := networkingv1.NetworkPolicyPeer{
		PodSelector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "backend"}},
	}
	ports := []Port{{Port: 8080, Protocol: "TCP"}}
	edges := generateEdges("pol", "default", DirectionEgress, peer, ports, buildTestIndex(defaultTestNodes()))
	if len(edges) == 0 {
		t.Fatal("expected at least 1 edge")
	}
	for _, e := range edges {
		if len(e.Ports) != 1 || e.Ports[0].Port != 8080 {
			t.Errorf("edge missing ports: %+v", e)
		}
	}
}
