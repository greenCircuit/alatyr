package store

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"graph/internal/config"
	"graph/internal/models"
	"graph/internal/policy"
)

// Focused tests on IsNodesReachable against the reason-enum model. "Locked" /
// default-deny is no longer a PolicyStatuses bit — it's a CoverageDenyAll marker
// rule sitting in the NodeRules bucket, so fixtures inject rules and assert on
// the per-direction DirectionReason + the deduped Culprits an operator edits.

const (
	srcNs   = "src-ns"
	dstNs   = "dst-ns"
	srcNsID = "ns/src-ns"
	dstNsID = "ns/dst-ns"
	srcID   = "src-ns/src-pod"
	dstID   = "dst-ns/dst-pod"

	// otherDstID is a peer that is neither the queried dst pod nor its ns-node,
	// so an egress allow rule pointing at it is a near-miss (allowed elsewhere).
	otherDstID = "dst-ns/other-pod"
)

// buildCache builds a minimal cache: two NSIndex entries so the ns-node IDs
// resolve, plus one engine whose NodeRules are the given rules bucketed the way
// the engines bucket them (egress by SrcID, ingress by the protected DstID).
func buildCache(engine string, rules ...models.Rule) *models.Cache {
	srcNsNode := &models.WorkloadNode{ID: srcNsID, Namespace: srcNs, Type: models.NodeTypeNamespace}
	dstNsNode := &models.WorkloadNode{ID: dstNsID, Namespace: dstNs, Type: models.NodeTypeNamespace}
	cache := &models.Cache{
		NsIndex: map[string]models.NSIndex{
			srcNs: {NSNode: srcNsNode},
			dstNs: {NSNode: dstNsNode},
		},
		EvaluationResults: map[string]models.EvaluationResult{
			engine: {NodeRules: buildNodeRules(rules), Nodes: cidrNodesFromRules(rules)},
		},
	}
	cache.RebuildWorkloadIndex()
	return cache
}

// cidrNodesFromRules mirrors what k8spolicy.buildRules stamps onto
// EvaluationResult.Nodes for every "cidr:" peer a rule references — reachability
// now reads CidrType off the cached node instead of reparsing the CIDR string,
// so fixtures must register the node the same way the engine does.
func cidrNodesFromRules(rules []models.Rule) map[string]models.WorkloadNode {
	nodes := map[string]models.WorkloadNode{}
	for _, rule := range rules {
		for _, id := range []string{rule.SrcID, rule.DstID} {
			if !strings.HasPrefix(id, models.CIDRIDPrefix) {
				continue
			}
			cidr := strings.TrimPrefix(id, models.CIDRIDPrefix)
			nodes[id] = models.WorkloadNode{
				ID:       id,
				Label:    cidr,
				Type:     models.NodeTypeCIDR,
				CidrType: policy.CidrType(cidr),
			}
		}
	}
	return nodes
}

// buildNodeRules mirrors the engines' per-node index: egress keys by SrcID,
// ingress by DstID (the protected node), split by action. Reachability reads
// this, so fixtures must build it the same way the engines do.
func buildNodeRules(rules []models.Rule) map[string]models.NodeRules {
	buckets := map[string]*models.NodeRules{}
	for _, rule := range rules {
		key := rule.DstID
		if rule.Direction == models.DirectionEgress {
			key = rule.SrcID
		}
		bucket, ok := buckets[key]
		if !ok {
			bucket = &models.NodeRules{}
			buckets[key] = bucket
		}
		direction := &bucket.Ingress
		if rule.Direction == models.DirectionEgress {
			direction = &bucket.Egress
		}
		if rule.Action == models.ActionAllow {
			direction.Allow = append(direction.Allow, rule)
		} else {
			direction.Deny = append(direction.Deny, rule)
		}
	}
	out := make(map[string]models.NodeRules, len(buckets))
	for nodeID, bucket := range buckets {
		out[nodeID] = *bucket
	}
	return out
}

// Rule constructors — restricted (real) allows/denies and the two structural
// markers, shaped the way each engine emits them.

// Engines stamp AllPorts=true on portless rules, so the constructors mirror
// that; withPorts flips it off when ports are given.
func egressTo(peerID string) models.Rule {
	return models.Rule{
		SrcID: srcID, DstID: peerID,
		Direction: models.DirectionEgress, Action: models.ActionAllow,
		Coverage: models.CoverageRestricted, AllPorts: true,
	}
}

func ingressFrom(peerID string) models.Rule {
	return models.Rule{
		SrcID: peerID, DstID: dstID,
		Direction: models.DirectionIngress, Action: models.ActionAllow,
		Coverage: models.CoverageRestricted, AllPorts: true,
	}
}

func ingressDenyFrom(peerID string) models.Rule {
	rule := ingressFrom(peerID)
	rule.Action = models.ActionDeny
	return rule
}

// egressDenyAll / ingressDenyAll are the locked-empty markers: empty peer,
// CoverageDenyAll, Action defaults to allow so they land in the Allow bucket.
func egressDenyAll() models.Rule {
	return models.Rule{
		SrcID: srcID, Direction: models.DirectionEgress,
		Coverage:    models.CoverageDenyAll,
		Contributor: models.PolicyRef{Source: "k8s", Name: "src-default-deny", Namespace: srcNs},
	}
}

func ingressDenyAll() models.Rule {
	return models.Rule{
		DstID: dstID, Direction: models.DirectionIngress,
		Coverage:    models.CoverageDenyAll,
		Contributor: models.PolicyRef{Source: "k8s", Name: "dst-default-deny", Namespace: dstNs},
	}
}

// egressAllowAll / ingressAllowAll are the empty-peer-list stanzas: allow any
// destination/source, CoverageAllowAll, empty peer ID. AllPorts mirrors the
// engines: no ports on the stanza = every port.
func egressAllowAll(ports ...models.Port) models.Rule {
	return models.Rule{
		SrcID: srcID, Direction: models.DirectionEgress, Action: models.ActionAllow,
		Coverage: models.CoverageAllowAll, Ports: ports, AllPorts: len(ports) == 0,
	}
}

func ingressAllowAll(ports ...models.Port) models.Rule {
	return models.Rule{
		DstID: dstID, Direction: models.DirectionIngress, Action: models.ActionAllow,
		Coverage: models.CoverageAllowAll, Ports: ports, AllPorts: len(ports) == 0,
	}
}

// egressUnenforced is the marker for a policy that names egress but doesn't
// constrain it — like allow-all and deny-all markers it has an empty DstID.
func egressUnenforced() models.Rule {
	return models.Rule{
		SrcID: srcID, Direction: models.DirectionEgress,
		Coverage: models.CoverageUnenforced,
	}
}

func withPorts(rule models.Rule, ports ...models.Port) models.Rule {
	rule.Ports = ports
	rule.AllPorts = len(ports) == 0
	return rule
}

func tcpPort(number int) models.Port { return models.Port{Port: number, Protocol: "TCP"} }
func udpPort(number int) models.Port { return models.Port{Port: number, Protocol: "UDP"} }

// egressToCIDR mirrors egressTo but stamps the synthetic CIDR-peer DstID that
// k8spolicy.expandPeerRules emits for ipBlock peers (`"cidr:" + <cidr>`). The
// rule stays Coverage=CoverageRestricted just like real ipBlock rules — the
// engine does not stamp CoverageAllowAll even when the CIDR is 0.0.0.0/0.
func egressToCIDR(cidr string) models.Rule {
	return models.Rule{
		SrcID: srcID, DstID: "cidr:" + cidr,
		Direction: models.DirectionEgress, Action: models.ActionAllow,
		Coverage: models.CoverageRestricted, AllPorts: true,
		Contributor: models.PolicyRef{Source: "k8s", Name: "egress-cidr", Namespace: srcNs},
	}
}

func ingressFromCIDR(cidr string) models.Rule {
	return models.Rule{
		SrcID: "cidr:" + cidr, DstID: dstID,
		Direction: models.DirectionIngress, Action: models.ActionAllow,
		Coverage: models.CoverageRestricted, AllPorts: true,
		Contributor: models.PolicyRef{Source: "k8s", Name: "ingress-cidr", Namespace: dstNs},
	}
}

// installCIDRConfig pins pod + service CIDRs for CIDR-classification tests and
// restores the prior config on teardown so package-level state does not leak.
func installCIDRConfig(t *testing.T) {
	t.Helper()
	prev := config.Get()
	config.Set(config.Config{
		PodCIDR: "10.244.0.0/16",
		SvcCIDR: "10.96.0.0/12",
	})
	t.Cleanup(func() { config.Set(prev) })
}

// ingressCatchAllDeny is how Istio emits action:DENY rules:[{}] — a blanket
// deny of every source. Unlike ingressDenyAll (k8s lock, Action defaults to
// allow → Allow bucket), this carries Action=Deny so it lands in the Deny
// bucket, and SrcID is empty because it denies any source, not a named peer.
func ingressCatchAllDeny() models.Rule {
	return models.Rule{
		DstID: dstID, Direction: models.DirectionIngress,
		Action: models.ActionDeny, Coverage: models.CoverageDenyAll,
		Contributor: models.PolicyRef{Source: "istio", Name: "deny-all-backend", Namespace: dstNs},
	}
}

// decideDirectionVerdict is pure precedence over the sorted slices — test it
// directly so a precedence regression names this func, not the whole pass.
// Key case: an explicit deny must beat a coexisting allow (Istio DENY > ALLOW),
// which only holds if DenyMatches is checked before AllowMatches.
func TestDecideDirectionVerdict_Precedence(t *testing.T) {
	// decideDirectionVerdict operates on converted NodeRules; an empty index
	// leaves endpoints unresolved, which precedence doesn't care about.
	asNodeRules := func(rules ...models.Rule) []models.NodeRule {
		var converted []models.NodeRule
		for _, rule := range rules {
			converted = append(converted, toNodeRule(rule, nil))
		}
		return converted
	}
	denyRule := asNodeRules(ingressCatchAllDeny())
	allowRule := asNodeRules(ingressFrom(srcID))
	nearMiss := asNodeRules(egressTo(otherDstID))

	cases := []struct {
		name    string
		verdict DirectionVerdict
		want    DirectionReason
	}{
		{"explicit deny beats allow", DirectionVerdict{
			DenyMatches: denyRule, AllowMatches: allowRule,
		}, ReasonExplicitDeny},
		{"allow beats locked-no-match", DirectionVerdict{
			AllowMatches: allowRule, OtherAllowMatches: nearMiss,
		}, ReasonPermitted},
		{"locked-no-match beats default-deny", DirectionVerdict{
			OtherAllowMatches: nearMiss, DenyAllMatches: asNodeRules(ingressDenyAll()),
		}, ReasonLockedNoMatch},
		{"default-deny alone", DirectionVerdict{
			DenyAllMatches: asNodeRules(ingressDenyAll()),
		}, ReasonDefaultDeny},
		{"nothing governs", DirectionVerdict{}, ReasonNoOpinion},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			got, _ := decideDirectionVerdict(testCase.verdict)
			if got != testCase.want {
				t.Errorf("reason: want %q, got %q", testCase.want, got)
			}
		})
	}
}

// collectFromSide is the ingress sorter. A blanket Istio deny (Deny bucket,
// empty SrcID) must sort into DenyMatches regardless of who the source is —
// this is the exact seam the reachability bug fell through.
func TestCollectFromSide_BlanketDenyMatchesAnySource(t *testing.T) {
	dstBucket := buildNodeRules([]models.Rule{ingressCatchAllDeny()})[dstID]

	verdict := collectFromSide(models.NodeRules{}, dstBucket, srcNsID, srcID, nil)

	if verdict.Reason != ReasonExplicitDeny {
		t.Errorf("reason: want %q, got %q", ReasonExplicitDeny, verdict.Reason)
	}
	if len(verdict.DenyMatches) != 1 {
		t.Fatalf("DenyMatches: want 1 (blanket deny credited), got %d", len(verdict.DenyMatches))
	}
}

// Verdict-level Ports/AllPorts exist so the UI can diff egress vs ingress port
// sets without re-walking AllowMatches. One broad test per side: dedup across
// the ns-node + pod buckets, allow-all stanzas matching any peer and feeding
// ports, AllPorts propagation, near-miss ports excluded, and the unenforced
// marker (empty peer ID, like allow-all) not swallowed as an allow match.
func TestCollectSides_PortsAndAllowAll(t *testing.T) {
	assertPorts := func(t *testing.T, got []models.Port, want ...models.Port) {
		t.Helper()
		if len(got) != len(want) {
			t.Fatalf("ports: want %d unique, got %d: %+v", len(want), len(got), got)
		}
		gotSet := map[string]bool{}
		for _, port := range got {
			gotSet[fmt.Sprintf("%d/%d/%s", port.Port, port.EndPort, port.Protocol)] = true
		}
		for _, port := range want {
			if !gotSet[fmt.Sprintf("%d/%d/%s", port.Port, port.EndPort, port.Protocol)] {
				t.Errorf("ports: missing %s/%d in %+v", port.Protocol, port.Port, got)
			}
		}
	}

	t.Run("egress aggregates deduped ports from matching rules only", func(t *testing.T) {
		nsRule := withPorts(egressTo(dstID), tcpPort(443), tcpPort(8080))
		nsRule.SrcID = srcNsID
		nsBucket := buildNodeRules([]models.Rule{nsRule})[srcNsID]
		podBucket := buildNodeRules([]models.Rule{
			withPorts(egressTo(dstID), tcpPort(80), tcpPort(443)), // 443 dupes the ns bucket
			withPorts(egressTo(otherDstID), tcpPort(9999)),        // near miss — ports must not leak in
			egressAllowAll(udpPort(53), tcpPort(53)),              // matches any peer; UDP/TCP 53 stay distinct
		})[srcID]

		verdict := collectToSide(nsBucket, podBucket, dstNsID, dstID, nil)

		if verdict.Reason != ReasonPermitted {
			t.Fatalf("reason: want %q, got %q", ReasonPermitted, verdict.Reason)
		}
		if len(verdict.AllowMatches) != 3 {
			t.Errorf("AllowMatches: want 3 (ns + pod + allow-all), got %d", len(verdict.AllowMatches))
		}
		if verdict.AllPorts {
			t.Errorf("AllPorts: want false, every matching rule names ports")
		}
		assertPorts(t, verdict.Ports, tcpPort(80), tcpPort(443), tcpPort(8080), udpPort(53), tcpPort(53))
	})

	t.Run("egress allow-all without ports sets AllPorts", func(t *testing.T) {
		podBucket := buildNodeRules([]models.Rule{egressAllowAll()})[srcID]

		verdict := collectToSide(models.NodeRules{}, podBucket, dstNsID, dstID, nil)

		if verdict.Reason != ReasonPermitted {
			t.Fatalf("reason: want %q, got %q", ReasonPermitted, verdict.Reason)
		}
		if !verdict.AllPorts {
			t.Errorf("AllPorts: want true for portless allow-all")
		}
		assertPorts(t, verdict.Ports)
	})

	t.Run("egress unenforced marker is not an allow-all match", func(t *testing.T) {
		podBucket := buildNodeRules([]models.Rule{egressUnenforced()})[srcID]

		verdict := collectToSide(models.NodeRules{}, podBucket, dstNsID, dstID, nil)

		if verdict.Reason != ReasonNoOpinion {
			t.Errorf("reason: want %q, got %q", ReasonNoOpinion, verdict.Reason)
		}
		if len(verdict.AllowMatches) != 0 {
			t.Errorf("AllowMatches: want 0, unenforced marker leaked in: %+v", verdict.AllowMatches)
		}
	})

	t.Run("ingress mirrors ports, allow-all and AllPorts", func(t *testing.T) {
		nsRule := withPorts(ingressFrom(srcID), tcpPort(80), tcpPort(8443))
		nsRule.DstID = dstNsID
		nsBucket := buildNodeRules([]models.Rule{nsRule})[dstNsID]
		podBucket := buildNodeRules([]models.Rule{
			withPorts(ingressFrom(srcID), tcpPort(80)), // dupes the ns bucket
			ingressAllowAll(tcpPort(443)),
			ingressAllowAll(), // portless allow-all flips AllPorts
		})[dstID]

		verdict := collectFromSide(nsBucket, podBucket, srcNsID, srcID, nil)

		if verdict.Reason != ReasonPermitted {
			t.Fatalf("reason: want %q, got %q", ReasonPermitted, verdict.Reason)
		}
		if !verdict.AllPorts {
			t.Errorf("AllPorts: want true, portless allow-all present")
		}
		assertPorts(t, verdict.Ports, tcpPort(80), tcpPort(8443), tcpPort(443))
	})
}

func TestIsNodesReachable_DirectionReasons(t *testing.T) {
	cases := []struct {
		name        string
		rules       []models.Rule
		wantVerdict string
		wantStatus  string
		wantEgress  DirectionReason
		wantIngress DirectionReason
	}{
		{
			name:        "allow both sides",
			rules:       []models.Rule{egressTo(dstID), ingressFrom(srcID)},
			wantVerdict: "allow", wantStatus: "allow",
			wantEgress: ReasonPermitted, wantIngress: ReasonPermitted,
		},
		{
			name:        "no policy either side is not enforced",
			rules:       nil,
			wantVerdict: "allow", wantStatus: "not enforced",
			wantEgress: ReasonNoOpinion, wantIngress: ReasonNoOpinion,
		},
		{
			name:        "ingress explicit deny wins over allow",
			rules:       []models.Rule{ingressFrom(srcID), ingressDenyFrom(srcID)},
			wantVerdict: "deny", wantStatus: "deny",
			wantEgress: ReasonNoOpinion, wantIngress: ReasonExplicitDeny,
		},
		{
			name:        "ingress default-deny marker blocks",
			rules:       []models.Rule{ingressDenyAll()},
			wantVerdict: "deny", wantStatus: "deny",
			wantEgress: ReasonNoOpinion, wantIngress: ReasonDefaultDeny,
		},
		{
			name:        "egress allowed elsewhere is locked-no-match not default-deny",
			rules:       []models.Rule{egressTo(otherDstID)},
			wantVerdict: "deny", wantStatus: "deny",
			wantEgress: ReasonLockedNoMatch, wantIngress: ReasonNoOpinion,
		},
		{
			name:        "egress default-deny marker blocks",
			rules:       []models.Rule{egressDenyAll()},
			wantVerdict: "deny", wantStatus: "deny",
			wantEgress: ReasonDefaultDeny, wantIngress: ReasonNoOpinion,
		},
		{
			// Istio action:DENY rules:[{}] — blanket deny in the Deny bucket with
			// empty SrcID. Must block regardless of who the source is.
			name:        "ingress catch-all deny (istio blanket) blocks any source",
			rules:       []models.Rule{ingressCatchAllDeny()},
			wantVerdict: "deny", wantStatus: "deny",
			wantEgress: ReasonNoOpinion, wantIngress: ReasonExplicitDeny,
		},
		{
			name:        "egress ns-node matcher permits pod to pod",
			rules:       []models.Rule{egressTo(dstNsID), ingressFrom(srcID)},
			wantVerdict: "allow", wantStatus: "allow",
			wantEgress: ReasonPermitted, wantIngress: ReasonPermitted,
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			cache := buildCache("k8s", testCase.rules...)
			got := PoliciesCanReach(context.Background(), cache, srcID, srcNs, dstID, dstNs)

			if got.Verdict != testCase.wantVerdict {
				t.Errorf("verdict: want %q, got %q (reason=%q)", testCase.wantVerdict, got.Verdict, got.Reason)
			}
			engine := got.Engines["k8s"]
			if engine.Status != testCase.wantStatus {
				t.Errorf("status: want %q, got %q", testCase.wantStatus, engine.Status)
			}
			if engine.Egress.Reason != testCase.wantEgress {
				t.Errorf("egress reason: want %q, got %q", testCase.wantEgress, engine.Egress.Reason)
			}
			if engine.Ingress.Reason != testCase.wantIngress {
				t.Errorf("ingress reason: want %q, got %q", testCase.wantIngress, engine.Ingress.Reason)
			}
		})
	}
}

func TestIsNodesReachable_CulpritsDedupToPolicy(t *testing.T) {
	// Two egress allow rules from the SAME policy, both pointing elsewhere. The
	// near-miss culprits must collapse to one policy.
	policy := models.PolicyRef{Source: "k8s", Name: "web-netpol", Namespace: srcNs}
	firstRule := egressTo(otherDstID)
	firstRule.Contributor = policy
	secondRule := egressTo("dst-ns/third-pod")
	secondRule.Contributor = policy

	cache := buildCache("k8s", firstRule, secondRule)
	got := PoliciesCanReach(context.Background(), cache, srcID, srcNs, dstID, dstNs)

	egress := got.Engines["k8s"].Egress
	if egress.Reason != ReasonLockedNoMatch {
		t.Fatalf("egress reason: want locked-no-match, got %q", egress.Reason)
	}
	if len(egress.Culprits) != 1 {
		t.Fatalf("culprits: want 1 (deduped by policy), got %d: %+v", len(egress.Culprits), egress.Culprits)
	}
	if egress.Culprits[0].Name != "web-netpol" || egress.Culprits[0].Namespace != srcNs {
		t.Errorf("culprit: want %s/web-netpol, got %s/%s", srcNs, egress.Culprits[0].Namespace, egress.Culprits[0].Name)
	}
}

func TestIsNodesReachable_AllowOnOneEngineDeniedOnAnotherBlocks(t *testing.T) {
	// Multi-engine AND: k8s permits both directions, istio locks ingress with a
	// blanket action:DENY (empty rule, any source) → overall deny, with each
	// engine's own status preserved. Uses the real Istio catch-all shape, not a
	// src-specific deny, so it exercises the Deny-bucket + empty-SrcID seam.
	cache := buildCache("k8s", egressTo(dstID), ingressFrom(srcID))
	cache.EvaluationResults["istio"] = models.EvaluationResult{
		NodeRules: buildNodeRules([]models.Rule{ingressCatchAllDeny()}),
	}
	got := PoliciesCanReach(context.Background(), cache, srcID, srcNs, dstID, dstNs)

	if got.Verdict != "deny" {
		t.Fatalf("verdict: want deny (istio blocks), got %q", got.Verdict)
	}
	if got.Engines["k8s"].Status != "allow" {
		t.Errorf("k8s engine: want allow, got %q", got.Engines["k8s"].Status)
	}
	if got.Engines["istio"].Status != "deny" {
		t.Errorf("istio engine: want deny, got %q", got.Engines["istio"].Status)
	}
}

// TestIsNodesReachable_CIDRPeerCoversInternalWorkload documents the bug where
// collectToSide / collectFromSide compare rule.DstID / rule.SrcID via string
// equality against a workload node ID and never consult whether an ipBlock
// peer CIDR would actually cover the counterpart pod's IP space. k8s
// NetworkPolicy semantics match ipBlocks against real pod IPs at runtime, so
// an egress allow to 0.0.0.0/0 or to the cluster pod CIDR permits traffic to
// every internal pod — reachability must reflect that.
//
// The engine emits these rules with DstID="cidr:<cidr>" and
// Coverage=CoverageRestricted (buildRules.go expandPeerRules); reachability
// currently sinks them into OtherAllowMatches → ReasonLockedNoMatch → deny.
// The three "does not cover" cases lock in the correct near-miss behavior so
// the fix does not over-broaden to arbitrary external CIDRs.
func TestIsNodesReachable_CIDRPeerCoversInternalWorkload(t *testing.T) {
	installCIDRConfig(t)

	cases := []struct {
		name        string
		rules       []models.Rule
		wantVerdict string
		wantStatus  string
		wantEgress  DirectionReason
		wantIngress DirectionReason
	}{
		{
			// 0.0.0.0/0 as an egress peer matches every IP, including internal
			// pod IPs. Current bug: DstID="cidr:0.0.0.0/0" != dstID and Coverage
			// is CoverageRestricted (not CoverageAllowAll), so the rule falls
			// into OtherAllowMatches and the verdict is deny.
			name:        "egress 0.0.0.0/0 covers any internal dst pod",
			rules:       []models.Rule{egressToCIDR("0.0.0.0/0")},
			wantVerdict: "allow", wantStatus: "allow",
			wantEgress: ReasonPermitted, wantIngress: ReasonNoOpinion,
		},
		{
			// Cluster pod CIDR covers every pod IP. Common pattern: allow
			// egress to the pod CIDR to permit intra-cluster east-west.
			name:        "egress pod CIDR covers internal dst pod",
			rules:       []models.Rule{egressToCIDR("10.244.0.0/16")},
			wantVerdict: "allow", wantStatus: "allow",
			wantEgress: ReasonPermitted, wantIngress: ReasonNoOpinion,
		},
		{
			// Service CIDR — clients hit a ClusterIP which is in the svc range.
			// Egress-side check on the destination workload treats coverage of
			// the svc CIDR the same as covering the workload it fronts.
			name:        "egress svc CIDR covers internal dst pod",
			rules:       []models.Rule{egressToCIDR("10.96.0.0/12")},
			wantVerdict: "allow", wantStatus: "allow",
			wantEgress: ReasonPermitted, wantIngress: ReasonNoOpinion,
		},
		{
			// Public external CIDR that does not overlap pod / svc CIDR. Rule
			// is a genuine near-miss for an internal pod destination — verdict
			// stays deny with locked-no-match. This is already the current
			// behavior; the case is here so a fix does not overshoot.
			name:        "egress unrelated external CIDR does not cover internal dst pod",
			rules:       []models.Rule{egressToCIDR("203.0.113.0/24")},
			wantVerdict: "deny", wantStatus: "deny",
			wantEgress: ReasonLockedNoMatch, wantIngress: ReasonNoOpinion,
		},
		{
			// Mirror on ingress: from ipBlock 0.0.0.0/0 admits any source, so
			// an internal src pod must count. Same bug on collectFromSide.
			name:        "ingress 0.0.0.0/0 admits any internal src pod",
			rules:       []models.Rule{ingressFromCIDR("0.0.0.0/0")},
			wantVerdict: "allow", wantStatus: "allow",
			wantEgress: ReasonNoOpinion, wantIngress: ReasonPermitted,
		},
		{
			name:        "ingress pod CIDR admits internal src pod",
			rules:       []models.Rule{ingressFromCIDR("10.244.0.0/16")},
			wantVerdict: "allow", wantStatus: "allow",
			wantEgress: ReasonNoOpinion, wantIngress: ReasonPermitted,
		},
		{
			name:        "ingress unrelated external CIDR does not admit internal src pod",
			rules:       []models.Rule{ingressFromCIDR("203.0.113.0/24")},
			wantVerdict: "deny", wantStatus: "deny",
			wantEgress: ReasonNoOpinion, wantIngress: ReasonLockedNoMatch,
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			cache := buildCache("k8s", testCase.rules...)
			got := PoliciesCanReach(context.Background(), cache, srcID, srcNs, dstID, dstNs)

			if got.Verdict != testCase.wantVerdict {
				t.Errorf("verdict: want %q, got %q (reason=%q)", testCase.wantVerdict, got.Verdict, got.Reason)
			}
			engine := got.Engines["k8s"]
			if engine.Status != testCase.wantStatus {
				t.Errorf("status: want %q, got %q", testCase.wantStatus, engine.Status)
			}
			if engine.Egress.Reason != testCase.wantEgress {
				t.Errorf("egress reason: want %q, got %q", testCase.wantEgress, engine.Egress.Reason)
			}
			if engine.Ingress.Reason != testCase.wantIngress {
				t.Errorf("ingress reason: want %q, got %q", testCase.wantIngress, engine.Ingress.Reason)
			}
		})
	}
}

// TestIsNodesReachable_CIDRExceptCarveOutOverridesCovering documents the
// second-order bug: an egress allow to 0.0.0.0/0 with an ipBlock.except
// covering the pod CIDR must NOT reach an internal pod. k8spolicy emits the
// except carve-out as an ActionDeny rule with Coverage=CoverageExcept and
// DstID="cidr:<exceptCIDR>". Current reachability only puts a rule in the
// deny bucket when DstID matches the queried peer or Coverage=DenyAll, so
// the except carve-out is silently dropped and the covering allow wins even
// though the operator explicitly excluded internal traffic.
func TestIsNodesReachable_CIDRExceptCarveOutOverridesCovering(t *testing.T) {
	installCIDRConfig(t)

	policy := models.PolicyRef{Source: "k8s", Name: "egress-external-only", Namespace: srcNs}
	allow := egressToCIDR("0.0.0.0/0")
	allow.Contributor = policy
	except := models.Rule{
		SrcID: srcID, DstID: "cidr:10.244.0.0/16",
		Direction: models.DirectionEgress, Action: models.ActionDeny,
		Coverage: models.CoverageExcept, AllPorts: true,
		Contributor: policy,
	}

	cache := buildCache("k8s", allow, except, ingressFrom(srcID))
	got := PoliciesCanReach(context.Background(), cache, srcID, srcNs, dstID, dstNs)

	if got.Verdict != "deny" {
		t.Errorf("verdict: want deny (except carves out pod CIDR), got %q (reason=%q)", got.Verdict, got.Reason)
	}
	egress := got.Engines["k8s"].Egress
	// Carved-out, not explicit-deny: the operator wrote an except list, not a
	// deny object, so the reason must not send them hunting for one to delete.
	if egress.Reason != ReasonCarvedOut {
		t.Errorf("egress reason: want %q (except list, no deny object), got %q", ReasonCarvedOut, egress.Reason)
	}
	if len(egress.DenyMatches) != 0 {
		t.Errorf("egress deny matches: want 0 (carve-outs are not denies), got %d", len(egress.DenyMatches))
	}
}

// TestIsNodesReachable_AllowInternetExceptPodAndSvcCIDR mirrors the common
// "egress-to-internet-only" pattern: allow 0.0.0.0/0 with ipBlock.except
// entries for both the pod CIDR and the service CIDR. Reachability between
// two internal pods must resolve to deny — the covering allow is fully
// carved out for internal traffic by the two except rules. The engine emits
// each except as ActionDeny + Coverage=CoverageExcept + DstID="cidr:<except>"
// (buildRules.go expandPeerRules), and both must land in CarveOutMatches so
// decideDirectionVerdict picks ReasonCarvedOut over the ReasonPermitted the
// covering allow would otherwise produce.
func TestIsNodesReachable_AllowInternetExceptPodAndSvcCIDR(t *testing.T) {
	installCIDRConfig(t)

	policy := models.PolicyRef{Source: "k8s", Name: "egress-external-only", Namespace: srcNs}
	allow := egressToCIDR("0.0.0.0/0")
	allow.Contributor = policy
	exceptPod := models.Rule{
		SrcID: srcID, DstID: "cidr:10.244.0.0/16",
		Direction: models.DirectionEgress, Action: models.ActionDeny,
		Coverage: models.CoverageExcept, AllPorts: true,
		Contributor: policy,
	}
	exceptSvc := models.Rule{
		SrcID: srcID, DstID: "cidr:10.96.0.0/12",
		Direction: models.DirectionEgress, Action: models.ActionDeny,
		Coverage: models.CoverageExcept, AllPorts: true,
		Contributor: policy,
	}

	cache := buildCache("k8s", allow, exceptPod, exceptSvc, ingressFrom(srcID))
	got := PoliciesCanReach(context.Background(), cache, srcID, srcNs, dstID, dstNs)

	if got.Verdict != "deny" {
		t.Errorf("verdict: want deny (except carves out pod+svc CIDR), got %q (reason=%q)", got.Verdict, got.Reason)
	}
	engine := got.Engines["k8s"]
	if engine.Egress.Reason != ReasonCarvedOut {
		t.Errorf("egress reason: want %q, got %q", ReasonCarvedOut, engine.Egress.Reason)
	}
	if len(engine.Egress.CarveOutMatches) != 2 {
		t.Errorf("egress carve-out matches: want 2 (pod + svc except), got %d", len(engine.Egress.CarveOutMatches))
	}
}

// TestIsNodesReachable_EgressToWanNodeNotBlockedByInternalExcept pins the
// reported kas→0.0.0.0/0 case: the queried DST is the WAN CIDR node itself,
// not an internal pod. The egress policy allows 0.0.0.0/0 except the pod and
// svc CIDRs. Those except carve-outs only remove internal sub-ranges — they do
// NOT contain the 0.0.0.0/0 node, so reaching the WAN node must stay allowed.
//
// Bug: collectToSide's deny loop promotes any except rule via
// isClusterCoveringCidr(rule.DstID) — a dst-independent check. It fires for the
// pod/svc except CIDRs regardless of what dst is queried, so the WAN dst gets a
// spurious ReasonExplicitDeny even though no except range covers it.
func TestIsNodesReachable_EgressToWanNodeNotBlockedByInternalExcept(t *testing.T) {
	installCIDRConfig(t)

	wanDstID := models.CIDRIDPrefix + "0.0.0.0/0"
	policy := models.PolicyRef{Source: "k8s", Name: "kas-external-access", Namespace: srcNs}
	allow := egressToCIDR("0.0.0.0/0")
	allow.Contributor = policy
	exceptPod := models.Rule{
		SrcID: srcID, DstID: models.CIDRIDPrefix + "10.244.0.0/16",
		Direction: models.DirectionEgress, Action: models.ActionDeny,
		Coverage: models.CoverageExcept, AllPorts: true,
		Contributor: policy,
	}
	exceptSvc := models.Rule{
		SrcID: srcID, DstID: models.CIDRIDPrefix + "10.96.0.0/12",
		Direction: models.DirectionEgress, Action: models.ActionDeny,
		Coverage: models.CoverageExcept, AllPorts: true,
		Contributor: policy,
	}

	cache := buildCache("k8s", allow, exceptPod, exceptSvc)
	// DST is the external WAN node (ns=="") — not an internal pod.
	got := PoliciesCanReach(context.Background(), cache, srcID, srcNs, wanDstID, "")

	if got.Verdict != "allow" {
		t.Errorf("verdict: want allow (internal except does not cover WAN node), got %q (reason=%q)", got.Verdict, got.Reason)
	}
	engine := got.Engines["k8s"]
	if engine.Egress.Reason != ReasonPermitted {
		t.Errorf("egress reason: want %q (allow to 0.0.0.0/0 matches WAN dst), got %q", ReasonPermitted, engine.Egress.Reason)
	}
	if len(engine.Egress.DenyMatches) != 0 {
		t.Errorf("egress deny matches: want 0 (except ranges do not contain WAN node), got %d", len(engine.Egress.DenyMatches))
	}
}

// TestIsNodesReachable_AllowBroaderThanPodCIDR verifies that an egress allow
// with a peer that engulfs the pod CIDR (e.g. 10.0.0.0/8 covering pod
// 10.244.0.0/16) is treated as covering internal pods. String equality would
// miss this — the fix uses subnet containment.
func TestIsNodesReachable_AllowBroaderThanPodCIDR(t *testing.T) {
	installCIDRConfig(t)

	cache := buildCache("k8s", egressToCIDR("10.0.0.0/8"), ingressFrom(srcID))
	got := PoliciesCanReach(context.Background(), cache, srcID, srcNs, dstID, dstNs)

	if got.Verdict != "allow" {
		t.Errorf("verdict: want allow (10.0.0.0/8 ⊃ pod 10.244.0.0/16), got %q (reason=%q)", got.Verdict, got.Reason)
	}
	if reason := got.Engines["k8s"].Egress.Reason; reason != ReasonPermitted {
		t.Errorf("egress reason: want %q, got %q", ReasonPermitted, reason)
	}
}

// The reported Calico shape: one GlobalNetworkPolicy allowing the pod/svc/LAN
// nets then denying everything else. The engine buckets it into narrow allows
// (per CIDR) plus the policy's catch-all deny for the 0.0.0.0/0 bucket — both
// real rules from the SAME policy. Covers, in one scenario:
//   - dst is an internal pod (IP inside the allowed pod CIDR) → permitted; the
//     policy's own catch-all deny is its fallback for other peers, not this one
//   - dst is the WAN CIDR node → the same policy blocks: no allowed net contains
//     0.0.0.0/0, so the catch-all deny stands (exfil channel stays shut)
//   - a LAN-only allow must never speak for an internal pod
func TestIsNodesReachable_CalicoAllowInternalNetsThenDenyAll(t *testing.T) {
	installCIDRConfig(t)

	calicoPolicy := models.PolicyRef{Source: "calico", Name: "egress-default-deny"}
	allowNet := func(cidr string) models.Rule {
		rule := egressToCIDR(cidr)
		rule.Contributor = calicoPolicy
		return rule
	}
	catchAllDeny := models.Rule{
		SrcID: srcID, Direction: models.DirectionEgress, Action: models.ActionDeny,
		Coverage: models.CoverageDenyAll, AllPorts: true, Contributor: calicoPolicy,
	}
	rules := []models.Rule{
		allowNet("10.244.0.0/16"), allowNet("10.96.0.0/12"), allowNet("192.168.8.0/24"),
		catchAllDeny, ingressFrom(srcID),
	}

	// internal pod dst: pod CIDR allow wins over the same policy's fallback deny
	cache := buildCache("calico", rules...)
	got := PoliciesCanReach(context.Background(), cache, srcID, srcNs, dstID, dstNs)
	if got.Verdict != "allow" {
		t.Errorf("internal dst: want allow (pod CIDR allowed), got %q (reason=%q)", got.Verdict, got.Reason)
	}
	if reason := got.Engines["calico"].Egress.Reason; reason != ReasonPermitted {
		t.Errorf("internal dst egress reason: want %q, got %q", ReasonPermitted, reason)
	}

	// WAN dst: no allowed net contains 0.0.0.0/0 → catch-all deny governs
	wanDstID := models.CIDRIDPrefix + "0.0.0.0/0"
	wanCache := buildCache("calico", rules...)
	wanCache.EvaluationResults["calico"].Nodes[wanDstID] = models.WorkloadNode{
		ID: wanDstID, Label: "0.0.0.0/0", Type: models.NodeTypeCIDR, CidrType: models.CIDRWan,
	}
	wanCache.RebuildWorkloadIndex()
	gotWan := PoliciesCanReach(context.Background(), wanCache, srcID, srcNs, wanDstID, "")
	if gotWan.Verdict != "deny" {
		t.Errorf("WAN dst: want deny (only internal nets allowed), got %q (reason=%q)", gotWan.Verdict, gotWan.Reason)
	}

	// LAN-only allow + catch-all deny: an internal pod is NOT in 192.168.8.0/24
	lanCache := buildCache("calico", allowNet("192.168.8.0/24"), catchAllDeny, ingressFrom(srcID))
	gotLan := PoliciesCanReach(context.Background(), lanCache, srcID, srcNs, dstID, dstNs)
	if gotLan.Verdict != "deny" {
		t.Errorf("LAN-only allow: want deny for internal dst, got %q (reason=%q)", gotLan.Verdict, gotLan.Reason)
	}
}
