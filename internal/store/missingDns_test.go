package store

import (
	"context"
	"testing"

	"alatyr/internal/config"
	"alatyr/internal/models"
	"alatyr/internal/utils"
)

// MissingDns tests. Helpers/constants from reachability_test.go (same package).

const (
	dnsNs   = "kube-system"
	dnsNsID = "ns/kube-system"
	dnsID   = "kube-system/coredns-abc"
)

var dnsLabels = map[string]string{"k8s-app": "kube-dns"}

// Sets DNS target config, restores on cleanup.
func setDnsConfig(t *testing.T) {
	t.Helper()
	prev := config.Get()
	config.Set(config.Config{DNSNamespace: dnsNs, DNSLabels: dnsLabels})
	t.Cleanup(func() { config.Set(prev) })
}

// Cache with src + coredns + both ns-nodes. Real LabelIndex so coredns resolves.
func dnsCache(engine string, srcEgress ...models.Rule) *models.Cache {
	srcPod := models.WorkloadNode{ID: srcID, Namespace: srcNs, Label: "src-pod"}
	srcNsNode := &models.WorkloadNode{ID: srcNsID, Namespace: srcNs, Type: models.NodeTypeNamespace}
	dnsNsNode := &models.WorkloadNode{ID: dnsNsID, Namespace: dnsNs, Type: models.NodeTypeNamespace}
	dnsWorkloads := []models.WorkloadNode{
		{ID: dnsID, Namespace: dnsNs, Label: "coredns", Labels: dnsLabels},
	}
	dnsLabelIndex := map[string][]*models.WorkloadNode{}
	for key, value := range dnsLabels {
		dnsLabelIndex[utils.MakeLabelIndexKey(key, value)] = []*models.WorkloadNode{&dnsWorkloads[0]}
	}
	cache := &models.Cache{
		NsIndex: map[string]models.NSIndex{
			srcNs: {NSNode: srcNsNode, Workloads: []models.WorkloadNode{srcPod}},
			dnsNs: {NSNode: dnsNsNode, Workloads: dnsWorkloads, LabelIndex: dnsLabelIndex},
		},
		EvaluationResults: map[string]models.EvaluationResult{
			engine: {NodeRules: buildNodeRules(srcEgress), Nodes: cidrNodesFromRules(srcEgress)},
		},
	}
	cache.RebuildWorkloadIndex()
	return cache
}

// Locked egress, allow to wrong peer — flag.
func TestMissingDns_LockedNoAllowToDNS(t *testing.T) {
	setDnsConfig(t)
	cache := dnsCache("k8s", egressDenyAll(), egressTo(otherDstID))

	issues := MissingDns(context.Background(), cache)

	if len(issues) != 1 {
		t.Fatalf("issues: want 1, got %d", len(issues))
	}
	if issues[0].Type != models.NoDNSEgress {
		t.Errorf("type: want %q, got %q", models.NoDNSEgress, issues[0].Type)
	}
}

// Allow to dns pod — ok.
func TestMissingDns_AllowToDNSPod(t *testing.T) {
	setDnsConfig(t)
	cache := dnsCache("k8s", egressDenyAll(), egressTo(dnsID))

	if issues := MissingDns(context.Background(), cache); len(issues) != 0 {
		t.Fatalf("issues: want 0 (dns pod allowed), got %d", len(issues))
	}
}

// Allow to dns ns-node — ok.
func TestMissingDns_AllowToDNSNamespace(t *testing.T) {
	setDnsConfig(t)
	cache := dnsCache("k8s", egressDenyAll(), egressTo(dnsNsID))

	if issues := MissingDns(context.Background(), cache); len(issues) != 0 {
		t.Fatalf("issues: want 0 (dns ns allowed), got %d", len(issues))
	}
}

// No egress rules — NoOpinion, dns reachable.
func TestMissingDns_NoEgressLockdown(t *testing.T) {
	setDnsConfig(t)
	cache := dnsCache("k8s")

	if issues := MissingDns(context.Background(), cache); len(issues) != 0 {
		t.Fatalf("issues: want 0 (no egress lockdown), got %d", len(issues))
	}
}

// Explicit deny to dns — flag.
func TestMissingDns_ExplicitDenyToDNS(t *testing.T) {
	setDnsConfig(t)
	denyDNS := models.Rule{
		SrcID: srcID, DstID: dnsID,
		Direction: models.DirectionEgress, Action: models.ActionDeny,
		Coverage: models.CoverageRestricted,
	}
	cache := dnsCache("k8s", denyDNS)

	if issues := MissingDns(context.Background(), cache); len(issues) != 1 {
		t.Fatalf("issues: want 1 (explicit deny), got %d", len(issues))
	}
}

// Allow reaches dns; port set decides. Missing 53 flags, culprit = allowing policy.
func TestMissingDns_PortRestrictedAllow(t *testing.T) {
	setDnsConfig(t)
	allowPolicy := models.PolicyRef{Source: "k8s", Name: "allow-dns", Namespace: srcNs}
	toDnsWithPorts := func(ports ...models.Port) models.Rule {
		rule := withPorts(egressTo(dnsID), ports...)
		rule.Contributor = allowPolicy
		return rule
	}
	cases := []struct {
		name       string
		rules      []models.Rule
		wantIssues int
	}{
		{"port 53 allowed", []models.Rule{toDnsWithPorts(udpPort(53))}, 0},
		{"range covering 53", []models.Rule{toDnsWithPorts(models.Port{Port: 50, EndPort: 60, Protocol: "UDP"})}, 0},
		{"portless allow means all ports", []models.Rule{toDnsWithPorts()}, 0},
		{"second rule allows all ports", []models.Rule{toDnsWithPorts(tcpPort(443)), egressAllowAll()}, 0},
		{"only non-dns ports", []models.Rule{toDnsWithPorts(tcpPort(443), tcpPort(8080))}, 1},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			rules := append([]models.Rule{egressDenyAll()}, testCase.rules...)
			cache := dnsCache("k8s", rules...)

			issues := MissingDns(context.Background(), cache)

			if len(issues) != testCase.wantIssues {
				t.Fatalf("issues: want %d, got %d: %+v", testCase.wantIssues, len(issues), issues)
			}
			if testCase.wantIssues == 0 {
				return
			}
			issue := issues[0]
			if issue.EgressReason != ReasonPermitted {
				t.Errorf("reason: want %q (permitted but wrong ports), got %q", ReasonPermitted, issue.EgressReason)
			}
			if len(issue.EgressCulprits) != 1 || issue.EgressCulprits[0].Name != allowPolicy.Name {
				t.Errorf("culprits: want [%s] (the allowing policy), got %+v", allowPolicy.Name, issue.EgressCulprits)
			}
		})
	}
}

// Blanket `to: []` / 0.0.0.0/0 reach dns incidentally. Port 53 decides.
func TestMissingDns_IncidentalReachIsNotAnAllowToDNS(t *testing.T) {
	setDnsConfig(t)
	cases := []struct {
		name       string
		rules      []models.Rule
		wantIssues int
	}{
		{"blanket allow carrying 53", []models.Rule{egressAllowAll(udpPort(53))}, 0},
		{"blanket allow portless", []models.Rule{egressAllowAll()}, 0},
		{"cidr allow carrying 53", []models.Rule{withPorts(egressToCIDR("0.0.0.0/0"), udpPort(53))}, 0},
		{"blanket allow without 53", []models.Rule{egressAllowAll(tcpPort(443))}, 1},
		{"cidr allow without 53", []models.Rule{withPorts(egressToCIDR("0.0.0.0/0"), tcpPort(443))}, 1},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			rules := append([]models.Rule{egressDenyAll()}, testCase.rules...)
			cache := dnsCache("k8s", rules...)

			issues := MissingDns(context.Background(), cache)

			if len(issues) != testCase.wantIssues {
				t.Fatalf("issues: want %d, got %d: %+v", testCase.wantIssues, len(issues), issues)
			}
		})
	}
}

// Ports/AllPorts union across rules — 53 to another peer must not satisfy dns.
func TestMissingDns_PortsNotBorrowedFromOtherPeers(t *testing.T) {
	setDnsConfig(t)
	cases := []struct {
		name  string
		rules []models.Rule
	}{
		{"53 to another peer, 9153 to dns", []models.Rule{
			withPorts(egressTo(otherDstID), udpPort(53)),
			withPorts(egressTo(dnsID), tcpPort(9153)),
		}},
		{"all ports to another peer, 9153 to dns", []models.Rule{
			egressTo(otherDstID),
			withPorts(egressTo(dnsID), tcpPort(9153)),
		}},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			rules := append([]models.Rule{egressDenyAll()}, testCase.rules...)
			cache := dnsCache("k8s", rules...)

			if issues := MissingDns(context.Background(), cache); len(issues) != 1 {
				t.Fatalf("issues: want 1 (dns peer has no 53), got %d: %+v", len(issues), issues)
			}
		})
	}
}

// Catch-all podSelector attaches rules to ns-node — must cover the workload.
func TestMissingDns_NamespaceWideRuleCoversWorkload(t *testing.T) {
	setDnsConfig(t)
	nsWideTo := func(peerID string, ports ...models.Port) models.Rule {
		rule := withPorts(egressTo(peerID), ports...)
		rule.SrcID = srcNsID
		rule.Contributor = models.PolicyRef{Source: "k8s", Name: "ns-wide-egress", Namespace: srcNs}
		return rule
	}
	cases := []struct {
		name       string
		rules      []models.Rule
		wantIssues int
	}{
		{"ns-wide allow to dns on 53", []models.Rule{nsWideTo(dnsID, udpPort(53))}, 0},
		{"ns-wide allow to dns ns node on 53", []models.Rule{nsWideTo(dnsNsID, udpPort(53))}, 0},
		{"ns-wide allow to dns without 53", []models.Rule{nsWideTo(dnsID, tcpPort(9153))}, 1},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			rules := append([]models.Rule{egressDenyAll()}, testCase.rules...)
			cache := dnsCache("k8s", rules...)

			if issues := MissingDns(context.Background(), cache); len(issues) != testCase.wantIssues {
				t.Fatalf("issues: want %d, got %d: %+v", testCase.wantIssues, len(issues), issues)
			}
		})
	}
}

// Dns pod is its own dst — never flag it.
func TestMissingDns_SkipsDNSAsSrc(t *testing.T) {
	setDnsConfig(t)
	dnsNsNode := &models.WorkloadNode{ID: dnsNsID, Namespace: dnsNs, Type: models.NodeTypeNamespace}
	dnsWorkloads := []models.WorkloadNode{
		{ID: dnsID, Namespace: dnsNs, Label: "coredns", Labels: dnsLabels},
	}
	dnsLabelIndex := map[string][]*models.WorkloadNode{}
	for key, value := range dnsLabels {
		dnsLabelIndex[utils.MakeLabelIndexKey(key, value)] = []*models.WorkloadNode{&dnsWorkloads[0]}
	}
	lockdownForDNS := models.Rule{
		SrcID: dnsID, Direction: models.DirectionEgress,
		Coverage:    models.CoverageDenyAll,
		Contributor: models.PolicyRef{Source: "k8s", Name: "dns-default-deny", Namespace: dnsNs},
	}
	cache := &models.Cache{
		NsIndex: map[string]models.NSIndex{
			dnsNs: {NSNode: dnsNsNode, Workloads: dnsWorkloads, LabelIndex: dnsLabelIndex},
		},
		EvaluationResults: map[string]models.EvaluationResult{
			"k8s": {NodeRules: buildNodeRules([]models.Rule{lockdownForDNS})},
		},
	}
	cache.RebuildWorkloadIndex()

	if issues := MissingDns(context.Background(), cache); len(issues) != 0 {
		t.Fatalf("issues: want 0 (dns is its own dst — skip), got %d", len(issues))
	}
}

// Dns ns missing from cache — no issues, no false-positive cascade.
func TestMissingDns_DNSNotInCache(t *testing.T) {
	setDnsConfig(t)
	srcPod := models.WorkloadNode{ID: srcID, Namespace: srcNs, Label: "src-pod"}
	srcNsNode := &models.WorkloadNode{ID: srcNsID, Namespace: srcNs, Type: models.NodeTypeNamespace}
	cache := &models.Cache{
		NsIndex: map[string]models.NSIndex{
			srcNs: {NSNode: srcNsNode, Workloads: []models.WorkloadNode{srcPod}},
		},
		EvaluationResults: map[string]models.EvaluationResult{
			"k8s": {NodeRules: buildNodeRules([]models.Rule{egressDenyAll()})},
		},
	}
	cache.RebuildWorkloadIndex()

	if issues := MissingDns(context.Background(), cache); len(issues) != 0 {
		t.Fatalf("issues: want 0 (dns not in cache), got %d", len(issues))
	}
}

// Issue carries src + dst endpoints and engine name for the UI.
func TestMissingDns_IssueCarriesEndpoints(t *testing.T) {
	setDnsConfig(t)
	cache := dnsCache("k8s", egressDenyAll())

	issues := MissingDns(context.Background(), cache)
	if len(issues) != 1 {
		t.Fatalf("issues: want 1, got %d", len(issues))
	}
	issue := issues[0]
	if issue.Src == nil || issue.Src.ID != srcID {
		t.Errorf("src: want %q, got %+v", srcID, issue.Src)
	}
	if issue.Dst == nil || issue.Dst.ID != dnsID {
		t.Errorf("dst: want dns %q, got %+v", dnsID, issue.Dst)
	}
	if issue.Engine != "k8s" {
		t.Errorf("engine: want %q, got %q", "k8s", issue.Engine)
	}
}

// Short-circuit in MissingDns: blanket allow (no peer named) carrying the port
// means the node reaches DNS. Peer-named rules never count — ports are per-peer.
func TestAllowsFullPortAccess(t *testing.T) {
	cases := []struct {
		name  string
		rules []models.Rule
		want  bool
	}{
		{"blanket allow on 53", []models.Rule{egressAllowAll(udpPort(53))}, true},
		{"blanket range covering 53", []models.Rule{egressAllowAll(models.Port{Port: 50, EndPort: 60, Protocol: "UDP"})}, true},
		{"blanket range below 53", []models.Rule{egressAllowAll(models.Port{Port: 50, EndPort: 52, Protocol: "UDP"})}, false},
		{"blanket on other port only", []models.Rule{egressAllowAll(tcpPort(443))}, false},
		// portless blanket allow means every port, but AllPorts isn't read here —
		// verdict.AllPorts in the permitted branch already skips those nodes
		{"portless blanket allow", []models.Rule{egressAllowAll()}, false},
		// 53 to a named peer must not satisfy DNS for a different peer
		{"53 to a named peer", []models.Rule{withPorts(egressTo(otherDstID), udpPort(53))}, false},
		{"53 named, 443 blanket", []models.Rule{withPorts(egressTo(otherDstID), udpPort(53)), egressAllowAll(tcpPort(443))}, false},
		// markers share the empty DstID but carry no ports
		{"deny-all and unenforced markers", []models.Rule{egressDenyAll(), egressUnenforced()}, false},
		{"no rules at all", nil, false},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			nodeRules := buildNodeRules(testCase.rules)

			if got := allowsFullPortAccess(nodeRules[srcID], 53); got != testCase.want {
				t.Fatalf("want %v, got %v", testCase.want, got)
			}
		})
	}
}

// Engines intersect: calico blanket 53 must not mask a k8s egress lockdown.
func TestMissingDns_OtherEngineBlanketDoesNotMaskBlock(t *testing.T) {
	setDnsConfig(t)
	cache := dnsCache("k8s", egressDenyAll(), egressTo(otherDstID))
	cache.EvaluationResults["calico"] = models.EvaluationResult{
		NodeRules: buildNodeRules([]models.Rule{egressAllowAll(udpPort(53))}),
	}

	issues := MissingDns(context.Background(), cache)

	if len(issues) != 1 {
		t.Fatalf("issues: want 1 (k8s blocks dns), got %d", len(issues))
	}
	if issues[0].Engine != "k8s" {
		t.Errorf("engine: want k8s, got %q", issues[0].Engine)
	}
}
