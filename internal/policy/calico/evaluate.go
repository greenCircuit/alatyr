package calico

import (
	"context"
	"fmt"
	"log/slog"

	"graph/internal/k8s"
	"graph/internal/models"
)

const sourceName = "calico"

type source struct {
	client k8s.KubernetesClient
	log    *slog.Logger
}

func New(client k8s.KubernetesClient, logger *slog.Logger) *source {
	if logger == nil {
		logger = slog.Default()
	}
	return &source{client: client, log: logger}
}

func (s *source) Name() string {
	return sourceName
}

// Evaluate — fetch globals once, sort by precedence, resolve first-match per
// (workload, direction, bucket), derive status. Rules bucketed by the matched
// workload's namespace (GlobalNetworkPolicy has none of its own — ADR 0005).
// Two-level filter: nsSelector runs per-ns, workload selector per-workload.
// Kills O(workloads × policies) nsSelector calls at scale.
func (s *source) Evaluate(ctx context.Context, namespaces []string, index map[string]models.NSIndex) (models.EvaluationResult, error) {
	policies, err := s.getPolicies(ctx)
	if err != nil {
		return models.EvaluationResult{}, err
	}
	sortByPrecedence(policies)

	result := models.EvaluationResult{
		AllowByNs:      map[string][]models.Rule{},
		DenyByNs:       map[string][]models.Rule{},
		PolicyStatuses: map[string]models.PolicyStatus{},
		NodePolicies:   map[string][]models.PolicyRef{},
		NodeRules:      map[string]models.NodeRules{},
		Nodes:          map[string]models.WorkloadNode{},
	}
	
	for _, ns := range namespaces {
		var nsLabels map[string]string
		if nsNode := index[ns].NSNode; nsNode != nil {
			nsLabels = nsNode.Labels
		}
		cache := buildNsSelection(policies, nsLabels)
		if len(cache.policies) == 0 {
			continue
		}
		for _, node := range index[ns].Workloads {
			s.resolveWorkload(cache, node, ns, &result)
		}
	}

	return result, nil
}

// resolveWorkload runs the full pipeline for one node and folds it into result:
// select governing policies → first-match resolve per direction → build rules +
// CIDR nodes → group by direction/action → derive status. Workloads with no
// selecting policy leave no calico trace — other engines still speak.
func (s *source) resolveWorkload(cache nsSelection, node models.WorkloadNode, ns string, result *models.EvaluationResult) {
	selecting := workloadSelectingPolicies(cache.policies, node)
	if len(selecting) == 0 {
		return
	}

	egressEdges := resolveDirection(node, models.DirectionEgress, cache.egressBuckets, cache.egressCandidates)
	ingressEdges := resolveDirection(node, models.DirectionIngress, cache.ingressBuckets, cache.ingressCandidates)
	egressBucket, egressNodes := buildRulesForWorkload(node, egressEdges, models.DirectionEgress)
	ingressBucket, ingressNodes := buildRulesForWorkload(node, ingressEdges, models.DirectionIngress)

	// Baseline verdict per direction. Governed but no direction-wide catch-all
	// resolved → Calico default-denies everything not explicitly allowed;
	// synthesize CoverageDenyAll so reach detection doesn't read the gap as "no
	// opinion" (mirrors k8s empty-rules deny). Not governed → CoverageUnenforced
	// marker, action-neutral, lands in Allow to match prior zero-value behavior.
	if anyGoverns(selecting, models.DirectionEgress) {
		if !hasCatchAll(egressEdges) {
			egressBucket.Deny = append(egressBucket.Deny, denyAllBaseline(models.DirectionEgress, node, selecting[0].ref))
		}
	} else {
		egressBucket.Allow = append(egressBucket.Allow, unenforcedRule(models.DirectionEgress, selecting[0].ref))
	}
	if anyGoverns(selecting, models.DirectionIngress) {
		if !hasCatchAll(ingressEdges) {
			ingressBucket.Deny = append(ingressBucket.Deny, denyAllBaseline(models.DirectionIngress, node, selecting[0].ref))
		}
	} else {
		ingressBucket.Allow = append(ingressBucket.Allow, unenforcedRule(models.DirectionIngress, selecting[0].ref))
	}

	// synthetic CIDR endpoints from both directions
	for id, cidrNode := range egressNodes {
		result.Nodes[id] = cidrNode
	}
	for id, cidrNode := range ingressNodes {
		result.Nodes[id] = cidrNode
	}

	nodeRules := models.NodeRules{Egress: egressBucket, Ingress: ingressBucket}
	result.NodeRules[node.ID] = nodeRules

	// ns-level slices = grouped rules flattened across direction
	result.AllowByNs[ns] = append(result.AllowByNs[ns], nodeRules.Egress.Allow...)
	result.AllowByNs[ns] = append(result.AllowByNs[ns], nodeRules.Ingress.Allow...)
	result.DenyByNs[ns] = append(result.DenyByNs[ns], nodeRules.Egress.Deny...)
	result.DenyByNs[ns] = append(result.DenyByNs[ns], nodeRules.Ingress.Deny...)

	result.PolicyStatuses[node.ID] = buildPolicyStatus(selecting, egressEdges, ingressEdges)
	if refs := policyRefs(selecting); len(refs) > 0 {
		result.NodePolicies[node.ID] = refs
	}
}

// anyGoverns — does any selecting policy set doesEgress/doesIngress for the
// direction. If none do, calico has no opinion on that direction.
func anyGoverns(selecting []globalPolicy, direction models.Direction) bool {
	for _, gp := range selecting {
		if direction == models.DirectionEgress && gp.doesEgress {
			return true
		}
		if direction == models.DirectionIngress && gp.doesIngress {
			return true
		}
	}
	return false
}

// hasCatchAll — did the first-match walk resolve a direction-wide verdict
// (catch-all CIDR, all ports). When true the catch-all bucket already carries
// the baseline (AllowAll or DenyAll); when false the direction has only narrow
// or port-scoped verdicts and needs an explicit default-deny baseline.
func hasCatchAll(edges []resolvedEdge) bool {
	for _, edge := range edges {
		if edge.peer.kind == peerCIDR && edge.peer.cidr == catchAllCIDR && edge.port == (models.Port{}) {
			return true
		}
	}
	return false
}

// denyAllBaseline — synthetic direction-wide deny for a governed direction with
// no catch-all verdict. Calico default-denies anything not explicitly allowed;
// without this the graph shows only the narrow allows and reads as "no opinion"
// on the rest. Endpoint follows the blanket convention: workload on its own
// side, peer side empty (no CIDR node).
func denyAllBaseline(direction models.Direction, node models.WorkloadNode, contributor models.PolicyRef) models.Rule {
	rule := models.Rule{
		Direction:   direction,
		Action:      models.ActionDeny,
		Coverage:    models.CoverageDenyAll,
		AllPorts:    true,
		Contributor: contributor,
	}
	if direction == models.DirectionIngress {
		rule.DstID = node.ID
	} else {
		rule.SrcID = node.ID
	}
	return rule
}

// unenforcedRule — synthetic marker Rule signalling calico selects the
// workload but doesn't restrict this direction. Contributor points to the
// first selecting policy for provenance; SrcID/DstID stay empty.
func unenforcedRule(direction models.Direction, contributor models.PolicyRef) models.Rule {
	return models.Rule{
		Direction:   direction,
		Coverage:    models.CoverageUnenforced,
		Contributor: contributor,
	}
}

// getPolicies fetches every GlobalNetworkPolicy (cluster-scoped) and normalizes
// each into a globalPolicy. Decode errors fail loud — the PolicySource contract
// requires surfacing them rather than rendering a partial graph. Returns nothing
// on clusters without Calico (client returns nil).
func (s *source) getPolicies(_ context.Context) ([]globalPolicy, error) {
	raw, err := s.client.GetGlobalNetworkPolicies()
	if err != nil {
		return nil, err
	}
	out := make([]globalPolicy, 0, len(raw))
	for _, gnp := range raw {
		gp, err := toGlobalPolicy(gnp)
		if err != nil {
			return nil, fmt.Errorf("calico GlobalNetworkPolicy %q: %w", gnp.Name, err)
		}
		out = append(out, gp)
	}
	return out, nil
}
