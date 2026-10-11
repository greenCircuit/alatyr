package store

import (
	"alatyr/internal/models"
)

type layeringFind struct {
	ingressAllowed []models.PolicyRef
	egressAllowed  []models.PolicyRef
	message        string
}

// in node is a cidr and is part of k8s network CIDR range
func isInternalCidrAggregate(node models.WorkloadNode) bool {
	if node.Type != models.NodeTypeCIDR {
		return false
	}
	return node.CidrType == models.CIDRk8sPod || node.CidrType == models.CIDRk8sSvc
}


// layer detection entrypoint
func isLayeredPolicy(prober *reachabilityProber, engine string, srcNode, dstNode models.WorkloadNode) (bool, layeringFind) {
	switch {
	// first: cidr node carries no ns, so the ns case below would find zero
	// evidence and report a conflict it never checked
	case isInternalCidrAggregate(srcNode) || isInternalCidrAggregate(dstNode):
		return true, layeringFind{
			message: engine + " has no rule naming this cluster range, CIDR targets pods and namespaces, not external CIDRs",
		}

	// wide ns policy and fine grained one to workload to ns
	case srcNode.Type == models.NodeTypeNamespace || dstNode.Type == models.NodeTypeNamespace:
		find, finerRules := finerPolicies(prober.data, engine, srcNode, dstNode)
		if len(find.ingressAllowed) == 0 && len(find.egressAllowed) == 0 {
			return false, layeringFind{}
		}
		// check if workloads are reachable in case there are deny policies
		if !anyRuleReachable(prober, finerRules, srcNode.Namespace, dstNode.Namespace) {
			return false, layeringFind{}
		}

		find.message = engine + " blocks at namespace granularity while its own targeted policies allow workloads across this pair"
		return true, find
	}
	return false, layeringFind{}
}

// one live path is enough, rest denied is least privilege
func anyRuleReachable(prober *reachabilityProber, rules []models.Rule, srcNs, dstNs string) bool {
	for _, rule := range rules {
		if prober.probe(rule.SrcID, srcNs, rule.DstID, dstNs).Verdict != "deny" {
			return true
		}
	}
	return false
}

// check is have fine grained policy to ns, used when have wide ns policy from one engine and fine grained for other
func finerPolicies(data *models.Cache, engine string, srcNode, dstNode models.WorkloadNode) (layeringFind, []models.Rule) {
	eval, ok := data.EvaluationResults[engine]
	if !ok {
		return layeringFind{}, nil
	}
	var findRules layeringFind
	var finer []models.Rule
	seen := map[string]bool{} // direction+identity: policy emits a rule per port and peer
	for _, namespace := range []string{srcNode.Namespace, dstNode.Namespace} {
		if namespace == "" {
			continue
		}
		for _, rule := range eval.AllowByNs[namespace] {
			if rule.Action != models.ActionAllow {
				continue
			}
			ruleSrc, srcFound := data.WorkloadByID[rule.SrcID]
			ruleDst, dstFound := data.WorkloadByID[rule.DstID]

			nodesFound := srcFound && dstFound && ruleSrc.ID != ruleDst.ID
			samePair := ruleSrc.Namespace == srcNode.Namespace && ruleDst.Namespace == dstNode.Namespace
			fineGrained := ruleSrc.Type != models.NodeTypeNamespace || ruleDst.Type != models.NodeTypeNamespace

			if !nodesFound || !samePair || !fineGrained {
				continue
			}
			finer = append(finer, rule)
			key := string(rule.Direction) + "|" + policyIdentityKey(rule.Contributor)
			if seen[key] {
				continue
			}
			seen[key] = true
			if rule.Direction == models.DirectionIngress {
				findRules.ingressAllowed = append(findRules.ingressAllowed, rule.Contributor)
				continue
			}
			findRules.egressAllowed = append(findRules.egressAllowed, rule.Contributor)
		}
	}
	return findRules, finer
}
