package k8spolicy

import (
	"testing"

	"alatyr/internal/models"
	"alatyr/internal/utils"

	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

// testPolicy builds a minimal *networkingv1.NetworkPolicy with just name +
// namespace set. Used by expandPeerRules call sites that only need the
// identity strings — timestamps, spec fields not exercised here.
func testPolicy(name, namespace string) *networkingv1.NetworkPolicy {
	return &networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
	}
}

// buildFixtureNSIndex constructs an NSIndex for one namespace's fixture data.
// Mirrors graph.buildWorkloadIndex but inlined here to avoid a graph→k8spolicy
// cycle. Picks up the namespace node from the fixture if present.
func buildFixtureNSIndex(nodes []models.WorkloadNode) models.NSIndex {
	labelIndex := map[string][]*models.WorkloadNode{}
	var nsNode *models.WorkloadNode
	for position := range nodes {
		node := &nodes[position]
		if node.Type == models.NodeTypeNamespace {
			nsNode = node
		}
		for key, value := range node.Labels {
			entry := utils.MakeLabelIndexKey(key, value)
			labelIndex[entry] = append(labelIndex[entry], node)
		}
	}
	return models.NSIndex{
		Workloads:  nodes,
		LabelIndex: labelIndex,
		NSNode:     nsNode,
	}
}

var (
	nodeFrontend = models.WorkloadNode{ID: "frontend-uid", Labels: map[string]string{"app": "frontend"}, Namespace: "default"}
	nodeBackend  = models.WorkloadNode{ID: "backend-uid", Labels: map[string]string{"app": "backend"}, Namespace: "default"}
	nodeNS       = models.WorkloadNode{ID: "ns-default", Type: models.NodeTypeNamespace, Namespace: "default", Labels: map[string]string{"kubernetes.io/metadata.name": "default"}}
)

func buildTestIndex(nodes []models.WorkloadNode) map[string]models.NSIndex {
	return map[string]models.NSIndex{"default": buildFixtureNSIndex(nodes)}
}

func buildMultiNSIndex(nodesByNS map[string][]models.WorkloadNode) map[string]models.NSIndex {
	result := map[string]models.NSIndex{}
	for ns, nodes := range nodesByNS {
		result[ns] = buildFixtureNSIndex(nodes)
	}
	return result
}

// buildAllowTuples adapts the test-fixture shape (single NS policy slice) to
// buildAllowRulesByNs' map-by-NS signature, then flattens for assertions.
func buildAllowTuples(index map[string]models.NSIndex, policies []networkingv1.NetworkPolicy) []models.Rule {
	policiesByNS := map[string][]*networkingv1.NetworkPolicy{}
	for i := range policies {
		networkPolicy := &policies[i]
		namespace := networkPolicy.Namespace
		if namespace == "" {
			namespace = "default"
		}
		policiesByNS[namespace] = append(policiesByNS[namespace], networkPolicy)
	}
	rulesByNs, _, _ := buildAllowRulesByNs(index, policiesByNS)
	var flat []models.Rule
	for _, rules := range rulesByNs {
		flat = append(flat, rules...)
	}
	return flat
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
	tuples := buildAllowTuples(buildTestIndex(nodes), []networkingv1.NetworkPolicy{networkPolicy})

	// Policy also implies an ingress lock (k8s: all policies affect Ingress),
	// so a deny-all ingress marker rides along. Assert on the real allow tuple.
	allow := rulesWithCoverage(tuples, models.CoverageRestricted)
	if len(allow) != 1 {
		t.Fatalf("expected 1 restricted allow tuple, got %d (total %d)", len(allow), len(tuples))
	}
	tuple := allow[0]
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
	tuples := buildAllowTuples(buildTestIndex(nodes), []networkingv1.NetworkPolicy{networkPolicy})

	// Egress is not locked here → an unenforced egress marker rides along.
	// Assert on the real allow tuple.
	allow := rulesWithCoverage(tuples, models.CoverageRestricted)
	if len(allow) != 1 {
		t.Fatalf("expected 1 restricted allow tuple, got %d (total %d)", len(allow), len(tuples))
	}
	tuple := allow[0]
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
	tuples := buildAllowTuples(buildTestIndex(defaultTestNodes()), nil)
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
	tuples := buildAllowTuples(buildTestIndex(nodes), []networkingv1.NetworkPolicy{networkPolicy})
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
	tuples, _ := expandEgressRules(&networkPolicy, buildTestIndex(nodes))
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
	tuples, _ := expandEgressRules(&networkPolicy, buildTestIndex(defaultTestNodes()))
	if len(tuples) != 0 {
		t.Errorf("nil PodSelector should produce no tuples, got %d", len(tuples))
	}
}

func TestGetTargetEgressTuples_NoRules(t *testing.T) {
	networkPolicy := networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{Namespace: "default"},
	}
	// Bare policy: no PolicyTypes → egress not locked. Recorded as unenforced
	// (visible in the table) rather than dropped, but never deny-all.
	tuples, _ := expandEgressRules(&networkPolicy, buildTestIndex(defaultTestNodes()))
	if len(tuples) != 1 {
		t.Fatalf("expected 1 egress marker, got %d", len(tuples))
	}
	if tuples[0].Coverage != models.CoverageUnenforced {
		t.Errorf("egress not locked → want unenforced, got %q", tuples[0].Coverage)
	}
}

// getTargetIngressTuples

func TestGetTargetIngressTuples_Match(t *testing.T) {
	nodes := defaultTestNodes()
	networkPolicy := policyWithIngress(
		map[string]string{"app": "backend"},
		map[string]string{"app": "frontend"},
	)
	tuples, _ := expandIngressRules(&networkPolicy, buildTestIndex(nodes))
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
	tuples, _ := expandIngressRules(&networkPolicy, buildTestIndex(defaultTestNodes()))
	if len(tuples) != 0 {
		t.Errorf("nil PodSelector should produce no tuples, got %d", len(tuples))
	}
}

func TestGetTargetIngressTuples_NoRules(t *testing.T) {
	networkPolicy := networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{Namespace: "default"},
	}
	// Bare policy: empty PolicyTypes implies Ingress lock → deny-all ingress.
	tuples, _ := expandIngressRules(&networkPolicy, buildTestIndex(defaultTestNodes()))
	if len(tuples) != 1 {
		t.Fatalf("expected 1 ingress marker, got %d", len(tuples))
	}
	if tuples[0].Coverage != models.CoverageDenyAll {
		t.Errorf("ingress implied-locked → want deny-all, got %q", tuples[0].Coverage)
	}
}

// generateTuples — namespace-only selector

func TestGenerateTuples_NSOnly_CatchAll(t *testing.T) {
	peer := networkingv1.NetworkPolicyPeer{
		NamespaceSelector: &metav1.LabelSelector{}, // empty = catch-all → skip
	}
	tuples, _ := expandPeerRules(testPolicy("pol", "default"), 0, models.DirectionEgress, peer, nil, buildTestIndex(defaultTestNodesWithNS()))
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
	tuples, _ := expandPeerRules(testPolicy("pol", "src"), 0, models.DirectionEgress, peer, nil, buildTestIndex(defaultTestNodesWithNS()))
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
	tuples, _ := expandPeerRules(testPolicy("pol", "src"), 0, models.DirectionEgress, peer, nil, buildTestIndex(defaultTestNodesWithNS()))
	if len(tuples) != 0 {
		t.Errorf("non-matching namespace selector should produce no tuples, got %d", len(tuples))
	}
}

// generateTuples — pod-only selector

func TestGenerateTuples_PodOnly_CatchAll(t *testing.T) {
	peer := networkingv1.NetworkPolicyPeer{
		PodSelector: &metav1.LabelSelector{}, // empty = catch-all → collapse to NS node
	}
	tuples, _ := expandPeerRules(testPolicy("pol", "default"), 0, models.DirectionEgress, peer, nil, buildTestIndex(defaultTestNodesWithNS()))
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
	tuples, _ := expandPeerRules(testPolicy("pol", "default"), 0, models.DirectionEgress, peer, nil, index)
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
	tuples, _ := expandPeerRules(testPolicy("pol", "default"), 0, models.DirectionEgress, peer, nil, index)
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
	tuples, _ := expandPeerRules(testPolicy("pol", "default"), 0, models.DirectionEgress, peer, nil, index)
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
	tuples, cidrNodes := expandPeerRules(testPolicy("pol", "default"), 0, models.DirectionEgress, peer, nil, buildTestIndex(defaultTestNodes()))
	if len(tuples) != 1 {
		t.Fatalf("expected 1 tuple, got %d", len(tuples))
	}
	if tuples[0].DstID != "cidr:10.0.0.0/8" {
		t.Errorf("want prefixed CIDR as target, got %s", tuples[0].DstID)
	}
	if tuples[0].Action != models.ActionAllow {
		t.Errorf("want Allow, got %v", tuples[0].Action)
	}
	node, ok := cidrNodes["cidr:10.0.0.0/8"]
	if !ok {
		t.Fatalf("expected synthetic CIDR node for cidr:10.0.0.0/8, got %v", cidrNodes)
	}
	if node.Type != models.NodeTypeCIDR {
		t.Errorf("want NodeTypeCIDR, got %s", node.Type)
	}
	if node.Label != "10.0.0.0/8" {
		t.Errorf("want raw CIDR as Label, got %s", node.Label)
	}
}

func TestGenerateTuples_IPBlock_Except_EmitsDenyRules(t *testing.T) {
	peer := networkingv1.NetworkPolicyPeer{
		IPBlock: &networkingv1.IPBlock{
			CIDR:   "10.0.0.0/8",
			Except: []string{"10.1.0.0/16", "10.2.0.0/16"},
		},
	}
	tuples, cidrNodes := expandPeerRules(testPolicy("pol", "default"), 0, models.DirectionEgress, peer, nil, buildTestIndex(defaultTestNodes()))
	if len(tuples) != 3 {
		t.Fatalf("want 1 allow + 2 deny (3 total), got %d: %+v", len(tuples), tuples)
	}
	var allow, deny []models.Rule
	for _, tuple := range tuples {
		if tuple.Action == models.ActionAllow {
			allow = append(allow, tuple)
		} else {
			deny = append(deny, tuple)
		}
	}
	if len(allow) != 1 || allow[0].DstID != "cidr:10.0.0.0/8" {
		t.Errorf("want 1 allow to cidr:10.0.0.0/8, got %+v", allow)
	}
	if len(deny) != 2 {
		t.Fatalf("want 2 deny rules, got %d", len(deny))
	}
	denyIDs := map[string]bool{deny[0].DstID: true, deny[1].DstID: true}
	if !denyIDs["cidr:10.1.0.0/16"] || !denyIDs["cidr:10.2.0.0/16"] {
		t.Errorf("deny targets mismatch: %+v", denyIDs)
	}
	for _, tuple := range deny {
		if tuple.Coverage != models.CoverageExcept {
			t.Errorf("except deny must carry CoverageExcept, got %q", tuple.Coverage)
		}
		if tuple.Contributor.Name != "pol" {
			t.Errorf("except deny lost contributor: %+v", tuple.Contributor)
		}
	}
	for _, id := range []string{"cidr:10.0.0.0/8", "cidr:10.1.0.0/16", "cidr:10.2.0.0/16"} {
		if _, ok := cidrNodes[id]; !ok {
			t.Errorf("missing synthetic node for %s", id)
		}
	}
}

func TestGenerateTuples_IPBlock_Except_SelfNullifying(t *testing.T) {
	// Pathological but legal: except covers the entire allowed CIDR.
	peer := networkingv1.NetworkPolicyPeer{
		IPBlock: &networkingv1.IPBlock{
			CIDR:   "10.0.0.0/8",
			Except: []string{"10.0.0.0/8"},
		},
	}
	tuples, _ := expandPeerRules(testPolicy("pol", "default"), 0, models.DirectionEgress, peer, nil, buildTestIndex(defaultTestNodes()))
	if len(tuples) != 2 {
		t.Fatalf("want 1 allow + 1 deny on same CIDR, got %d", len(tuples))
	}
	var sawAllow, sawDeny bool
	for _, tuple := range tuples {
		if tuple.DstID != "cidr:10.0.0.0/8" {
			t.Errorf("want same CIDR on both, got %s", tuple.DstID)
		}
		if tuple.Action == models.ActionAllow {
			sawAllow = true
		}
		if tuple.Action == models.ActionDeny && tuple.Coverage == models.CoverageExcept {
			sawDeny = true
		}
	}
	if !sawAllow || !sawDeny {
		t.Errorf("want both an allow and an except-deny on the same CIDR, got allow=%v deny=%v", sawAllow, sawDeny)
	}
}

func TestBuildAllowRulesByNs_CIDRNodesSurfaced(t *testing.T) {
	networkPolicy := networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{Name: "np", Namespace: "default"},
		Spec: networkingv1.NetworkPolicySpec{
			PodSelector: metav1.LabelSelector{MatchLabels: map[string]string{"app": "frontend"}},
			Egress: []networkingv1.NetworkPolicyEgressRule{
				{To: []networkingv1.NetworkPolicyPeer{
					{IPBlock: &networkingv1.IPBlock{CIDR: "0.0.0.0/0", Except: []string{"169.254.169.254/32"}}},
				}},
			},
		},
	}
	policiesByNS := map[string][]*networkingv1.NetworkPolicy{"default": {&networkPolicy}}
	_, _, cidrNodes := buildAllowRulesByNs(buildTestIndex(defaultTestNodes()), policiesByNS)
	for _, id := range []string{"cidr:0.0.0.0/0", "cidr:169.254.169.254/32"} {
		node, ok := cidrNodes[id]
		if !ok {
			t.Errorf("missing node for %s", id)
			continue
		}
		if node.Type != models.NodeTypeCIDR {
			t.Errorf("%s: want NodeTypeCIDR, got %s", id, node.Type)
		}
	}
}

func TestBuildAllowRulesByNs_CIDRNodesDedupedAcrossPolicies(t *testing.T) {
	// Two policies referencing the same CIDR must produce one node, not two.
	makePolicy := func(name string) *networkingv1.NetworkPolicy {
		return &networkingv1.NetworkPolicy{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default"},
			Spec: networkingv1.NetworkPolicySpec{
				PodSelector: metav1.LabelSelector{MatchLabels: map[string]string{"app": "frontend"}},
				Egress: []networkingv1.NetworkPolicyEgressRule{
					{To: []networkingv1.NetworkPolicyPeer{
						{IPBlock: &networkingv1.IPBlock{CIDR: "10.0.0.0/8"}},
					}},
				},
			},
		}
	}
	policiesByNS := map[string][]*networkingv1.NetworkPolicy{
		"default": {makePolicy("pol-a"), makePolicy("pol-b")},
	}
	_, _, cidrNodes := buildAllowRulesByNs(buildTestIndex(defaultTestNodes()), policiesByNS)
	if len(cidrNodes) != 1 {
		t.Errorf("want single deduped CIDR node, got %d: %+v", len(cidrNodes), cidrNodes)
	}
}

func TestGenerateTuples_IngressIPBlock_SrcIDPrefixedAfterSwap(t *testing.T) {
	// Ingress path: buildAllowRulesByNs swaps SrcID/DstID. CIDR peer must land
	// in SrcID with the cidr: prefix intact.
	networkPolicy := networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{Name: "np", Namespace: "default"},
		Spec: networkingv1.NetworkPolicySpec{
			PodSelector: metav1.LabelSelector{MatchLabels: map[string]string{"app": "backend"}},
			Ingress: []networkingv1.NetworkPolicyIngressRule{
				{From: []networkingv1.NetworkPolicyPeer{
					{IPBlock: &networkingv1.IPBlock{CIDR: "192.168.1.0/24"}},
				}},
			},
		},
	}
	tuples := buildAllowTuples(buildTestIndex(defaultTestNodes()), []networkingv1.NetworkPolicy{networkPolicy})
	var found bool
	for _, tuple := range tuples {
		if tuple.Direction == models.DirectionIngress && tuple.SrcID == "cidr:192.168.1.0/24" && tuple.DstID == nodeBackend.ID {
			found = true
		}
	}
	if !found {
		t.Errorf("want ingress rule with SrcID=cidr:192.168.1.0/24 → DstID=%s; got %+v", nodeBackend.ID, tuples)
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
	tuples, _ := expandPeerRules(testPolicy("pol", "default"), 0, models.DirectionEgress, peer, ports, buildTestIndex(defaultTestNodes()))
	if len(tuples) == 0 {
		t.Fatal("expected at least 1 tuple")
	}
	for _, tuple := range tuples {
		if len(tuple.Ports) != 1 || tuple.Ports[0].Port != 8080 {
			t.Errorf("tuple missing expected port: %+v", tuple)
		}
	}
}
