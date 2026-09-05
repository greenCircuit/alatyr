package k8spolicy

import (
	"testing"

	"alatyr/internal/models"
	"alatyr/internal/policy"

	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// deriveStatusKeys exercises the engine's full status-key pipeline
// (buildPolicyStatus → policy.DeriveStatusKeys) so tests assert against
// final keys without depending on the deleted BuildStatusKeys facade.
func deriveStatusKeys(policies []networkingv1.NetworkPolicy) []models.StatusKey {
	ptrs := make([]*networkingv1.NetworkPolicy, len(policies))
	for i := range policies {
		ptrs[i] = &policies[i]
	}
	return policy.DeriveStatusKeys(buildPolicyStatus(ptrs))
}

// ── helpers ───────────────────────────────────────────────────────────────────

func makeNetworkPolicy(namespace string, policyTypes []networkingv1.PolicyType, ingress []networkingv1.NetworkPolicyIngressRule, egress []networkingv1.NetworkPolicyEgressRule) networkingv1.NetworkPolicy {
	return networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{Namespace: namespace},
		Spec: networkingv1.NetworkPolicySpec{
			PolicyTypes: policyTypes,
			Ingress:     ingress,
			Egress:      egress,
		},
	}
}

func ingressFrom(peers ...networkingv1.NetworkPolicyPeer) networkingv1.NetworkPolicyIngressRule {
	return networkingv1.NetworkPolicyIngressRule{From: peers}
}

func egressTo(peers ...networkingv1.NetworkPolicyPeer) networkingv1.NetworkPolicyEgressRule {
	return networkingv1.NetworkPolicyEgressRule{To: peers}
}

func ipBlockPeer(cidr string) networkingv1.NetworkPolicyPeer {
	return networkingv1.NetworkPolicyPeer{IPBlock: &networkingv1.IPBlock{CIDR: cidr}}
}

func nsSelectorPeer(nsName string) networkingv1.NetworkPolicyPeer {
	return networkingv1.NetworkPolicyPeer{
		NamespaceSelector: &metav1.LabelSelector{
			MatchLabels: map[string]string{"kubernetes.io/metadata.name": nsName},
		},
	}
}

func hasStatus(keys []models.StatusKey, want models.StatusKey) bool {
	for _, key := range keys {
		if key == want {
			return true
		}
	}
	return false
}

// ── BuildStatusKeys tests ─────────────────────────────────────────────────────

// No policies → workload is fully open: collapsed into StatusInternetFull.
func TestBuildStatusKeys_NoPolicies(t *testing.T) {
	got := deriveStatusKeys(nil)
	if !hasStatus(got, models.StatusInternetFull) {
		t.Error("expected StatusInternetFull when no policies")
	}
	if hasStatus(got, models.StatusIsolated) {
		t.Error("unexpected StatusIsolated when no policies")
	}
}

// Explicit deny-all: PolicyTypes=[Ingress,Egress] with no rules → isolated.
func TestBuildStatusKeys_DenyAll(t *testing.T) {
	networkPolicy := makeNetworkPolicy("ns-a",
		[]networkingv1.PolicyType{networkingv1.PolicyTypeIngress, networkingv1.PolicyTypeEgress},
		nil, nil,
	)
	got := deriveStatusKeys([]networkingv1.NetworkPolicy{networkPolicy})
	if !hasStatus(got, models.StatusIsolated) {
		t.Error("expected StatusIsolated for deny-all policy")
	}
	if hasStatus(got, models.StatusInternetEgress) {
		t.Error("unexpected StatusInternetEgress for deny-all policy")
	}
	if hasStatus(got, models.StatusInternetIngress) {
		t.Error("unexpected StatusInternetIngress for deny-all policy")
	}
}

// Only ingress locked (PolicyTypes=[Ingress]) → egress still open → internet egress fires.
func TestBuildStatusKeys_OnlyIngressLocked(t *testing.T) {
	networkPolicy := makeNetworkPolicy("ns-a",
		[]networkingv1.PolicyType{networkingv1.PolicyTypeIngress},
		nil, nil,
	)
	got := deriveStatusKeys([]networkingv1.NetworkPolicy{networkPolicy})
	if !hasStatus(got, models.StatusInternetEgress) {
		t.Error("expected StatusInternetEgress when only ingress is locked")
	}
	if hasStatus(got, models.StatusInternetIngress) {
		t.Error("unexpected StatusInternetIngress when ingress is locked with no internet rule")
	}
	if hasStatus(got, models.StatusIsolated) {
		t.Error("unexpected StatusIsolated when egress is not locked")
	}
}

// Only egress locked (PolicyTypes=[Egress]) → ingress still open → internet ingress fires.
func TestBuildStatusKeys_OnlyEgressLocked(t *testing.T) {
	networkPolicy := makeNetworkPolicy("ns-a",
		[]networkingv1.PolicyType{networkingv1.PolicyTypeEgress},
		nil, nil,
	)
	got := deriveStatusKeys([]networkingv1.NetworkPolicy{networkPolicy})
	if !hasStatus(got, models.StatusInternetIngress) {
		t.Error("expected StatusInternetIngress when only egress is locked")
	}
	if hasStatus(got, models.StatusInternetEgress) {
		t.Error("unexpected StatusInternetEgress when egress is locked with no internet rule")
	}
	if hasStatus(got, models.StatusIsolated) {
		t.Error("unexpected StatusIsolated when ingress is not locked")
	}
}

// Implicit PolicyTypes (unset) with no egress rules → only ingress locked.
func TestBuildStatusKeys_ImplicitIngressOnly(t *testing.T) {
	networkPolicy := makeNetworkPolicy("ns-a", nil, nil, nil)
	got := deriveStatusKeys([]networkingv1.NetworkPolicy{networkPolicy})
	if !hasStatus(got, models.StatusInternetEgress) {
		t.Error("expected StatusInternetEgress: implicit policy locks ingress only, egress remains open")
	}
	if hasStatus(got, models.StatusIsolated) {
		t.Error("unexpected StatusIsolated when egress is not locked")
	}
}

// Implicit PolicyTypes with egress rules → both directions locked → isolated.
func TestBuildStatusKeys_ImplicitBothLocked(t *testing.T) {
	networkPolicy := makeNetworkPolicy("ns-a", nil, nil,
		[]networkingv1.NetworkPolicyEgressRule{egressTo()},
	)
	got := deriveStatusKeys([]networkingv1.NetworkPolicy{networkPolicy})
	if !hasStatus(got, models.StatusIsolated) {
		t.Error("expected StatusIsolated: implicit policy with egress rules locks both directions")
	}
}

// Explicit internet egress (0.0.0.0/0) → StatusInternetEgress, no StatusIsolated.
func TestBuildStatusKeys_InternetEgress(t *testing.T) {
	networkPolicy := makeNetworkPolicy("ns-a",
		[]networkingv1.PolicyType{networkingv1.PolicyTypeIngress, networkingv1.PolicyTypeEgress},
		nil,
		[]networkingv1.NetworkPolicyEgressRule{egressTo(ipBlockPeer("0.0.0.0/0"))},
	)
	got := deriveStatusKeys([]networkingv1.NetworkPolicy{networkPolicy})
	if !hasStatus(got, models.StatusInternetEgress) {
		t.Error("expected StatusInternetEgress for 0.0.0.0/0 egress rule")
	}
	if hasStatus(got, models.StatusIsolated) {
		t.Error("unexpected StatusIsolated when internet egress is explicitly allowed")
	}
}

// Explicit internet ingress (0.0.0.0/0) → StatusInternetIngress, no StatusIsolated.
func TestBuildStatusKeys_InternetIngress(t *testing.T) {
	networkPolicy := makeNetworkPolicy("ns-a",
		[]networkingv1.PolicyType{networkingv1.PolicyTypeIngress, networkingv1.PolicyTypeEgress},
		[]networkingv1.NetworkPolicyIngressRule{ingressFrom(ipBlockPeer("0.0.0.0/0"))},
		nil,
	)
	got := deriveStatusKeys([]networkingv1.NetworkPolicy{networkPolicy})
	if !hasStatus(got, models.StatusInternetIngress) {
		t.Error("expected StatusInternetIngress for 0.0.0.0/0 ingress rule")
	}
	if hasStatus(got, models.StatusIsolated) {
		t.Error("unexpected StatusIsolated when internet ingress is explicitly allowed")
	}
}

// Private CIDR (10.0.0.0/8) is LAN, not internet — and prevents air-gapped.
func TestBuildStatusKeys_PrivateCIDR_LanNotInternet(t *testing.T) {
	networkPolicy := makeNetworkPolicy("ns-a",
		[]networkingv1.PolicyType{networkingv1.PolicyTypeIngress, networkingv1.PolicyTypeEgress},
		[]networkingv1.NetworkPolicyIngressRule{ingressFrom(ipBlockPeer("10.0.0.0/8"))},
		[]networkingv1.NetworkPolicyEgressRule{egressTo(ipBlockPeer("10.0.0.0/8"))},
	)
	got := deriveStatusKeys([]networkingv1.NetworkPolicy{networkPolicy})
	if hasStatus(got, models.StatusInternetEgress) {
		t.Error("unexpected StatusInternetEgress for private CIDR")
	}
	if hasStatus(got, models.StatusInternetIngress) {
		t.Error("unexpected StatusInternetIngress for private CIDR")
	}
	if !hasStatus(got, models.StatusLanFull) {
		t.Error("expected StatusLanFull: bidirectional LAN traffic")
	}
	if hasStatus(got, models.StatusIsolated) {
		t.Error("unexpected StatusIsolated: LAN traffic means not air-gapped")
	}
}

// Cross-namespace egress selector pointing to a DIFFERENT namespace → StatusCrossNamespace.
func TestBuildStatusKeys_CrossNSEgress(t *testing.T) {
	networkPolicy := makeNetworkPolicy("ns-a",
		[]networkingv1.PolicyType{networkingv1.PolicyTypeEgress},
		nil,
		[]networkingv1.NetworkPolicyEgressRule{egressTo(nsSelectorPeer("ns-b"))},
	)
	got := deriveStatusKeys([]networkingv1.NetworkPolicy{networkPolicy})
	if !hasStatus(got, models.StatusCrossNamespace) {
		t.Error("expected StatusCrossNamespace for egress to different namespace")
	}
}

// Cross-namespace ingress selector pointing to a DIFFERENT namespace → StatusCrossNamespace.
func TestBuildStatusKeys_CrossNSIngress(t *testing.T) {
	networkPolicy := makeNetworkPolicy("ns-a",
		[]networkingv1.PolicyType{networkingv1.PolicyTypeIngress},
		[]networkingv1.NetworkPolicyIngressRule{ingressFrom(nsSelectorPeer("ns-b"))},
		nil,
	)
	got := deriveStatusKeys([]networkingv1.NetworkPolicy{networkPolicy})
	if !hasStatus(got, models.StatusCrossNamespace) {
		t.Error("expected StatusCrossNamespace for ingress from different namespace")
	}
}

// NamespaceSelector pointing to SAME namespace must NOT trigger StatusCrossNamespace.
func TestBuildStatusKeys_SameNSSelectorNotCrossNS(t *testing.T) {
	networkPolicy := makeNetworkPolicy("ns-a",
		[]networkingv1.PolicyType{networkingv1.PolicyTypeEgress},
		nil,
		[]networkingv1.NetworkPolicyEgressRule{egressTo(nsSelectorPeer("ns-a"))},
	)
	got := deriveStatusKeys([]networkingv1.NetworkPolicy{networkPolicy})
	if hasStatus(got, models.StatusCrossNamespace) {
		t.Error("unexpected StatusCrossNamespace: namespace selector points to same namespace")
	}
}

// Multiple policies accumulate: policy A locks ingress, policy B locks egress → isolated.
func TestBuildStatusKeys_MultiplePoliciesAccumulate(t *testing.T) {
	ingressPolicy := makeNetworkPolicy("ns-a", []networkingv1.PolicyType{networkingv1.PolicyTypeIngress}, nil, nil)
	egressPolicy := makeNetworkPolicy("ns-a", []networkingv1.PolicyType{networkingv1.PolicyTypeEgress}, nil, nil)
	got := deriveStatusKeys([]networkingv1.NetworkPolicy{ingressPolicy, egressPolicy})
	if !hasStatus(got, models.StatusIsolated) {
		t.Error("expected StatusIsolated when ingress and egress are locked by separate policies")
	}
}

// Isolated + cross-namespace can coexist: locked from internet but allows cluster-internal cross-NS.
func TestBuildStatusKeys_IsolatedAndCrossNS(t *testing.T) {
	networkPolicy := makeNetworkPolicy("ns-a",
		[]networkingv1.PolicyType{networkingv1.PolicyTypeIngress, networkingv1.PolicyTypeEgress},
		nil,
		[]networkingv1.NetworkPolicyEgressRule{egressTo(nsSelectorPeer("ns-b"))},
	)
	got := deriveStatusKeys([]networkingv1.NetworkPolicy{networkPolicy})
	if !hasStatus(got, models.StatusIsolated) {
		t.Error("expected StatusIsolated: no internet paths even with cross-NS egress")
	}
	if !hasStatus(got, models.StatusCrossNamespace) {
		t.Error("expected StatusCrossNamespace alongside StatusIsolated")
	}
}

// ── namespace-access status key tests ────────────────────────────────────────

func emptyPeer() networkingv1.NetworkPolicyPeer {
	return networkingv1.NetworkPolicyPeer{}
}

func sameNsPeer(namespace string) networkingv1.NetworkPolicyPeer {
	return networkingv1.NetworkPolicyPeer{
		NamespaceSelector: &metav1.LabelSelector{
			MatchLabels: map[string]string{"kubernetes.io/metadata.name": namespace},
		},
	}
}

// Nil podSelector + nil namespaceSelector → all pods in same ns → StatusNamespaceEgress.
func TestBuildStatusKeys_InnerNsEgress_NilSelectors(t *testing.T) {
	networkPolicy := makeNetworkPolicy("ns-a",
		[]networkingv1.PolicyType{networkingv1.PolicyTypeEgress},
		nil,
		[]networkingv1.NetworkPolicyEgressRule{egressTo(emptyPeer())},
	)
	got := deriveStatusKeys([]networkingv1.NetworkPolicy{networkPolicy})
	if !hasStatus(got, models.StatusNamespaceEgress) {
		t.Error("expected StatusNamespaceEgress: nil podSelector + nil namespaceSelector = all pods in ns")
	}
	if hasStatus(got, models.StatusNamespaceFull) {
		t.Error("unexpected StatusNamespaceFull: only egress is set")
	}
}

// Nil podSelector + namespaceSelector matching same ns → StatusNamespaceEgress.
func TestBuildStatusKeys_InnerNsEgress_SameNsSelector(t *testing.T) {
	networkPolicy := makeNetworkPolicy("ns-a",
		[]networkingv1.PolicyType{networkingv1.PolicyTypeEgress},
		nil,
		[]networkingv1.NetworkPolicyEgressRule{egressTo(sameNsPeer("ns-a"))},
	)
	got := deriveStatusKeys([]networkingv1.NetworkPolicy{networkPolicy})
	if !hasStatus(got, models.StatusNamespaceEgress) {
		t.Error("expected StatusNamespaceEgress: nil podSelector + same-ns namespaceSelector")
	}
}

// podSelector set → targets specific pods, not all → no StatusNamespaceEgress.
func TestBuildStatusKeys_InnerNsEgress_WithPodSelector_NoNsAccess(t *testing.T) {
	peer := networkingv1.NetworkPolicyPeer{
		PodSelector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "foo"}},
	}
	networkPolicy := makeNetworkPolicy("ns-a",
		[]networkingv1.PolicyType{networkingv1.PolicyTypeEgress},
		nil,
		[]networkingv1.NetworkPolicyEgressRule{egressTo(peer)},
	)
	got := deriveStatusKeys([]networkingv1.NetworkPolicy{networkPolicy})
	if hasStatus(got, models.StatusNamespaceEgress) {
		t.Error("unexpected StatusNamespaceEgress: podSelector is set so not all-pods access")
	}
}

// Nil podSelector + nil namespaceSelector on ingress → StatusNamespaceIngress.
func TestBuildStatusKeys_InnerNsIngress_NilSelectors(t *testing.T) {
	networkPolicy := makeNetworkPolicy("ns-a",
		[]networkingv1.PolicyType{networkingv1.PolicyTypeIngress},
		[]networkingv1.NetworkPolicyIngressRule{ingressFrom(emptyPeer())},
		nil,
	)
	got := deriveStatusKeys([]networkingv1.NetworkPolicy{networkPolicy})
	if !hasStatus(got, models.StatusNamespaceIngress) {
		t.Error("expected StatusNamespaceIngress: nil podSelector + nil namespaceSelector = all pods in ns")
	}
	if hasStatus(got, models.StatusNamespaceFull) {
		t.Error("unexpected StatusNamespaceFull: only ingress is set")
	}
}

// Nil podSelector + namespaceSelector matching same ns on ingress → StatusNamespaceIngress.
func TestBuildStatusKeys_InnerNsIngress_SameNsSelector(t *testing.T) {
	networkPolicy := makeNetworkPolicy("ns-a",
		[]networkingv1.PolicyType{networkingv1.PolicyTypeIngress},
		[]networkingv1.NetworkPolicyIngressRule{ingressFrom(sameNsPeer("ns-a"))},
		nil,
	)
	got := deriveStatusKeys([]networkingv1.NetworkPolicy{networkPolicy})
	if !hasStatus(got, models.StatusNamespaceIngress) {
		t.Error("expected StatusNamespaceIngress: nil podSelector + same-ns namespaceSelector")
	}
}

// podSelector set on ingress peer → targets specific pods, not all → no StatusNamespaceIngress.
func TestBuildStatusKeys_InnerNsIngress_WithPodSelector_NoNsAccess(t *testing.T) {
	peer := networkingv1.NetworkPolicyPeer{
		PodSelector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "foo"}},
	}
	networkPolicy := makeNetworkPolicy("ns-a",
		[]networkingv1.PolicyType{networkingv1.PolicyTypeIngress},
		[]networkingv1.NetworkPolicyIngressRule{ingressFrom(peer)},
		nil,
	)
	got := deriveStatusKeys([]networkingv1.NetworkPolicy{networkPolicy})
	if hasStatus(got, models.StatusNamespaceIngress) {
		t.Error("unexpected StatusNamespaceIngress: podSelector is set so not all-pods access")
	}
}

// Both ingress and egress to/from all same-ns pods → StatusNamespaceFull, not the individual keys.
func TestBuildStatusKeys_InnerNs_FullAccess(t *testing.T) {
	networkPolicy := makeNetworkPolicy("ns-a",
		[]networkingv1.PolicyType{networkingv1.PolicyTypeIngress, networkingv1.PolicyTypeEgress},
		[]networkingv1.NetworkPolicyIngressRule{ingressFrom(emptyPeer())},
		[]networkingv1.NetworkPolicyEgressRule{egressTo(emptyPeer())},
	)
	got := deriveStatusKeys([]networkingv1.NetworkPolicy{networkPolicy})
	if !hasStatus(got, models.StatusNamespaceFull) {
		t.Error("expected StatusNamespaceFull: both ingress and egress to all same-ns pods")
	}
	if hasStatus(got, models.StatusNamespaceEgress) {
		t.Error("unexpected StatusNamespaceEgress: should be collapsed into StatusNamespaceFull")
	}
	if hasStatus(got, models.StatusNamespaceIngress) {
		t.Error("unexpected StatusNamespaceIngress: should be collapsed into StatusNamespaceFull")
	}
}

// BUG FIX: ipBlock-only egress peer (no pod/ns selector) must NOT set StatusNamespaceEgress.
func TestBuildStatusKeys_IPBlockEgress_NoInnerNsAccess(t *testing.T) {
	networkPolicy := makeNetworkPolicy("ns-a",
		[]networkingv1.PolicyType{networkingv1.PolicyTypeEgress},
		nil,
		[]networkingv1.NetworkPolicyEgressRule{egressTo(ipBlockPeer("192.168.1.0/24"))},
	)
	got := deriveStatusKeys([]networkingv1.NetworkPolicy{networkPolicy})
	if hasStatus(got, models.StatusNamespaceEgress) {
		t.Error("unexpected StatusNamespaceEgress: ipBlock peer should not imply intra-namespace access")
	}
}

// BUG FIX: ipBlock-only ingress peer must NOT set StatusNamespaceIngress.
func TestBuildStatusKeys_IPBlockIngress_NoInnerNsAccess(t *testing.T) {
	networkPolicy := makeNetworkPolicy("ns-a",
		[]networkingv1.PolicyType{networkingv1.PolicyTypeIngress},
		[]networkingv1.NetworkPolicyIngressRule{ingressFrom(ipBlockPeer("192.168.1.0/24"))},
		nil,
	)
	got := deriveStatusKeys([]networkingv1.NetworkPolicy{networkPolicy})
	if hasStatus(got, models.StatusNamespaceIngress) {
		t.Error("unexpected StatusNamespaceIngress: ipBlock peer should not imply intra-namespace access")
	}
}

// BUG FIX: empty podSelector struct (non-nil &LabelSelector{}) egress peer must set StatusNamespaceEgress.
func TestBuildStatusKeys_EmptyPodSelectorStructEgress_InnerNsAccess(t *testing.T) {
	peer := networkingv1.NetworkPolicyPeer{PodSelector: &metav1.LabelSelector{}}
	networkPolicy := makeNetworkPolicy("ns-a",
		[]networkingv1.PolicyType{networkingv1.PolicyTypeEgress},
		nil,
		[]networkingv1.NetworkPolicyEgressRule{egressTo(peer)},
	)
	got := deriveStatusKeys([]networkingv1.NetworkPolicy{networkPolicy})
	if !hasStatus(got, models.StatusNamespaceEgress) {
		t.Error("expected StatusNamespaceEgress: empty podSelector struct is catch-all same-ns")
	}
}

// BUG FIX: empty podSelector struct ingress peer must set StatusNamespaceIngress.
func TestBuildStatusKeys_EmptyPodSelectorStructIngress_InnerNsAccess(t *testing.T) {
	peer := networkingv1.NetworkPolicyPeer{PodSelector: &metav1.LabelSelector{}}
	networkPolicy := makeNetworkPolicy("flux-system",
		[]networkingv1.PolicyType{networkingv1.PolicyTypeIngress, networkingv1.PolicyTypeEgress},
		[]networkingv1.NetworkPolicyIngressRule{ingressFrom(peer)},
		nil,
	)
	got := deriveStatusKeys([]networkingv1.NetworkPolicy{networkPolicy})
	if !hasStatus(got, models.StatusNamespaceIngress) {
		t.Error("expected StatusNamespaceIngress: empty podSelector struct is catch-all same-ns")
	}
}

// ── LAN / api-server status key tests ────────────────────────────────────────

// LAN egress only (no ingress LAN) → StatusLanEgress, not StatusLanFull.
func TestBuildStatusKeys_LanEgressOnly(t *testing.T) {
	networkPolicy := makeNetworkPolicy("ns-a",
		[]networkingv1.PolicyType{networkingv1.PolicyTypeIngress, networkingv1.PolicyTypeEgress},
		nil,
		[]networkingv1.NetworkPolicyEgressRule{egressTo(ipBlockPeer("192.168.5.0/24"))},
	)
	got := deriveStatusKeys([]networkingv1.NetworkPolicy{networkPolicy})
	if !hasStatus(got, models.StatusLanEgress) {
		t.Error("expected StatusLanEgress for private CIDR egress")
	}
	if hasStatus(got, models.StatusLanFull) {
		t.Error("unexpected StatusLanFull when only egress is set")
	}
	if hasStatus(got, models.StatusIsolated) {
		t.Error("unexpected StatusIsolated: LAN egress means not air-gapped")
	}
}

// LAN ingress only → StatusLanIngress.
func TestBuildStatusKeys_LanIngressOnly(t *testing.T) {
	networkPolicy := makeNetworkPolicy("ns-a",
		[]networkingv1.PolicyType{networkingv1.PolicyTypeIngress, networkingv1.PolicyTypeEgress},
		[]networkingv1.NetworkPolicyIngressRule{ingressFrom(ipBlockPeer("172.16.0.0/12"))},
		nil,
	)
	got := deriveStatusKeys([]networkingv1.NetworkPolicy{networkPolicy})
	if !hasStatus(got, models.StatusLanIngress) {
		t.Error("expected StatusLanIngress for private CIDR ingress")
	}
	if hasStatus(got, models.StatusIsolated) {
		t.Error("unexpected StatusIsolated: LAN ingress means not air-gapped")
	}
}

// 0.0.0.0/0 is internet only, NOT also LAN.
func TestBuildStatusKeys_CatchAllIsInternetNotLan(t *testing.T) {
	networkPolicy := makeNetworkPolicy("ns-a",
		[]networkingv1.PolicyType{networkingv1.PolicyTypeIngress, networkingv1.PolicyTypeEgress},
		nil,
		[]networkingv1.NetworkPolicyEgressRule{egressTo(ipBlockPeer("0.0.0.0/0"))},
	)
	got := deriveStatusKeys([]networkingv1.NetworkPolicy{networkPolicy})
	if !hasStatus(got, models.StatusInternetEgress) {
		t.Error("expected StatusInternetEgress for 0.0.0.0/0")
	}
	if hasStatus(got, models.StatusLanEgress) || hasStatus(got, models.StatusLanFull) {
		t.Error("unexpected LAN badge for 0.0.0.0/0 — internet bucket only")
	}
}

// namespaceSelector pointing to different ns → cross-NS, NOT inner-ns egress.
func TestBuildStatusKeys_InnerNsEgress_DifferentNs_NoCrossNsEgressAccess(t *testing.T) {
	networkPolicy := makeNetworkPolicy("ns-a",
		[]networkingv1.PolicyType{networkingv1.PolicyTypeEgress},
		nil,
		[]networkingv1.NetworkPolicyEgressRule{egressTo(nsSelectorPeer("ns-b"))},
	)
	got := deriveStatusKeys([]networkingv1.NetworkPolicy{networkPolicy})
	if hasStatus(got, models.StatusNamespaceEgress) {
		t.Error("unexpected StatusNamespaceEgress: namespaceSelector points to different namespace")
	}
	if !hasStatus(got, models.StatusCrossNamespace) {
		t.Error("expected StatusCrossNamespace: namespaceSelector points to different namespace")
	}
}
