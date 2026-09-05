package calico

import (
	"fmt"
	"strings"
	"time"

	"alatyr/internal/models"

	calicov3 "github.com/projectcalico/api/pkg/apis/projectcalico/v3"
	"github.com/projectcalico/api/pkg/lib/numorstring"
)

// toGlobalPolicy normalizes a decoded GlobalNetworkPolicy CRD into the engine's
// globalPolicy. Selectors parse once here (memoized as closures); rules flatten
// to calicoRule carrying the peer EntityRule's nets/notNets/ports. GNP is
// cluster-scoped — namespace stays "". Errors fail loud: a policy we can't fully
// model must not silently under-report.
func toGlobalPolicy(gnp *calicov3.GlobalNetworkPolicy) (globalPolicy, error) {
	spec := gnp.Spec

	selectorMatch, err := parseSelector(spec.Selector)
	if err != nil {
		return globalPolicy{}, err
	}

	// Empty NamespaceSelector = all namespaces (Calico GNP semantics). The nil
	// matcher is the "match all ns" sentinel the ns-filter expects.
	var nsSelectorMatch workloadMatcher
	if strings.TrimSpace(spec.NamespaceSelector) != "" {
		nsSelectorMatch, err = parseSelector(spec.NamespaceSelector)
		if err != nil {
			return globalPolicy{}, err
		}
	}

	ingress, err := convertRules(spec.Ingress, models.DirectionIngress)
	if err != nil {
		return globalPolicy{}, err
	}
	egress, err := convertRules(spec.Egress, models.DirectionEgress)
	if err != nil {
		return globalPolicy{}, err
	}

	doesIngress, doesEgress := deriveTypes(spec.Types, spec.Ingress, spec.Egress)

	tier := spec.Tier
	if tier == "" {
		tier = "default"
	}

	return globalPolicy{
		name:                gnp.Name,
		order:               normalizeOrder(spec.Order),
		tier:                tier,
		ingress:             ingress,
		egress:              egress,
		doesIngress:         doesIngress,
		doesEgress:          doesEgress,
		selectorMatch:       selectorMatch,
		nsSelectorMatch:     nsSelectorMatch,
		selectsAllWorkloads: isCatchAllSelector(spec.Selector),
		ref: models.PolicyRef{
			Source:    sourceName,
			Name:      gnp.Name,
			CreatedAt: creationTime(gnp),
			Order:     spec.Order, // raw *float64; nil preserved via omitempty
			Tier:      tier,       // "" defaulted to "default" above
		},
	}, nil
}

// convertRules flattens Calico ingress/egress rules to calicoRule. The peer is
// the Destination EntityRule for egress, Source for ingress — the side whose
// nets/ports the engine resolves against. Log rules drop out (Calico logs and
// continues; no verdict). Only nets/notNets/ports carry through; label-scoped
// peers fail loud in checkPeerSupported rather than collapse to catch-all.
func convertRules(rules []calicov3.Rule, direction models.Direction) ([]calicoRule, error) {
	out := make([]calicoRule, 0, len(rules))
	for index, rule := range rules {
		if rule.Action == calicov3.Log {
			continue // non-terminal, verdict-neutral
		}
		peer := rule.Destination
		if direction == models.DirectionIngress {
			peer = rule.Source
		}
		if err := checkPeerSupported(peer, direction, index); err != nil {
			return nil, err
		}
		ports, err := convertPorts(peer.Ports, rule.Protocol)
		if err != nil {
			return nil, err
		}
		out = append(out, calicoRule{
			action:  mapAction(rule.Action),
			isPass:  rule.Action == calicov3.Pass,
			nets:    peer.Nets,
			notNets: peer.NotNets,
			ports:   ports,
		})
	}
	return out, nil
}

// checkPeerSupported rejects rule peers this engine can't resolve. The bucket
// model keys on (cidr,port), so a label-scoped peer has no CIDR to land in —
// letting it through with empty nets would render as match-everything and
// invert the verdict. all() and empty selectors are genuinely unconstrained,
// so they pass. Scope for now: nets/notNets/ports only.
func checkPeerSupported(peer calicov3.EntityRule, direction models.Direction, index int) error {
	unsupported := ""
	switch {
	case !isCatchAllSelector(peer.Selector):
		unsupported = fmt.Sprintf("selector %q", peer.Selector)
	case strings.TrimSpace(peer.NotSelector) != "":
		unsupported = fmt.Sprintf("notSelector %q", peer.NotSelector)
	case !isCatchAllSelector(peer.NamespaceSelector):
		unsupported = fmt.Sprintf("namespaceSelector %q", peer.NamespaceSelector)
	case peer.ServiceAccounts != nil:
		unsupported = "serviceAccounts"
	case peer.Services != nil:
		unsupported = "services"
	case len(peer.NotPorts) > 0:
		unsupported = "notPorts"
	}
	if unsupported == "" {
		return nil
	}
	return fmt.Errorf("%s rule %d: peer %s not yet supported", direction, index, unsupported)
}

// mapAction maps a Calico action to the engine's allow/deny. Pass carries via
// calicoRule.isPass; its action value is unused, so it falls through to Allow.
func mapAction(action calicov3.Action) models.RuleAction {
	if action == calicov3.Deny {
		return models.ActionDeny
	}
	return models.ActionAllow
}

// convertPorts maps Calico numeric ports to models.Port. Named ports and port
// ranges aren't representable in the (cidr,port) bucket model yet — fail loud
// rather than silently broaden the rule to all-ports. Protocol lives on the
// rule in Calico, not the port.
func convertPorts(ports []numorstring.Port, protocol *numorstring.Protocol) ([]models.Port, error) {
	if len(ports) == 0 {
		return nil, nil
	}
	protocolName := ""
	if protocol != nil {
		protocolName = protocol.String()
	}
	out := make([]models.Port, 0, len(ports))
	for _, port := range ports {
		if port.PortName != "" {
			return nil, fmt.Errorf("named port %q not yet supported", port.PortName)
		}
		if port.MinPort != port.MaxPort {
			return nil, fmt.Errorf("port range %d-%d not yet supported", port.MinPort, port.MaxPort)
		}
		out = append(out, models.Port{Port: int(port.MinPort), Protocol: protocolName})
	}
	return out, nil
}

// deriveTypes resolves whether the policy governs ingress/egress. Explicit
// Types win. When empty (possible in hand-written demo YAML; the apiserver
// always populates it) apply Calico's default: Egress when egress rules exist
// and ingress rules don't; Ingress otherwise; both when both are present.
func deriveTypes(types []calicov3.PolicyType, ingress, egress []calicov3.Rule) (doesIngress, doesEgress bool) {
	if len(types) > 0 {
		for _, policyType := range types {
			if policyType == calicov3.PolicyTypeIngress {
				doesIngress = true
			}
			if policyType == calicov3.PolicyTypeEgress {
				doesEgress = true
			}
		}
		return
	}
	if len(egress) > 0 {
		doesEgress = true
	}
	if len(ingress) > 0 || len(egress) == 0 {
		doesIngress = true
	}
	return
}

// creationTime returns the CRD creation timestamp, or nil when unset so
// omitempty drops it rather than marshaling year 0001.
func creationTime(gnp *calicov3.GlobalNetworkPolicy) *time.Time {
	if gnp.CreationTimestamp.IsZero() {
		return nil
	}
	created := gnp.CreationTimestamp.Time
	return &created
}
