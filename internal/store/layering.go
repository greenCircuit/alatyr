package store

// Layering classification: decides whether an ns-level policy conflict is a
// genuine block or two policy tiers composing as designed (a coarse ns allow
// over a pod-granular baseline). Evidence comes from the blocking culprits'
// own compiled rules; the fused reachability verdict stays the only judge.

import (
	"context"

	"graph/internal/models"
)

// candidatePair is one pod-granular path hiding under an ns-level block — a
// culprit policy clause that names concrete workloads on both ends.
type candidatePair struct {
	src models.WorkloadNode
	dst models.WorkloadNode
}

// fineAllows carries the policies that verified the working pod-level paths —
// the layering evidence the UI shows next to the coarse block, per direction.
type fineAllows struct {
	ingress []models.PolicyRef
	egress  []models.PolicyRef
}

// classifyLayering decides whether an ns-endpoint conflict is a genuine block
// or two policy tiers composing as designed: join the blocking culprits back
// to their compiled rules, keep the pod-granular srcNs→dstNs pairs those rules
// name, and probe just those with the fused verdict. Any reachable fine path
// means the coarse block is expected layering (partial access); none means a
// real conflict. The returned fineAllows are the policies that permitted the
// verified paths, so the issue can show WHICH fine tier composes with the block.
func classifyLayering(ctx context.Context, prober *reachabilityProber, verdict ReachabilityResult, srcNs, dstNs string) (models.IssueType, fineAllows) {
	candidates := findPodLevelCandidates(prober.data, verdict, srcNs, dstNs)
	reachable := 0
	var ingressAllowRules, egressAllowRules []models.NodeRule
	for _, pair := range candidates {
		result := prober.probe(ctx, pair.src.ID, pair.src.Namespace, pair.dst.ID, pair.dst.Namespace)
		if result.Verdict == "deny" {
			continue
		}
		reachable++
		for _, engineVerdict := range result.Engines {
			ingressAllowRules = append(ingressAllowRules, engineVerdict.Ingress.AllowMatches...)
			egressAllowRules = append(egressAllowRules, engineVerdict.Egress.AllowMatches...)
		}
	}
	fine := fineAllows{
		ingress: dedupContributors(ingressAllowRules),
		egress:  dedupContributors(egressAllowRules),
	}
	return classifyConflict(reachable, len(candidates)), fine
}

// excludePolicyRefs drops refs present in exclude, by policy identity. Used to
// keep a row's own culprits out of its layering evidence — a baseline policy
// is both the blocker and the fine tier, and echoing it as evidence reads as
// duplication; the evidence should be the OTHER tier (the coarse allow).
func excludePolicyRefs(refs, exclude []models.PolicyRef) []models.PolicyRef {
	type policyIdentity struct{ source, name, namespace string }
	excluded := map[policyIdentity]bool{}
	for _, ref := range exclude {
		excluded[policyIdentity{ref.Source, ref.Name, ref.Namespace}] = true
	}
	var kept []models.PolicyRef
	for _, ref := range refs {
		if excluded[policyIdentity{ref.Source, ref.Name, ref.Namespace}] {
			continue
		}
		kept = append(kept, ref)
	}
	return kept
}

// mergePolicyRefs appends extras not already present by policy identity.
func mergePolicyRefs(base, extras []models.PolicyRef) []models.PolicyRef {
	type policyIdentity struct{ source, name, namespace string }
	seen := map[policyIdentity]bool{}
	for _, ref := range base {
		seen[policyIdentity{ref.Source, ref.Name, ref.Namespace}] = true
	}
	for _, ref := range extras {
		identity := policyIdentity{ref.Source, ref.Name, ref.Namespace}
		if seen[identity] {
			continue
		}
		seen[identity] = true
		base = append(base, ref)
	}
	return base
}

// classifyConflict maps probe results over pod-granular candidates to the
// final issue type. Pure — unit-testable with raw counts.
func classifyConflict(reachable, total int) models.IssueType {
	if total > 0 && reachable > 0 {
		return models.IssuesPartial
	}
	return models.PolicyConflicts
}

// findPodLevelCandidates joins every denying engine's culprits back to their
// compiled rules (the verdict distills rules to PolicyRefs; AllowByNs still
// holds the rules) and keeps the pairs where both ends resolve to workloads in
// the conflicting namespaces. Deduped — a policy repeats a pair per port.
func findPodLevelCandidates(data *models.Cache, verdict ReachabilityResult, srcNs, dstNs string) []candidatePair {
	seen := make(map[string]bool)
	var candidates []candidatePair
	for engineName, engineVerdict := range verdict.Engines {
		if engineVerdict.Status != "deny" {
			continue
		}
		eval, ok := data.EvaluationResults[engineName]
		if !ok {
			continue
		}
		culprits := make([]models.PolicyRef, 0, len(engineVerdict.Ingress.Culprits)+len(engineVerdict.Egress.Culprits))
		culprits = append(culprits, engineVerdict.Ingress.Culprits...)
		culprits = append(culprits, engineVerdict.Egress.Culprits...)
		for _, culprit := range culprits {
			candidates = appendCulpritPairs(candidates, seen, data, eval, culprit, srcNs, dstNs)
		}
	}
	return candidates
}

// appendCulpritPairs joins one culprit ref to its compiled rules by
// contributor identity and appends the pod-granular srcNs→dstNs pairs those
// rules name. seen dedups pairs across culprits and ports.
func appendCulpritPairs(candidates []candidatePair, seen map[string]bool, data *models.Cache, eval models.EvaluationResult, culprit models.PolicyRef, srcNs, dstNs string) []candidatePair {
	for _, rule := range eval.AllowByNs[culprit.Namespace] {
		contributor := rule.Contributor
		if contributor.Source != culprit.Source || contributor.Name != culprit.Name || contributor.Namespace != culprit.Namespace {
			continue
		}
		srcMember, srcOk := data.WorkloadByID[rule.SrcID]
		if !srcOk || srcMember.Namespace != srcNs {
			continue
		}
		dstMember, dstOk := data.WorkloadByID[rule.DstID]
		if !dstOk || dstMember.Namespace != dstNs {
			continue
		}
		// A candidate must be finer than the ns pair it explains — at least one
		// pod-granular end. One coarse end is legitimate: baseline policies
		// (podSelector {}) compile their selected side to the ns node, so their
		// fine tier is pod→ns-node.
		if srcMember.Type == models.NodeTypeNamespace && dstMember.Type == models.NodeTypeNamespace {
			continue
		}
		pairKey := srcMember.ID + "->" + dstMember.ID
		if srcMember.ID == dstMember.ID || seen[pairKey] {
			continue
		}
		seen[pairKey] = true
		candidates = append(candidates, candidatePair{src: srcMember, dst: dstMember})
	}
	return candidates
}
