package calico

import (
	"graph/internal/models"
	"graph/internal/policy"
)

// resolvedEdge — winner of the first-match walk for one (workload, direction,
// bucket). Zero-value port means the bucket is direction-wide (no port
// restriction); otherwise the winner rule listed exactly this port.
type resolvedEdge struct {
	peer   peer
	port   models.Port
	action models.RuleAction
	winner models.PolicyRef
	// winner selects the whole namespace by construction (all() / empty selector)
	namespaceWide bool
}

// rulesFor picks the direction-specific rule slice off a policy.
func rulesFor(gp globalPolicy, direction models.Direction) []calicoRule {
	if direction == models.DirectionIngress {
		return gp.ingress
	}
	return gp.egress
}

// resolveDirection — first-match verdict per (cidr, port) bucket using the
// precomputed candidate index. Per-bucket walk cost = one selector call per
// candidate until first match; no CIDR/port arithmetic at this layer.
func resolveDirection(node models.WorkloadNode, direction models.Direction, buckets []bucketKey, candidates map[bucketKey][]bucketCandidate) []resolvedEdge {
	labels := node.Labels
	if node.Type == models.NodeTypeNamespace {
		labels = map[string]string{}
	}
	edges := make([]resolvedEdge, 0, len(buckets))
	for _, bucket := range buckets {
		for _, cand := range candidates[bucket] {
			if cand.workloadMatch == nil || !cand.workloadMatch(labels) {
				continue
			}
			edges = append(edges, resolvedEdge{
				peer:          bucket.peer,
				port:          bucket.port,
				action:        cand.action,
				winner:        cand.winner,
				namespaceWide: cand.selectsAll,
			})
			break
		}
	}
	return dropRestatedEdges(edges)
}

// dropRestatedEdges removes buckets whose winner is the same rule that already
// won the direction-wide bucket. Buckets come from the union of every selecting
// policy's nets, so a catch-all winner is first-match for nets it never named —
// emitting those draws arrows to CIDRs absent from that policy, claiming scope
// it does not have. A bucket only carries information when precedence diverged
// there. Port left in the comparison: an all-ports winner beating a port-scoped
// sibling bucket is a real narrowing, not a restatement.
func dropRestatedEdges(edges []resolvedEdge) []resolvedEdge {
	var directionWide *resolvedEdge
	for index, edge := range edges {
		if edge.peer.kind == peerCIDR && edge.peer.cidr == catchAllCIDR && edge.port == (models.Port{}) {
			directionWide = &edges[index]
			break
		}
	}
	if directionWide == nil {
		return edges
	}
	kept := make([]resolvedEdge, 0, len(edges))
	for _, edge := range edges {
		restated := edge.peer != directionWide.peer &&
			edge.port == directionWide.port &&
			edge.action == directionWide.action &&
			edge.winner == directionWide.winner
		if restated {
			continue
		}
		kept = append(kept, edge)
	}
	return kept
}

// buildRulesForWorkload — resolved edges → bucketed Allow/Deny + synthetic CIDR
// nodes. Egress points workload→CIDR, ingress CIDR→workload. catchAllCIDR
// buckets skip the peer-CIDR node entirely and leave the peer endpoint empty —
// the Coverage=AllowAll/DenyAll marker carries the direction-wide semantic,
// same as k8s blanket rules. Only narrower CIDRs (e.g. 10.0.0.0/8) synthesize
// a peer WorkloadNode. Splits by Action inline so the caller skips a re-walk.
func buildRulesForWorkload(node models.WorkloadNode, edges []resolvedEdge, direction models.Direction) (models.DirectionRules, map[string]models.WorkloadNode) {
	var bucket models.DirectionRules
	cidrNodes := map[string]models.WorkloadNode{}
	for _, edge := range edges {
		rule := models.Rule{
			Direction:     direction,
			Action:        edge.action,
			Coverage:      coverageFor(edge),
			Contributor:   edge.winner,
			NamespaceWide: edge.namespaceWide,
		}
		if edge.port == (models.Port{}) {
			rule.AllPorts = true
		} else {
			rule.Ports = []models.Port{edge.port}
		}
		if edge.peer.kind == peerCIDR && edge.peer.cidr == catchAllCIDR {
			if direction == models.DirectionIngress {
				rule.DstID = node.ID
			} else {
				rule.SrcID = node.ID
			}
		} else {
			cidrID := models.CIDRIDPrefix + edge.peer.cidr
			cidrNodes[cidrID] = models.WorkloadNode{
				ID:       cidrID,
				Label:    edge.peer.cidr,
				Type:     models.NodeTypeCIDR,
				CidrType: policy.CidrType(edge.peer.cidr),
			}
			if direction == models.DirectionIngress {
				rule.SrcID = cidrID
				rule.DstID = node.ID
			} else {
				rule.SrcID = node.ID
				rule.DstID = cidrID
			}
		}
		if edge.action == models.ActionDeny {
			bucket.Deny = append(bucket.Deny, rule)
		} else {
			bucket.Allow = append(bucket.Allow, rule)
		}
	}
	return bucket, cidrNodes
}

// coverageFor — direction-wide blanket requires BOTH catchAllCIDR AND
// (models.Port{}) (any-cidr, any-port). Any narrower CIDR OR any port
// restriction → per-slice rule = Restricted.
func coverageFor(edge resolvedEdge) models.Coverage {
	if edge.peer.kind == peerCIDR && edge.peer.cidr == catchAllCIDR && edge.port == (models.Port{}) {
		if edge.action == models.ActionAllow {
			return models.CoverageAllowAll
		}
		return models.CoverageDenyAll
	}
	return models.CoverageRestricted
}
