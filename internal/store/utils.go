package store

import (
	"alatyr/internal/models"
)

func toNodeRule(rule models.Rule, idIndex map[string]models.WorkloadNode) models.NodeRule {
	view := models.NodeRule{
		Direction:   rule.Direction,
		Ports:       rule.Ports,
		L7Match:     rule.L7Match,
		Action:      rule.Action,
		Contributor: rule.Contributor,
		SrcID:       rule.SrcID,
		DstID:       rule.DstID,
		DstSelector: rule.DstSelector,
		SrcSelector: rule.SrcSelector,
	}
	if srcNode, ok := idIndex[rule.SrcID]; ok {
		view.SrcLabel = srcNode.Label
		view.SrcNamespace = srcNode.Namespace
		view.SrcKind = srcNode.Type
	}
	if dstNode, ok := idIndex[rule.DstID]; ok {
		view.DstLabel = dstNode.Label
		view.DstNamespace = dstNode.Namespace
		view.DstKind = dstNode.Type
	}
	return view
}
