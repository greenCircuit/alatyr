package store

import (
	"context"
	"testing"

	"alatyr/internal/models"
)

// Tests for layering classification — the ns-vs-pod granularity split. An
// ns-level allow (istio seed) reads "deny" against k8s's pod-keyed rules
// because the ns-node probe can't match pod-granular clauses. classifyLayering
// must join the blocking culprit back to its compiled rules and probe the
// pod-level srcNs→dstNs pairs those rules name:
//   - a fine path verifies reachable → tiers composing → IssuesPartial
//   - fine paths exist but all deny  → genuine block     → PolicyConflicts
//   - no pod-level path names srcNs  → genuine block     → PolicyConflicts
// There is no drop case: a deny verdict always emits; only the type changes.
// Reuses srcID/dstNs/... constants and buildNodeRules from reachability_test.go.

const (
	dstMemberAID = "dst-ns/dst-a"
	dstMemberBID = "dst-ns/dst-b"
)

// baselineRef is the culprit identity: every compiled rule of the fixture
// baseline policy carries it, which is what the AllowByNs join matches on.
var baselineRef = models.PolicyRef{Source: "k8s", Name: "services-baseline", Namespace: dstNs}

// baselineRule is one compiled clause of the baseline policy: an ingress allow
// bucketed by DstID (ns-node bucket for the coarse block, member bucket for
// the pod-granular tier), stamped with the shared Contributor.
func baselineRule(dst, src string) models.Rule {
	return models.Rule{
		SrcID: src, DstID: dst,
		Direction: models.DirectionIngress, Action: models.ActionAllow,
		Coverage:    models.CoverageRestricted,
		Contributor: baselineRef,
	}
}

// layeringCache wires the two-tier setup: istio seeds src→dst-ns at namespace
// granularity (AllowByNs only — no NodeRules, so it has no opinion in
// reachability), k8s holds the baseline policy's compiled rules in both
// AllowByNs (the join target) and NodeRules (the enforcement the probes read).
// istioDenyRules optionally give istio pod-level enforcement to kill fine paths.
func layeringCache(k8sRules, istioDenyRules []models.Rule) *models.Cache {
	srcPod := models.WorkloadNode{ID: srcID, Namespace: srcNs, Label: "src-pod"}
	srcNsNode := &models.WorkloadNode{ID: srcNsID, Namespace: srcNs, Type: models.NodeTypeNamespace}
	dstNsNode := &models.WorkloadNode{ID: dstNsID, Namespace: dstNs, Type: models.NodeTypeNamespace}
	dstWorkloads := []models.WorkloadNode{
		{ID: dstNsID, Namespace: dstNs, Type: models.NodeTypeNamespace},
		{ID: dstMemberAID, Namespace: dstNs, Label: "dst-a"},
		{ID: dstMemberBID, Namespace: dstNs, Label: "dst-b"},
	}
	seed := []models.Rule{
		{SrcID: srcID, DstID: dstNsID, Direction: models.DirectionEgress, Action: models.ActionAllow},
		{SrcID: srcID, DstID: dstNsID, Direction: models.DirectionIngress, Action: models.ActionAllow},
	}
	cache := &models.Cache{
		NsIndex: map[string]models.NSIndex{
			srcNs: {NSNode: srcNsNode, Workloads: []models.WorkloadNode{srcPod}},
			dstNs: {NSNode: dstNsNode, Workloads: dstWorkloads},
		},
		EvaluationResults: map[string]models.EvaluationResult{
			"istio": {
				AllowByNs: map[string][]models.Rule{dstNs: seed},
				NodeRules: buildNodeRules(istioDenyRules),
			},
			"k8s": {
				AllowByNs: map[string][]models.Rule{dstNs: k8sRules},
				NodeRules: buildNodeRules(k8sRules),
			},
		},
	}
	cache.RebuildWorkloadIndex()
	return cache
}

// nsPairIssues filters to the ns-level pair under test — pod-level pairs from
// the same sweep may legitimately emit their own conflicts (case: istio denies
// a fine path), and those must not pollute the ns-pair assertion.
func nsPairIssues(issues []models.Issue) []models.Issue {
	var filtered []models.Issue
	for _, issue := range issues {
		if issue.Dst != nil && issue.Dst.ID == dstNsID {
			filtered = append(filtered, issue)
		}
	}
	return filtered
}

// Baseline-only namespace: the policy's podSelector {} compiles its selected
// side to the NS NODE, so the fine tier is pod→ns-node — one coarse end. The
// coarse pair is ns→ns (istio names the src namespace, so SrcID is the src
// ns-node, which can never match the baseline's pod-granular from-clause →
// deny). The candidate filter must accept the one-coarse-end rule; requiring
// pods on both ends re-phantoms every baseline-only namespace as a conflict.
func TestPolicyIssues_LayeringBaselineOnlyNamespace(t *testing.T) {
	// the coarse tier: an ns-level istio allow (mirrors external-access)
	coarseRef := models.PolicyRef{Source: "istio", Name: "external-access", Namespace: dstNs}
	seed := []models.Rule{
		{SrcID: srcNsID, DstID: dstNsID, Direction: models.DirectionEgress, Action: models.ActionAllow, Contributor: coarseRef},
		{SrcID: srcNsID, DstID: dstNsID, Direction: models.DirectionIngress, Action: models.ActionAllow, Contributor: coarseRef},
	}
	// the baseline's compiled tier: ns-node bucket admits the src POD only
	k8sRules := []models.Rule{baselineRule(dstNsID, srcID)}
	cache := layeringCache(k8sRules, nil)
	cache.EvaluationResults["istio"] = models.EvaluationResult{
		AllowByNs: map[string][]models.Rule{dstNs: seed},
		NodeRules: buildNodeRules(seed),
	}
	// the seed's SrcID is the src ns-node — it must resolve via WorkloadByID,
	// so the ns-node joins the workload list like real caches have it
	srcIndex := cache.NsIndex[srcNs]
	srcIndex.Workloads = append(srcIndex.Workloads,
		models.WorkloadNode{ID: srcNsID, Namespace: srcNs, Type: models.NodeTypeNamespace})
	cache.NsIndex[srcNs] = srcIndex
	cache.RebuildWorkloadIndex()

	issues := nsPairIssues(PolicyIssues(context.Background(), cache, nil))
	if len(issues) != 1 {
		t.Fatalf("ns-pair issues: want 1, got %d (%+v)", len(issues), issues)
	}
	if issues[0].Type != models.IssuesPartial {
		t.Errorf("type: want %q, got %q", models.IssuesPartial, issues[0].Type)
	}
	// Layering evidence = the COUNTERPART tier only: the coarse istio allow
	// rides on the issue; the row's own culprit (the baseline) must NOT echo
	// as evidence — that duplication is the UI lying.
	foundCoarse, foundBaseline := false, false
	for _, ref := range issues[0].IngressAllowed {
		if ref.Name == coarseRef.Name && ref.Source == coarseRef.Source {
			foundCoarse = true
		}
		if ref.Name == baselineRef.Name && ref.Source == baselineRef.Source {
			foundBaseline = true
		}
	}
	if !foundCoarse {
		t.Errorf("ingressAllowed: want coarse-tier ref %q, got %+v", coarseRef.Name, issues[0].IngressAllowed)
	}
	if foundBaseline {
		t.Errorf("ingressAllowed: culprit %q echoed as evidence (duplication), got %+v", baselineRef.Name, issues[0].IngressAllowed)
	}
}

// The ns-node bucket carries only a non-src clause, so the coarse probe reads
// locked-no-match (deny) with the baseline as culprit. The join then decides
// the type from the baseline's own pod-level clauses.
func TestPolicyIssues_LayeringClassification(t *testing.T) {
	// coarse block: the ns-node bucket admits only a non-src peer
	nsBlock := baselineRule(dstNsID, otherDstID)

	cases := []struct {
		name           string
		k8sRules       []models.Rule
		istioDenyRules []models.Rule
		wantType       models.IssueType
	}{
		{
			name:     "partial when a pod clause admits src and the path verifies",
			k8sRules: []models.Rule{nsBlock, baselineRule(dstMemberAID, srcID)},
			wantType: models.IssuesPartial,
		},
		{
			name:     "partial when every pod clause verifies — no drop, type still partial",
			k8sRules: []models.Rule{nsBlock, baselineRule(dstMemberAID, srcID), baselineRule(dstMemberBID, srcID)},
			wantType: models.IssuesPartial,
		},
		{
			name:     "conflict when culprit has no pod-level clauses",
			k8sRules: []models.Rule{nsBlock},
			wantType: models.PolicyConflicts,
		},
		{
			name:     "conflict when pod clauses admit only peers outside src ns",
			k8sRules: []models.Rule{nsBlock, baselineRule(dstMemberAID, otherDstID)},
			wantType: models.PolicyConflicts,
		},
		{
			name:     "conflict when another engine denies the fine path",
			k8sRules: []models.Rule{nsBlock, baselineRule(dstMemberAID, srcID)},
			istioDenyRules: []models.Rule{
				{SrcID: srcID, DstID: dstMemberAID, Direction: models.DirectionIngress, Action: models.ActionDeny},
			},
			wantType: models.PolicyConflicts,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cache := layeringCache(tc.k8sRules, tc.istioDenyRules)
			issues := nsPairIssues(PolicyIssues(context.Background(), cache, nil))
			if len(issues) != 1 {
				t.Fatalf("ns-pair issues: want 1, got %d (%+v)", len(issues), issues)
			}
			if issues[0].Type != tc.wantType {
				t.Errorf("type: want %q, got %q", tc.wantType, issues[0].Type)
			}
		})
	}
}
