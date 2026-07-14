package store

import (
	"context"
	"fmt"
	"testing"

	"graph/internal/models"
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
	return &models.Cache{
		NsIndex: map[string]models.NSIndex{
			srcNs: {NSNode: srcNsNode},
			dstNs: {NSNode: dstNsNode},
		},
		EvaluationResults: map[string]models.EvaluationResult{
			engine: {NodeRules: buildNodeRules(rules)},
		},
	}
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
			got := IsNodesReachable(context.Background(), cache, nil, srcID, srcNs, dstID, dstNs)

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
	// near-miss culprits must collapse to one policy — RuleIndex is not identity.
	policy := models.PolicyRef{Source: "k8s", Name: "web-netpol", Namespace: srcNs}
	firstRule := egressTo(otherDstID)
	firstRule.Contributor = policy
	firstRule.Contributor.RuleIndex = 0
	secondRule := egressTo("dst-ns/third-pod")
	secondRule.Contributor = policy
	secondRule.Contributor.RuleIndex = 1

	cache := buildCache("k8s", firstRule, secondRule)
	got := IsNodesReachable(context.Background(), cache, nil, srcID, srcNs, dstID, dstNs)

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
	got := IsNodesReachable(context.Background(), cache, nil, srcID, srcNs, dstID, dstNs)

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
