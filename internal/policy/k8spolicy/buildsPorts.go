package k8spolicy

import (
	"alatyr/internal/models"

	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

// convertPorts translates rule-level NetworkPolicyPort entries into models.Port.
// Empty input returns nil — expandPeerRules expands nil into a single all-ports rule.
func convertPorts(rulePorts []networkingv1.NetworkPolicyPort) []models.Port {
	if len(rulePorts) == 0 {
		return nil
	}
	out := make([]models.Port, 0, len(rulePorts))
	for _, rulePort := range rulePorts {
		port := models.Port{Protocol: "TCP"}
		if rulePort.Protocol != nil {
			port.Protocol = string(*rulePort.Protocol)
		}
		if rulePort.Port != nil {
			if rulePort.Port.Type == intstr.String {
				port.Name = rulePort.Port.StrVal
			} else {
				port.Port = int(rulePort.Port.IntVal)
			}
		}
		if rulePort.EndPort != nil {
			port.EndPort = int(*rulePort.EndPort)
		}
		out = append(out, port)
	}
	return out
}
