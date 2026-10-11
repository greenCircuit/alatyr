package store

import (
	"context"
	"testing"

	"alatyr/internal/models"
)

// Layering = one engine blocks at ns granularity while its OWN finer rules
// allow the pair. isLayeredPolicy decides which of three outcomes:
//
//	finer clause on this ns pair + pair reachable -> IssuesPolicyLayer
//	no finer clause on this ns pair               -> PolicyConflicts
//	finer clause but its pair is denied           -> PolicyConflicts
//
// Driven through PolicyIssues, so these also pin the buildIssues.go wiring.
// srcID/dstNs/otherDstID/buildNodeRules come from reachability_test.go.

const (
	dstMemberAID = "dst-ns/dst-a"
	dstMemberBID = "dst-ns/dst-b"
)

// Shared culprit identity — the AllowByNs join matches on it.
var baselineRef = models.PolicyRef{Source: "k8s", Name: "services-baseline", Namespace: dstNs}

// One compiled clause of the baseline policy. Bucketed by dst: ns-node = the
// coarse block, member pod = the finer tier.
func baselineRule(dst, src string) models.Rule {
	return models.Rule{
		SrcID: src, DstID: dst,
		Direction: models.DirectionIngress, Action: models.ActionAllow,
		Coverage:    models.CoverageRestricted,
		Contributor: baselineRef,
	}
}

// The coarse block: ns-node bucket admits a peer that is not srcID, so
// srcID->dst-ns reads locked-no-match with the baseline as culprit.
func nsBlock() models.Rule {
	return baselineRule(dstNsID, otherDstID)
}

// Two tiers: istio allows src->dst-ns at ns granularity (AllowByNs only, so no
// opinion in reachability), k8s holds the baseline clauses in AllowByNs (join
// target) + NodeRules (what probes read). istioRules give istio pod-level
// enforcement, to kill a finer path.
func layeringCache(k8sRules, istioRules []models.Rule) *models.Cache {
	dstWorkloads := []models.WorkloadNode{
		{ID: dstNsID, Namespace: dstNs, Type: models.NodeTypeNamespace},
		{ID: dstMemberAID, Namespace: dstNs, Label: "dst-a"},
		{ID: dstMemberBID, Namespace: dstNs, Label: "dst-b"},
		// must resolve, else clauses naming it fall out as unresolved endpoints
		// and the namespace checks never run
		{ID: otherDstID, Namespace: dstNs, Label: "other-pod"},
	}
	seed := []models.Rule{
		{SrcID: srcID, DstID: dstNsID, Direction: models.DirectionEgress, Action: models.ActionAllow},
		{SrcID: srcID, DstID: dstNsID, Direction: models.DirectionIngress, Action: models.ActionAllow},
	}
	cache := &models.Cache{
		NsIndex: map[string]models.NSIndex{
			srcNs: {
				NSNode:    &models.WorkloadNode{ID: srcNsID, Namespace: srcNs, Type: models.NodeTypeNamespace},
				Workloads: []models.WorkloadNode{{ID: srcID, Namespace: srcNs, Label: "src-pod"}},
			},
			dstNs: {
				NSNode:    &models.WorkloadNode{ID: dstNsID, Namespace: dstNs, Type: models.NodeTypeNamespace},
				Workloads: dstWorkloads,
			},
		},
		EvaluationResults: map[string]models.EvaluationResult{
			"istio": {
				AllowByNs: map[string][]models.Rule{dstNs: seed},
				NodeRules: buildNodeRules(istioRules),
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

// Run the detector, keep the ns-level pair. Pod-level pairs in the same sweep
// emit their own findings and must not pollute the assertion.
func nsPairIssue(t *testing.T, cache *models.Cache) models.Issue {
	t.Helper()
	var filtered []models.Issue
	for _, issue := range PolicyIssues(context.Background(), cache, nil) {
		if issue.Dst != nil && issue.Dst.ID == dstNsID {
			filtered = append(filtered, issue)
		}
	}
	if len(filtered) != 1 {
		t.Fatalf("ns-pair issues: want 1, got %d (%+v)", len(filtered), filtered)
	}
	return filtered[0]
}

func assertIssueType(t *testing.T, issue models.Issue, want models.IssueType) {
	t.Helper()
	if issue.Type != want {
		t.Errorf("type: want %q, got %q", want, issue.Type)
	}
}

// podSelector {} compiles one policy into both buckets: ns-node (the block)
// and pod (the allow). Evidence must be that baseline, never the istio
// counterpart tier — the lookup reads the blocking engine only.
func TestPolicyIssues_LayeringBaselineOnlyNamespace(t *testing.T) {
	coarseRef := models.PolicyRef{Source: "istio", Name: "external-access", Namespace: dstNs}
	seed := []models.Rule{
		{SrcID: srcNsID, DstID: dstNsID, Direction: models.DirectionEgress, Action: models.ActionAllow, Contributor: coarseRef},
		{SrcID: srcNsID, DstID: dstNsID, Direction: models.DirectionIngress, Action: models.ActionAllow, Contributor: coarseRef},
	}
	cache := layeringCache([]models.Rule{baselineRule(dstNsID, srcID)}, nil)
	cache.EvaluationResults["istio"] = models.EvaluationResult{
		AllowByNs: map[string][]models.Rule{dstNs: seed},
		NodeRules: buildNodeRules(seed),
	}
	// seed SrcID is the src ns-node, so it has to resolve via WorkloadByID
	srcIndex := cache.NsIndex[srcNs]
	srcIndex.Workloads = append(srcIndex.Workloads,
		models.WorkloadNode{ID: srcNsID, Namespace: srcNs, Type: models.NodeTypeNamespace})
	cache.NsIndex[srcNs] = srcIndex
	cache.RebuildWorkloadIndex()

	issue := nsPairIssue(t, cache)
	assertIssueType(t, issue, models.IssuesPolicyLayer)

	if len(issue.IngressAllowed) != 1 {
		t.Fatalf("ingressAllowed: want 1 ref, got %+v", issue.IngressAllowed)
	}
	if got := issue.IngressAllowed[0]; got.Source != baselineRef.Source || got.Name != baselineRef.Name {
		t.Errorf("ingressAllowed: want %q/%q, got %+v", baselineRef.Source, baselineRef.Name, got)
	}
}

// Same coarse block every case. Only the baseline's OTHER clauses vary.
func TestPolicyIssues_LayeringClassification(t *testing.T) {
	cases := []struct {
		name     string
		k8sRules []models.Rule
		wantType models.IssueType
	}{
		{
			name:     "finer clause names a pod of the source namespace",
			k8sRules: []models.Rule{nsBlock(), baselineRule(dstMemberAID, srcID)},
			wantType: models.IssuesPolicyLayer,
		},
		{
			name:     "no finer clause at all",
			k8sRules: []models.Rule{nsBlock()},
			wantType: models.PolicyConflicts,
		},
		{
			// otherDstID is in dst-ns: describes dst-ns->dst-ns, explains nothing
			name:     "finer clause names a peer outside the source namespace",
			k8sRules: []models.Rule{nsBlock(), baselineRule(dstMemberAID, otherDstID)},
			wantType: models.PolicyConflicts,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assertIssueType(t, nsPairIssue(t, layeringCache(tc.k8sRules, nil)), tc.wantType)
		})
	}
}

// Finer clause exists but istio denies that exact pod pair, so nothing can
// talk. Must stay a conflict, not get filed under info as "expected layering".
func TestPolicyIssues_LayeringRequiresReachableFinePath(t *testing.T) {
	istioDeny := models.Rule{
		SrcID: srcID, DstID: dstMemberAID,
		Direction: models.DirectionIngress, Action: models.ActionDeny,
	}
	cache := layeringCache(
		[]models.Rule{nsBlock(), baselineRule(dstMemberAID, srcID)},
		[]models.Rule{istioDeny},
	)
	assertIssueType(t, nsPairIssue(t, cache), models.PolicyConflicts)
}

const otherSrcID = "other-ns/other-pod"

// Namespace on the SOURCE end: istio allows src-ns -> dst-a, k8s baseline
// clauses sit on dst-a. Coarse pair denies because an ns-node source cannot
// match a pod-granular from-clause.
func nsSourceCache(baselineClauses []models.Rule) *models.Cache {
	coarseRef := models.PolicyRef{Source: "istio", Name: "external-access", Namespace: dstNs}
	seed := []models.Rule{
		{SrcID: srcNsID, DstID: dstMemberAID, Direction: models.DirectionEgress, Action: models.ActionAllow, Contributor: coarseRef},
		{SrcID: srcNsID, DstID: dstMemberAID, Direction: models.DirectionIngress, Action: models.ActionAllow, Contributor: coarseRef},
	}
	cache := &models.Cache{
		NsIndex: map[string]models.NSIndex{
			srcNs: {
				NSNode: &models.WorkloadNode{ID: srcNsID, Namespace: srcNs, Type: models.NodeTypeNamespace},
				Workloads: []models.WorkloadNode{
					{ID: srcNsID, Namespace: srcNs, Type: models.NodeTypeNamespace},
					{ID: srcID, Namespace: srcNs, Label: "src-pod"},
				},
			},
			dstNs: {
				NSNode: &models.WorkloadNode{ID: dstNsID, Namespace: dstNs, Type: models.NodeTypeNamespace},
				Workloads: []models.WorkloadNode{
					{ID: dstNsID, Namespace: dstNs, Type: models.NodeTypeNamespace},
					{ID: dstMemberAID, Namespace: dstNs, Label: "dst-a"},
				},
			},
			"other-ns": {
				NSNode: &models.WorkloadNode{ID: "ns/other-ns", Namespace: "other-ns", Type: models.NodeTypeNamespace},
				Workloads: []models.WorkloadNode{
					{ID: "ns/other-ns", Namespace: "other-ns", Type: models.NodeTypeNamespace},
					{ID: otherSrcID, Namespace: "other-ns", Label: "other-pod"},
				},
			},
		},
		EvaluationResults: map[string]models.EvaluationResult{
			"istio": {
				AllowByNs: map[string][]models.Rule{dstNs: seed},
				NodeRules: buildNodeRules(seed),
			},
			"k8s": {
				AllowByNs: map[string][]models.Rule{dstNs: baselineClauses},
				NodeRules: buildNodeRules(baselineClauses),
			},
		},
	}
	cache.RebuildWorkloadIndex()
	return cache
}

// Other cases all put the namespace on the dst end. Here it is the src end, so
// the join must key on the namespace each side belongs to (an ns node belongs
// to itself) instead of two blank namespace fields agreeing.
func TestPolicyIssues_LayeringNamespaceOnSourceEnd(t *testing.T) {
	cases := []struct {
		name     string
		clauses  []models.Rule
		wantType models.IssueType
	}{
		{
			name:     "baseline admits a pod of the source namespace",
			clauses:  []models.Rule{baselineRule(dstMemberAID, srcID)},
			wantType: models.IssuesPolicyLayer,
		},
		{
			name:     "baseline admits only pods of another namespace",
			clauses:  []models.Rule{baselineRule(dstMemberAID, otherSrcID)},
			wantType: models.PolicyConflicts,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var coarse []models.Issue
			for _, issue := range PolicyIssues(context.Background(), nsSourceCache(tc.clauses), nil) {
				if issue.Src != nil && issue.Src.ID == srcNsID && issue.Dst != nil && issue.Dst.ID == dstMemberAID {
					coarse = append(coarse, issue)
				}
			}
			if len(coarse) != 1 {
				t.Fatalf("coarse-pair issues: want 1, got %d (%+v)", len(coarse), coarse)
			}
			assertIssueType(t, coarse[0], tc.wantType)
		})
	}
}
