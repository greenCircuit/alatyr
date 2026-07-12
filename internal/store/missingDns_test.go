package store

import (
	"context"
	"testing"

	"graph/internal/config"
	"graph/internal/models"
	"graph/internal/utils"
)

// Tests for MissingDns — flags every workload whose egress is locked down but
// has no allow to the configured DNS target. Reuses helpers/constants from
// reachability_test.go (srcID/srcNs/srcNsID/otherDstID, buildNodeRules,
// egressTo, egressDenyAll) since we're in the same package.

const (
	dnsNs   = "kube-system"
	dnsNsID = "ns/kube-system"
	dnsID   = "kube-system/coredns-abc"
)

var dnsLabels = map[string]string{"k8s-app": "kube-dns"}

// setDnsConfig installs the DNS target the detector reads at call time and
// restores the previous config when the test ends.
func setDnsConfig(t *testing.T) {
	t.Helper()
	prev := config.Get()
	config.Set(config.Config{DNSNamespace: dnsNs, DNSLabels: dnsLabels})
	t.Cleanup(func() { config.Set(prev) })
}

// dnsCache wires src (in srcNs) + coredns (in kube-system) + ns-nodes for
// both, and one engine's NodeRules built from the given src-egress rules.
// dns ns is built with a real LabelIndex so IndexLabelMatch resolves coredns.
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
			engine: {NodeRules: buildNodeRules(srcEgress)},
		},
	}
	cache.RebuildWorkloadIndex()
	return cache
}

// Egress is locked (deny-all marker) with one near-miss allow — DNS never
// permitted, must flag.
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

// Explicit allow to the DNS pod satisfies egress — no issue.
func TestMissingDns_AllowToDNSPod(t *testing.T) {
	setDnsConfig(t)
	cache := dnsCache("k8s", egressDenyAll(), egressTo(dnsID))

	if issues := MissingDns(context.Background(), cache); len(issues) != 0 {
		t.Fatalf("issues: want 0 (dns pod allowed), got %d", len(issues))
	}
}

// Allow that targets the DNS namespace ns-node also satisfies egress.
func TestMissingDns_AllowToDNSNamespace(t *testing.T) {
	setDnsConfig(t)
	cache := dnsCache("k8s", egressDenyAll(), egressTo(dnsNsID))

	if issues := MissingDns(context.Background(), cache); len(issues) != 0 {
		t.Fatalf("issues: want 0 (dns ns allowed), got %d", len(issues))
	}
}

// Src has zero egress rules — NoOpinion — DNS trivially reachable, no issue.
func TestMissingDns_NoEgressLockdown(t *testing.T) {
	setDnsConfig(t)
	cache := dnsCache("k8s")

	if issues := MissingDns(context.Background(), cache); len(issues) != 0 {
		t.Fatalf("issues: want 0 (no egress lockdown), got %d", len(issues))
	}
}

// Explicit deny to the DNS pod — flag.
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

// DNS pod itself must not be flagged — coredns has no reason to egress to
// itself, and would otherwise trigger on every scan.
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

// DNS namespace absent from cache (ns not fetched, misconfig) → no issues,
// don't cascade false positives across the cluster.
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

// Issue must carry src + dns endpoints so the UI opens the reachability panel
// on click, and the engine name so the operator knows which policy to edit.
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
