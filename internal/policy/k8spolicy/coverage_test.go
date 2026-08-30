package k8spolicy

import (
	"fmt"
	"sort"
	"testing"

	"graph/internal/models"

	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

// Coverage classification tests. Assert the CORRECT k8s semantics for deny-all
// / allow-all / allow-all-ns / restricted, exercised directly on
// expandEgressRules + expandIngressRules so SrcID stamping (buildAllowRulesByNs)
// doesn't muddy the classification signal.
//
// Rule semantics under test:
//   deny-all D    = direction D locked (in PolicyTypes) AND zero rules for D
//   allow-all D   = a rule with an empty peer list (to:[{}] / from:[{}])
//   allow-all-ns  = a peer with a catch-all podSelector in the policy's own ns
//   restricted    = any specific selector / ipBlock
// A direction with len(rules)==0 but NOT locked is transparent — no marker.

func coveragePolicy(
	podLabels map[string]string,
	types []networkingv1.PolicyType,
	egress []networkingv1.NetworkPolicyEgressRule,
	ingress []networkingv1.NetworkPolicyIngressRule,
) *networkingv1.NetworkPolicy {
	return &networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{Name: "np", Namespace: "default"},
		Spec: networkingv1.NetworkPolicySpec{
			PodSelector: metav1.LabelSelector{MatchLabels: podLabels},
			PolicyTypes: types,
			Egress:      egress,
			Ingress:     ingress,
		},
	}
}

func podPeer(labels map[string]string) networkingv1.NetworkPolicyPeer {
	return networkingv1.NetworkPolicyPeer{PodSelector: &metav1.LabelSelector{MatchLabels: labels}}
}

// ruleKeys reduces rules to a comparable, order-independent signature:
// "direction|coverage|dstID".
func ruleKeys(rules []models.Rule) []string {
	keys := make([]string, 0, len(rules))
	for _, rule := range rules {
		keys = append(keys, fmt.Sprintf("%s|%s|%s", rule.Direction, rule.Coverage, rule.DstID))
	}
	sort.Strings(keys)
	return keys
}

func rulesWithCoverage(rules []models.Rule, coverage models.Coverage) []models.Rule {
	var out []models.Rule
	for _, rule := range rules {
		if rule.Coverage == coverage {
			out = append(out, rule)
		}
	}
	return out
}

func assertRuleKeys(t *testing.T, got []models.Rule, want []string) {
	t.Helper()
	gotKeys := ruleKeys(got)
	sort.Strings(want)
	if len(gotKeys) != len(want) {
		t.Fatalf("rule count: want %d %v, got %d %v", len(want), want, len(gotKeys), gotKeys)
	}
	for i := range want {
		if gotKeys[i] != want[i] {
			t.Errorf("rule[%d]: want %q, got %q (all got: %v)", i, want[i], gotKeys[i], gotKeys)
		}
	}
}

var (
	frontendSel = map[string]string{"app": "frontend"}
	backendSel  = map[string]string{"app": "backend"}
	catchAllSel = map[string]string{}
	egressType  = []networkingv1.PolicyType{networkingv1.PolicyTypeEgress}
	ingressType = []networkingv1.PolicyType{networkingv1.PolicyTypeIngress}
)

func TestExpandEgressRules_Coverage(t *testing.T) {
	index := buildTestIndex(defaultTestNodesWithNS())
	nsID := nodeNS.ID
	backendID := nodeBackend.ID

	cases := []struct {
		name   string
		policy *networkingv1.NetworkPolicy
		want   []string // direction|coverage|dstID
	}{
		{
			name:   "deny-all egress: locked, zero rules",
			policy: coveragePolicy(frontendSel, egressType, nil, nil),
			want:   []string{"egress|" + string(models.CoverageDenyAll) + "|"},
		},
		{
			// egress NOT in PolicyTypes → not deny-all, but still recorded as
			// unenforced so a broken/no-op policy stays visible in the table.
			name:   "no egress lock: ingress-only policy records unenforced egress",
			policy: coveragePolicy(frontendSel, ingressType, nil, nil),
			want:   []string{"egress|" + string(models.CoverageUnenforced) + "|"},
		},
		{
			name: "allow-all egress: empty rule",
			policy: coveragePolicy(frontendSel, egressType,
				[]networkingv1.NetworkPolicyEgressRule{{}}, nil),
			want: []string{"egress|" + string(models.CoverageAllowAll) + "|"},
		},
		{
			name: "allow-all-ns egress: catch-all podSelector peer",
			policy: coveragePolicy(frontendSel, egressType,
				[]networkingv1.NetworkPolicyEgressRule{{To: []networkingv1.NetworkPolicyPeer{podPeer(catchAllSel)}}}, nil),
			want: []string{"egress|" + string(models.CoverageAllowAllNs) + "|" + nsID},
		},
		{
			name: "restricted egress: specific pod peer",
			policy: coveragePolicy(frontendSel, egressType,
				[]networkingv1.NetworkPolicyEgressRule{{To: []networkingv1.NetworkPolicyPeer{podPeer(backendSel)}}}, nil),
			want: []string{"egress|" + string(models.CoverageRestricted) + "|" + backendID},
		},
		{
			// allow-all must not swallow sibling rules regardless of order.
			name: "allow-all rule does not drop siblings",
			policy: coveragePolicy(frontendSel, egressType,
				[]networkingv1.NetworkPolicyEgressRule{
					{},
					{To: []networkingv1.NetworkPolicyPeer{podPeer(backendSel)}},
				}, nil),
			want: []string{
				"egress|" + string(models.CoverageAllowAll) + "|",
				"egress|" + string(models.CoverageRestricted) + "|" + backendID,
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rules, _ := expandEgressRules(tc.policy, index)
			assertRuleKeys(t, rules, tc.want)
		})
	}
}

func TestExpandIngressRules_Coverage(t *testing.T) {
	index := buildTestIndex(defaultTestNodesWithNS())
	frontendID := nodeFrontend.ID

	cases := []struct {
		name   string
		policy *networkingv1.NetworkPolicy
		want   []string
	}{
		{
			name:   "deny-all ingress: locked, zero rules",
			policy: coveragePolicy(backendSel, ingressType, nil, nil),
			want:   []string{"ingress|" + string(models.CoverageDenyAll) + "|"},
		},
		{
			// The classic warehouse case: ingress rule present, no egress block.
			// Must keep the real ingress allow, NOT collapse to deny-all.
			name: "ingress allow preserved when policy has no egress",
			policy: coveragePolicy(backendSel, ingressType, nil,
				[]networkingv1.NetworkPolicyIngressRule{{From: []networkingv1.NetworkPolicyPeer{podPeer(frontendSel)}}}),
			want: []string{"ingress|" + string(models.CoverageRestricted) + "|" + frontendID},
		},
		{
			name: "allow-all ingress: empty rule, correct direction",
			policy: coveragePolicy(backendSel, ingressType, nil,
				[]networkingv1.NetworkPolicyIngressRule{{}}),
			want: []string{"ingress|" + string(models.CoverageAllowAll) + "|"},
		},
		{
			name: "allow-all-ns ingress: catch-all podSelector peer",
			policy: coveragePolicy(backendSel, ingressType, nil,
				[]networkingv1.NetworkPolicyIngressRule{{From: []networkingv1.NetworkPolicyPeer{podPeer(catchAllSel)}}}),
			want: []string{"ingress|" + string(models.CoverageAllowAllNs) + "|" + nodeNS.ID},
		},
		{
			// Egress-only policy: ingress not locked → unenforced, not deny-all.
			name:   "egress-only policy records unenforced ingress",
			policy: coveragePolicy(backendSel, egressType, nil, nil),
			want:   []string{"ingress|" + string(models.CoverageUnenforced) + "|"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rules, _ := expandIngressRules(tc.policy, index)
			assertRuleKeys(t, rules, tc.want)
		})
	}
}

// tcpPort builds a single-port rule filter for the allow-all port tests.
func tcpPort(port int32) []networkingv1.NetworkPolicyPort {
	proto := corev1.ProtocolTCP
	return []networkingv1.NetworkPolicyPort{{Protocol: &proto, Port: &intstr.IntOrString{IntVal: port}}}
}

// Marker rules must carry their contributor, and the allow-all marker must not
// silently drop the port grant — an empty peer list with a port filter is
// "all destinations on THIS port", not "all ports".
func TestCoverageMarkers_ContributorAndPorts(t *testing.T) {
	index := buildTestIndex(defaultTestNodesWithNS())

	denyAll, _ := expandEgressRules(coveragePolicy(frontendSel, egressType, nil, nil), index)
	if len(denyAll) != 1 || denyAll[0].Contributor.Name != "np" {
		t.Fatalf("deny-all marker must attribute to policy 'np', got %+v", denyAll)
	}

	// allow-all with NO ports = all ports → AllPorts must be flagged.
	egressAll, _ := expandEgressRules(
		coveragePolicy(frontendSel, egressType, []networkingv1.NetworkPolicyEgressRule{{}}, nil), index)
	if len(egressAll) != 1 || egressAll[0].Contributor.Name != "np" {
		t.Fatalf("allow-all egress: want 1 attributed rule, got %+v", egressAll)
	}
	if !egressAll[0].AllPorts {
		t.Errorf("portless allow-all egress must set AllPorts=true")
	}

	// allow-all egress WITH a port = all destinations on 443 only. The port
	// must survive and AllPorts must be false.
	egressPort, _ := expandEgressRules(
		coveragePolicy(frontendSel, egressType,
			[]networkingv1.NetworkPolicyEgressRule{{Ports: tcpPort(443)}}, nil), index)
	if len(egressPort) != 1 {
		t.Fatalf("allow-all egress w/ port: want 1 rule, got %d", len(egressPort))
	}
	if egressPort[0].AllPorts {
		t.Errorf("allow-all egress with a port filter must NOT set AllPorts")
	}
	if len(egressPort[0].Ports) != 1 || egressPort[0].Ports[0].Port != 443 {
		t.Errorf("allow-all egress dropped its port: %+v", egressPort[0].Ports)
	}

	// same on the ingress side.
	ingressPort, _ := expandIngressRules(
		coveragePolicy(backendSel, ingressType, nil,
			[]networkingv1.NetworkPolicyIngressRule{{Ports: tcpPort(443)}}), index)
	if len(ingressPort) != 1 {
		t.Fatalf("allow-all ingress w/ port: want 1 rule, got %d", len(ingressPort))
	}
	if ingressPort[0].AllPorts {
		t.Errorf("allow-all ingress with a port filter must NOT set AllPorts")
	}
	if len(ingressPort[0].Ports) != 1 || ingressPort[0].Ports[0].Port != 443 {
		t.Errorf("allow-all ingress dropped its port: %+v", ingressPort[0].Ports)
	}
}
