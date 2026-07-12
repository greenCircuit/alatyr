package store

import (
	"graph/internal/models"
)

// GetNodeData returns per-engine NodeInfo for the workload. In same ns only
func GetNodeData(data *models.Cache, nodeId string, ns string) map[string]models.NodeInfo {
	idIndex := data.WorkloadByID
	out := map[string]models.NodeInfo{}
	for engineName, engineEvaluate := range data.EvaluationResults {
		var policyRules []models.NodeRule
		for _, rule := range engineEvaluate.AllowByNs[ns] {
			if rule.SrcID == nodeId {
				policyRules = append(policyRules, toNodeRule(rule, idIndex))
			}
		}
		for _, rule := range engineEvaluate.DenyByNs[ns] {
			if rule.SrcID == nodeId {
				policyRules = append(policyRules, toNodeRule(rule, idIndex))
			}
		}
		policies := engineEvaluate.NodePolicies[nodeId]
		if len(policyRules) == 0 && len(policies) == 0 {
			continue
		}
		out[engineName] = models.NodeInfo{Rules: policyRules, Policies: policies}
	}
	return out
}
