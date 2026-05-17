package graph

import (
	"net"

	"graph/internal/config"
	networkingv1 "k8s.io/api/networking/v1"
)

// IsIpBlockInternetAccess reports whether the IPBlock targets the public internet.
// First iteration: only the catch-all 0.0.0.0/0 counts as internet. Other public
// CIDRs fall into the LAN bucket until a richer classifier lands.
func IsIpBlockInternetAccess(block networkingv1.IPBlock) bool {
	return block.CIDR == "0.0.0.0/0"
}

// IsIpBlockApiServerAccess reports whether the IPBlock targets the Kubernetes API
// server, based on apiServerCIDRs in config. Matches by subnet containment so a
// block of /24 inside an api-server /16 still counts.
func IsIpBlockApiServerAccess(block networkingv1.IPBlock) bool {
	cfg := config.Get()
	for _, c := range cfg.ApiServerCIDRs {
		if cidrContains(c, block.CIDR) {
			return true
		}
	}
	return false
}

// IsIpBlockClusterInternal reports whether the IPBlock lives inside the cluster
// pod or service CIDR.
func IsIpBlockClusterInternal(block networkingv1.IPBlock) bool {
	cfg := config.Get()
	return cidrContains(cfg.PodCIDR, block.CIDR) || cidrContains(cfg.SvcCIDR, block.CIDR)
}

// IsIpBlockLanAccess reports whether the IPBlock targets a private LAN outside
// the cluster: anything that is not 0.0.0.0/0, not cluster-internal, and not the
// API server.
func IsIpBlockLanAccess(block networkingv1.IPBlock) bool {
	if block.CIDR == "0.0.0.0/0" {
		return false
	}
	if IsIpBlockClusterInternal(block) {
		return false
	}
	if IsIpBlockApiServerAccess(block) {
		return false
	}
	return true
}

// cidrContains reports whether outerCIDR fully contains innerCIDR. Empty or
// unparseable inputs return false.
func cidrContains(outerCIDR, innerCIDR string) bool {
	if outerCIDR == "" || innerCIDR == "" {
		return false
	}
	_, outerNet, err := net.ParseCIDR(outerCIDR)
	if err != nil {
		return false
	}
	_, innerNet, err := net.ParseCIDR(innerCIDR)
	if err != nil {
		return false
	}
	return outerNet.Contains(innerNet.IP)
}
