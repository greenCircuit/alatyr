package calico

import (
	"graph/internal/models"
	"graph/internal/policy"
)


// buildNsSelection — full ns-level cache: nsSelector filter + bucket set +
// per-bucket candidate list, for both directions. Called once per namespace
// during Evaluate.
func buildNsSelection(policies []globalPolicy, nsLabels map[string]string) nsSelection {
	nsPolicies := nsSelectingPolicies(policies, nsLabels)
	if len(nsPolicies) == 0 {
		return nsSelection{}
	}
	egressBuckets := destinationBuckets(nsPolicies, models.DirectionEgress)
	ingressBuckets := destinationBuckets(nsPolicies, models.DirectionIngress)
	return nsSelection{
		policies:          nsPolicies,
		egressBuckets:     egressBuckets,
		ingressBuckets:    ingressBuckets,
		egressCandidates:  buildBucketCandidates(nsPolicies, models.DirectionEgress, egressBuckets),
		ingressCandidates: buildBucketCandidates(nsPolicies, models.DirectionIngress, ingressBuckets),
	}
}

// nsSelectingPolicies — first-pass filter by nsSelector only. Runs once per
// namespace, shared across every workload in that ns. Precedence order kept.
func nsSelectingPolicies(policies []globalPolicy, nsLabels map[string]string) []globalPolicy {
	var matches []globalPolicy
	for _, gp := range policies {
		if gp.nsSelectorMatch != nil && !gp.nsSelectorMatch(nsLabels) {
			continue
		}
		matches = append(matches, gp)
	}
	return matches
}

// workloadSelectingPolicies — second-pass filter by workload selector. Input
// is the ns-filtered slice from nsSelectingPolicies. Precedence order kept.
// Namespace nodes only match catch-all selectors.
func workloadSelectingPolicies(nsFiltered []globalPolicy, node models.WorkloadNode) []globalPolicy {
	var matches []globalPolicy
	for _, gp := range nsFiltered {
		if gp.selectorMatch == nil {
			continue
		}
		if node.Type == models.NodeTypeNamespace {
			if gp.selectorMatch(map[string]string{}) {
				matches = append(matches, gp)
			}
			continue
		}
		if gp.selectorMatch(node.Labels) {
			matches = append(matches, gp)
		}
	}
	return matches
}

// destinationBuckets — every (cidr, port) tuple any rule references, plus the
// (catchAll, allPorts) seed so a workload with only port-scoped rules still
// gets a chance at a direction-wide verdict.
func destinationBuckets(ordered []globalPolicy, direction models.Direction) []bucketKey {
	seed := bucketKey{peer: peer{kind: peerCIDR, cidr: catchAllCIDR}, port: models.Port{}}
	seen := map[bucketKey]bool{seed: true}
	buckets := []bucketKey{seed}
	for _, gp := range ordered {
		for _, rule := range rulesFor(gp, direction) {
			cidrs := rule.nets
			if len(cidrs) == 0 {
				cidrs = []string{catchAllCIDR}
			}
			ports := rule.ports
			if len(ports) == 0 {
				ports = []models.Port{models.Port{}}
			}
			for _, cidr := range cidrs {
				for _, port := range ports {
					key := bucketKey{peer: peer{kind: peerCIDR, cidr: cidr}, port: port}
					if !seen[key] {
						seen[key] = true
						buckets = append(buckets, key)
					}
				}
			}
		}
	}
	return buckets
}

// buildBucketCandidates — per (direction, bucket) walk ns-filtered policies in
// precedence order and precompute the ordered candidate list. Each policy
// contributes at most one rule per bucket (first terminal rule wins for that
// policy); Pass ends the policy's contribution without emitting.
func buildBucketCandidates(policies []globalPolicy, direction models.Direction, buckets []bucketKey) map[bucketKey][]bucketCandidate {
	out := make(map[bucketKey][]bucketCandidate, len(buckets))
	for _, bucket := range buckets {
		var candidates []bucketCandidate
		for _, gp := range policies {
			for _, rule := range rulesFor(gp, direction) {
				if !rule.covers(bucket.peer) {
					continue
				}
				if !portsCover(rule.ports, bucket.port) {
					continue
				}
				if rule.isPass {
					break
				}
				ref := gp.ref
				candidates = append(candidates, bucketCandidate{
					winner:        ref,
					action:        rule.action,
					workloadMatch: gp.selectorMatch,
					selectsAll:    gp.selectsAllWorkloads,
				})
				break
			}
		}
		if len(candidates) > 0 {
			out[bucket] = candidates
		}
	}
	return out
}

// netsCover — does a rule match the bucket. notNets carve out first; empty nets
// = no CIDR constraint = matches every bucket.
// covers — does the rule match a destination peer. Dispatches on peer kind so
// each dimension keeps its own algebra: CIDR uses address-space containment;
// selector/serviceAccount arms match labels when added. Unknown kind = no match
// (fail closed) so a half-wired peer kind can't leak an allow.
func (rule calicoRule) covers(target peer) bool {
	switch target.kind {
	case peerCIDR:
		return netsCover(rule.nets, rule.notNets, target.cidr)
	default:
		return false
	}
}

func netsCover(nets, notNets []string, bucket string) bool {
	for _, excluded := range notNets {
		if policy.CidrContains(excluded, bucket) {
			return false
		}
	}
	if len(nets) == 0 {
		return true
	}
	for _, allowed := range nets {
		if policy.CidrContains(allowed, bucket) {
			return true
		}
	}
	return false
}

// portsCover — empty rule ports = matches any bucket. A port-restricted rule
// matches only when the bucket names one of its listed ports; the all-ports
// bucket falls through because a restricted rule can't speak for it.
func portsCover(rulePorts []models.Port, bucket models.Port) bool {
	if len(rulePorts) == 0 {
		return true
	}
	if bucket == (models.Port{}) {
		return false
	}
	for _, port := range rulePorts {
		if port == bucket {
			return true
		}
	}
	return false
}
