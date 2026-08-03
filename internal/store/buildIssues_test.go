package store

import (
	"context"
	"strings"
	"testing"

	"graph/internal/models"
)

// Tests for PolicyIssues — the whole-cluster conflict scan. A "policy conflict"
// is an ALLOW rule whose src→dst path is nevertheless blocked once every
// engine's rules + locks intersect. Reuses the srcID/dstID constants and the
// buildNodeRules helper from buildStore_test.go (same package).

// engineResult builds one engine's EvaluationResult: allow/deny rules grouped
// by ns the way the store walk expects, the NodeRules index reachability reads,
// and the per-direction locks. Mirrors buildCache's engine construction.
func engineResult(allow, deny []models.Rule, srcEgressLocked, dstIngressLocked bool) models.EvaluationResult {
	allowByNs := map[string][]models.Rule{}
	denyByNs := map[string][]models.Rule{}
	for _, rule := range allow {
		if rule.Direction == models.DirectionEgress {
			allowByNs[srcNs] = append(allowByNs[srcNs], rule)
		} else {
			allowByNs[dstNs] = append(allowByNs[dstNs], rule)
		}
	}
	for _, rule := range deny {
		if rule.Direction == models.DirectionEgress {
			denyByNs[srcNs] = append(denyByNs[srcNs], rule)
		} else {
			denyByNs[dstNs] = append(denyByNs[dstNs], rule)
		}
	}
	return models.EvaluationResult{
		AllowByNs: allowByNs,
		DenyByNs:  denyByNs,
		NodeRules: buildNodeRules(append(append([]models.Rule{}, allow...), deny...)),
		PolicyStatuses: map[string]models.PolicyStatus{
			srcID: {EgressLocked: srcEgressLocked},
			dstID: {IngressLocked: dstIngressLocked},
		},
	}
}

// issueCache wires NsIndex with real pod workloads (so WorkloadByID resolves
// src/dst) plus the given engines.
func issueCache(engines map[string]models.EvaluationResult) *models.Cache {
	srcPod := models.WorkloadNode{ID: srcID, Namespace: srcNs, Label: "src-pod"}
	dstPod := models.WorkloadNode{ID: dstID, Namespace: dstNs, Label: "dst-pod"}
	srcNsNode := &models.WorkloadNode{ID: srcNsID, Namespace: srcNs, Type: models.NodeTypeNamespace}
	dstNsNode := &models.WorkloadNode{ID: dstNsID, Namespace: dstNs, Type: models.NodeTypeNamespace}
	cache := &models.Cache{
		NsIndex: map[string]models.NSIndex{
			srcNs: {NSNode: srcNsNode, Workloads: []models.WorkloadNode{srcPod}},
			dstNs: {NSNode: dstNsNode, Workloads: []models.WorkloadNode{dstPod}},
		},
		EvaluationResults: engines,
	}
	cache.RebuildWorkloadIndex()
	return cache
}

func allowBothDirections() []models.Rule {
	return []models.Rule{
		{SrcID: srcID, DstID: dstID, Direction: models.DirectionEgress, Action: models.ActionAllow},
		{SrcID: srcID, DstID: dstID, Direction: models.DirectionIngress, Action: models.ActionAllow},
	}
}

// One engine allows the path while another denies it — the canonical conflict.
// The emitted issue must carry both endpoints so the UI can open reachability.
func TestPolicyIssues_ConflictFlaggedWithEndpoints(t *testing.T) {
	cache := issueCache(map[string]models.EvaluationResult{
		"k8s": engineResult(allowBothDirections(), nil, true, true),
		"istio": engineResult(nil,
			[]models.Rule{{SrcID: srcID, DstID: dstID, Direction: models.DirectionIngress, Action: models.ActionDeny}},
			false, true),
	})

	issues := PolicyIssues(context.Background(), cache, nil)

	if len(issues) != 1 {
		t.Fatalf("issues: want 1 conflict, got %d", len(issues))
	}
	issue := issues[0]
	if issue.Type != models.PolicyConflicts {
		t.Errorf("type: want %q, got %q", models.PolicyConflicts, issue.Type)
	}
	if issue.Src == nil || issue.Src.ID != srcID {
		t.Errorf("src: want endpoint %q, got %+v", srcID, issue.Src)
	}
	if issue.Dst == nil || issue.Dst.ID != dstID {
		t.Errorf("dst: want endpoint %q, got %+v", dstID, issue.Dst)
	}
}

// Allowed and reachable path — no engine blocks it — must produce no issue.
func TestPolicyIssues_NoConflictWhenPathAllowed(t *testing.T) {
	cache := issueCache(map[string]models.EvaluationResult{
		"k8s": engineResult(allowBothDirections(), nil, true, true),
	})

	issues := PolicyIssues(context.Background(), cache, nil)

	if len(issues) != 0 {
		t.Fatalf("issues: want 0 (path permitted), got %d", len(issues))
	}
}

// Multiple allow rules on the same src→dst pair must be deduped to one issue —
// the scan keys by src-dst, not per rule.
func TestPolicyIssues_DedupesSamePair(t *testing.T) {
	allow := append(allowBothDirections(),
		models.Rule{SrcID: srcID, DstID: dstID, Direction: models.DirectionEgress, Action: models.ActionAllow})
	cache := issueCache(map[string]models.EvaluationResult{
		"k8s": engineResult(allow, nil, true, true),
		"istio": engineResult(nil,
			[]models.Rule{{SrcID: srcID, DstID: dstID, Direction: models.DirectionIngress, Action: models.ActionDeny}},
			false, true),
	})

	issues := PolicyIssues(context.Background(), cache, nil)

	if len(issues) != 1 {
		t.Fatalf("issues: want 1 (deduped by src-dst), got %d", len(issues))
	}
}

// Cross-engine peer-kind mismatch: Calico allows egress to the service CIDR
// while the k8s netpol on the same workload names only namespaces. A ClusterIP
// is DNAT'd to a backend pod before k8s policy runs, so k8s can never name that
// range — its silence is overlapping scope, not disagreement. The finding must
// land as partial access, and only for the cluster's own ranges: a WAN CIDR
// destination is something k8s CAN name via ipBlock, so a block there is a real
// conflict.
func TestPolicyIssues_InternalCidrPeerIsPartialNotConflict(t *testing.T) {
	installCIDRConfig(t)

	svcCidrID := models.CIDRIDPrefix + "10.96.0.0/12"
	wanCidrID := models.CIDRIDPrefix + "0.0.0.0/0"
	cidrIssueCache := func(peerID string, cidrType models.CidrType) *models.Cache {
		calico := engineResult([]models.Rule{{
			SrcID: srcID, DstID: peerID, Direction: models.DirectionEgress,
			Action: models.ActionAllow, Coverage: models.CoverageRestricted, AllPorts: true,
			Contributor: models.PolicyRef{Source: "calico", Name: "egress-enroll"},
		}}, nil, false, false)
		calico.Nodes = map[string]models.WorkloadNode{
			peerID: {ID: peerID, Label: peerID, Type: models.NodeTypeCIDR, CidrType: cidrType},
		}
		// k8s: egress locked, allow only to another pod → nothing names the range.
		k8s := engineResult([]models.Rule{{
			SrcID: srcID, DstID: otherDstID, Direction: models.DirectionEgress,
			Action: models.ActionAllow, Coverage: models.CoverageRestricted, AllPorts: true,
			Contributor: models.PolicyRef{Source: "k8s", Name: "internal-traffic", Namespace: srcNs},
		}}, nil, true, false)
		cache := issueCache(map[string]models.EvaluationResult{"calico": calico, "k8s": k8s})
		return cache
	}

	issues := PolicyIssues(context.Background(), cidrIssueCache(svcCidrID, models.CIDRk8sSvc), nil)
	if len(issues) != 1 {
		t.Fatalf("svc CIDR peer: want 1 issue, got %d", len(issues))
	}
	if issues[0].Type != models.IssuesPartial {
		t.Errorf("svc CIDR peer type: want %q (overlapping scope), got %q", models.IssuesPartial, issues[0].Type)
	}

	wanIssues := PolicyIssues(context.Background(), cidrIssueCache(wanCidrID, models.CIDRWan), nil)
	if len(wanIssues) != 1 {
		t.Fatalf("WAN CIDR peer: want 1 issue, got %d", len(wanIssues))
	}
	if wanIssues[0].Type != models.PolicyConflicts {
		t.Errorf("WAN CIDR peer type: want %q (k8s can name it via ipBlock), got %q", models.PolicyConflicts, wanIssues[0].Type)
	}
}

// `allow 0.0.0.0/0 except 10.96.0.0/12` is the standard "internet yes, cluster
// traffic via selectors" egress policy. The except entry is scoping syntax, not
// an access-control statement — there is no deny object, and pod-selector rules
// elsewhere still govern the range — so a Calico allow to it is layering, same
// as a blocker that never mentions the range at all. Both shapes stay partial
// access; neither may present as a conflict telling the operator to remove a
// deny that does not exist.
func TestPolicyIssues_ClusterRangeBlockStaysPartial(t *testing.T) {
	installCIDRConfig(t)

	svcCidrID := models.CIDRIDPrefix + "10.96.0.0/12"
	wanCidrID := models.CIDRIDPrefix + "0.0.0.0/0"

	// k8sDeny is the blocking side's deny rule; calico always allows the range.
	exceptCache := func(k8sDeny models.Rule) *models.Cache {
		calico := engineResult([]models.Rule{{
			SrcID: srcID, DstID: svcCidrID, Direction: models.DirectionEgress,
			Action: models.ActionAllow, Coverage: models.CoverageRestricted, AllPorts: true,
			Contributor: models.PolicyRef{Source: "calico", Name: "egress-enroll"},
		}}, nil, false, false)
		calico.Nodes = map[string]models.WorkloadNode{
			svcCidrID: {ID: svcCidrID, Label: strings.TrimPrefix(svcCidrID, models.CIDRIDPrefix), Type: models.NodeTypeCIDR, CidrType: models.CIDRk8sSvc},
			wanCidrID: {ID: wanCidrID, Label: strings.TrimPrefix(wanCidrID, models.CIDRIDPrefix), Type: models.NodeTypeCIDR, CidrType: models.CIDRWan},
		}
		k8s := engineResult([]models.Rule{{
			SrcID: srcID, DstID: wanCidrID, Direction: models.DirectionEgress,
			Action: models.ActionAllow, Coverage: models.CoverageRestricted, AllPorts: true,
			Contributor: models.PolicyRef{Source: "k8s", Name: "internet-egress", Namespace: srcNs},
		}}, []models.Rule{k8sDeny}, true, false)
		return issueCache(map[string]models.EvaluationResult{"calico": calico, "k8s": k8s})
	}

	// The fixture's 0.0.0.0/0 allow makes a second, unrelated pair reachable-then-
	// blocked; only the service-CIDR pair is under test here.
	svcIssueType := func(issues []models.Issue) models.IssueType {
		for _, issue := range issues {
			if issue.Dst != nil && issue.Dst.ID == svcCidrID {
				return issue.Type
			}
		}
		t.Fatalf("no issue emitted for %s", svcCidrID)
		return ""
	}

	// ipBlock.except carving the service CIDR out of the 0.0.0.0/0 allow.
	excepted := svcIssueType(PolicyIssues(context.Background(), exceptCache(models.Rule{
		SrcID: srcID, DstID: svcCidrID, Direction: models.DirectionEgress,
		Action: models.ActionDeny, Coverage: models.CoverageExcept, AllPorts: true,
		Contributor: models.PolicyRef{Source: "k8s", Name: "internet-egress", Namespace: srcNs},
	}), nil))
	if excepted != models.IssuesPartial {
		t.Errorf("except carve-out type: want %q (scoping syntax, not a deny), got %q", models.IssuesPartial, excepted)
	}

	// Same shape, but the block is a bare default-deny naming no peer — the
	// engine never spoke about this range, so it stays overlapping scope.
	unnamed := svcIssueType(PolicyIssues(context.Background(), exceptCache(models.Rule{
		SrcID: srcID, Direction: models.DirectionEgress,
		Action: models.ActionDeny, Coverage: models.CoverageDenyAll, AllPorts: true,
		Contributor: models.PolicyRef{Source: "k8s", Name: "default-deny", Namespace: srcNs},
	}), nil))
	if unnamed != models.IssuesPartial {
		t.Errorf("unnamed deny type: want %q (never named the range), got %q", models.IssuesPartial, unnamed)
	}
}

// Namespace-node src plus a cluster-range dst hits both classify cases at once.
// The internal-CIDR demotion must win: a CIDR node carries no namespace, so the
// layering pass joins zero pod-granular candidates and classifyConflict would
// report a conflict it never verified. Real shape — a baseline netpol selecting
// every pod in the ns (podSelector {}) compiles its selected side to the ns
// node, names only pod/ns peers, and never mentions the pod CIDR.
func TestPolicyIssues_NsNodeSrcToClusterRangeStaysPartial(t *testing.T) {
	installCIDRConfig(t)

	podCidrID := models.CIDRIDPrefix + "10.42.0.0/16"

	calico := engineResult([]models.Rule{{
		SrcID: srcNsID, DstID: podCidrID, Direction: models.DirectionEgress,
		Action: models.ActionAllow, Coverage: models.CoverageRestricted, AllPorts: true,
		Contributor: models.PolicyRef{Source: "calico", Name: "cluster-egress"},
	}}, nil, false, false)
	calico.Nodes = map[string]models.WorkloadNode{
		podCidrID: {ID: podCidrID, Label: strings.TrimPrefix(podCidrID, models.CIDRIDPrefix), Type: models.NodeTypeCIDR, CidrType: models.CIDRk8sPod},
	}
	// Baseline netpol: egress locked at the ns node, allowed only to a named pod.
	k8s := engineResult([]models.Rule{{
		SrcID: srcNsID, DstID: otherDstID, Direction: models.DirectionEgress,
		Action: models.ActionAllow, Coverage: models.CoverageRestricted, AllPorts: true,
		Contributor: models.PolicyRef{Source: "k8s", Name: "services-network-policies", Namespace: srcNs},
	}}, []models.Rule{{
		SrcID: srcNsID, Direction: models.DirectionEgress,
		Action: models.ActionDeny, Coverage: models.CoverageDenyAll, AllPorts: true,
		Contributor: models.PolicyRef{Source: "k8s", Name: "services-network-policies", Namespace: srcNs},
	}}, false, false)

	cache := issueCache(map[string]models.EvaluationResult{"calico": calico, "k8s": k8s})
	// The real builder appends the synthetic ns node to Workloads; issueCache only
	// parks it on NSNode, so WorkloadByID would never resolve the ns endpoint.
	srcIdx := cache.NsIndex[srcNs]
	srcIdx.Workloads = append(srcIdx.Workloads, *srcIdx.NSNode)
	cache.NsIndex[srcNs] = srcIdx
	cache.RebuildWorkloadIndex()

	issues := PolicyIssues(context.Background(), cache, nil)

	if len(issues) != 1 {
		t.Fatalf("issues: want 1, got %d", len(issues))
	}
	if issues[0].Type != models.IssuesPartial {
		t.Errorf("ns-node src to pod CIDR: want %q (overlapping scope), got %q", models.IssuesPartial, issues[0].Type)
	}
}

// Nested CIDRs: Calico allows the whole 192.168.8.0/24 while the k8s netpol on
// the same workload allows only the 192.168.8.118/32 API-server host inside it.
// The /24 as a whole IS blocked by k8s, so the verdict stays deny — but the
// engines disagree about scope, not intent, and that's its own issue type.
// Guards: an allow OUTSIDE the queried range keeps the plain conflict, and the
// default route can never be "narrowed" since it contains every CIDR.
func TestPolicyIssues_NestedCidrAllowIsScopeMismatch(t *testing.T) {
	installCIDRConfig(t)

	lanCidrID := models.CIDRIDPrefix + "192.168.8.0/24"
	hostCidrID := models.CIDRIDPrefix + "192.168.8.118/32"
	elsewhereCidrID := models.CIDRIDPrefix + "10.20.0.0/16"
	wanCidrID := models.CIDRIDPrefix + "0.0.0.0/0"

	// broadPeer = what the graph node stands for (Calico allows it outright),
	// narrowPeer = the only CIDR k8s names, so k8s blocks the aggregate.
	nestedCache := func(broadPeer string, broadType models.CidrType, narrowPeer string) *models.Cache {
		calico := engineResult([]models.Rule{{
			SrcID: srcID, DstID: broadPeer, Direction: models.DirectionEgress,
			Action: models.ActionAllow, Coverage: models.CoverageRestricted, AllPorts: true,
			Contributor: models.PolicyRef{Source: "calico", Name: "lan-egress"},
		}}, nil, false, false)
		k8s := engineResult([]models.Rule{{
			SrcID: srcID, DstID: narrowPeer, Direction: models.DirectionEgress,
			Action: models.ActionAllow, Coverage: models.CoverageRestricted,
			Ports:       []models.Port{{Port: 6443, Protocol: "TCP"}},
			Contributor: models.PolicyRef{Source: "k8s", Name: "gitlab-k8s-api", Namespace: srcNs},
		}}, nil, true, false)
		calico.Nodes = map[string]models.WorkloadNode{
			broadPeer:  {ID: broadPeer, Label: strings.TrimPrefix(broadPeer, models.CIDRIDPrefix), Type: models.NodeTypeCIDR, CidrType: broadType},
			narrowPeer: {ID: narrowPeer, Label: strings.TrimPrefix(narrowPeer, models.CIDRIDPrefix), Type: models.NodeTypeCIDR, CidrType: models.CIDRLan},
		}
		return issueCache(map[string]models.EvaluationResult{"calico": calico, "k8s": k8s})
	}

	nested := PolicyIssues(context.Background(), nestedCache(lanCidrID, models.CIDRLan, hostCidrID), nil)
	if len(nested) != 1 {
		t.Fatalf("nested CIDR: want 1 issue, got %d", len(nested))
	}
	if nested[0].Type != models.IssuesCidrScope {
		t.Errorf("nested CIDR type: want %q, got %q", models.IssuesCidrScope, nested[0].Type)
	}
	if !strings.Contains(nested[0].Message, "192.168.8.118/32") || !strings.Contains(nested[0].Message, "192.168.8.0/24") {
		t.Errorf("nested CIDR message must name both ranges, got %q", nested[0].Message)
	}
	// The counterpart tier: without the calico ref the row names only the
	// narrowing policy and the operator can't see what it disagrees with.
	foundBroadAllow := false
	for _, allowed := range nested[0].EgressAllowed {
		if allowed.Source == "calico" && allowed.Name == "lan-egress" {
			foundBroadAllow = true
		}
	}
	if !foundBroadAllow {
		t.Errorf("nested CIDR egressAllowed must name the policy allowing the whole range, got %+v", nested[0].EgressAllowed)
	}

	// Disjoint ranges are two unreachable pairs (each engine's own CIDR node),
	// so assert on the types rather than the count.
	for _, issue := range PolicyIssues(context.Background(), nestedCache(lanCidrID, models.CIDRLan, elsewhereCidrID), nil) {
		if issue.Type != models.PolicyConflicts {
			t.Errorf("allow outside the range: want %q, got %q", models.PolicyConflicts, issue.Type)
		}
	}
	for _, issue := range PolicyIssues(context.Background(), nestedCache(wanCidrID, models.CIDRWan, hostCidrID), nil) {
		if issue.Type != models.PolicyConflicts {
			t.Errorf("default route: want %q, got %q", models.PolicyConflicts, issue.Type)
		}
	}
}
