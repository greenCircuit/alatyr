package policy

import (
	"net"

	"alatyr/internal/config"
	"alatyr/internal/models"

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
		if CidrContains(c, block.CIDR) {
			return true
		}
	}
	return false
}

// IsIpBlockClusterInternal reports whether the IPBlock lives inside the cluster
// pod or service CIDR.
func IsIpBlockClusterInternal(block networkingv1.IPBlock) bool {
	cfg := config.Get()
	return CidrContains(cfg.PodCIDR, block.CIDR) || CidrContains(cfg.SvcCIDR, block.CIDR)
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

// CidrContains reports whether outerCIDR fully contains innerCIDR. Empty or
// unparseable inputs return false. Containment requires the outer prefix to be
// no longer than the inner prefix — otherwise a narrower /24 would be judged
// to "contain" a broader /16 that happens to share its base IP.
func CidrContains(outerCIDR, innerCIDR string) bool {
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
	outerBits, _ := outerNet.Mask.Size()
	innerBits, _ := innerNet.Mask.Size()
	if outerBits > innerBits {
		return false
	}
	return outerNet.Contains(innerNet.IP)
}

// CidrType classifies a CIDR string into the cluster-relevant bucket it belongs
// to: the whole-internet catch-all, the pod range, the service range, or a plain
// LAN/external range. Containment, not string equality: a peer of 10.0.0.0/8
// still counts as covering a pod CIDR of 10.244.0.0/16.
func CidrType(compareCIDR string) models.CidrType {
	cfg := config.Get()
	switch {
	case CidrContains(compareCIDR, "0.0.0.0/0"):
		return models.CIDRWan
	case CidrContains(compareCIDR, cfg.PodCIDR):
		return models.CIDRk8sPod
	case CidrContains(compareCIDR, cfg.SvcCIDR):
		return models.CIDRk8sSvc
	default:
		return models.CIDRLan

	}
}
