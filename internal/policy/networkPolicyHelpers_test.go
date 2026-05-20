package policy

import (
	"testing"

	"graph/internal/config"
	networkingv1 "k8s.io/api/networking/v1"
)

func block(cidr string) networkingv1.IPBlock {
	return networkingv1.IPBlock{CIDR: cidr}
}

func withConfig(t *testing.T, cfg config.Config) {
	t.Helper()
	prev := config.Get()
	config.Set(cfg)
	t.Cleanup(func() { config.Set(prev) })
}

func TestIsIpBlockInternetAccess(t *testing.T) {
	cases := []struct {
		cidr string
		want bool
	}{
		{"0.0.0.0/0", true},
		{"8.8.8.8/32", false},
		{"10.0.0.0/8", false},
		{"", false},
	}
	for _, tc := range cases {
		if got := IsIpBlockInternetAccess(block(tc.cidr)); got != tc.want {
			t.Errorf("IsIpBlockInternetAccess(%q) = %v, want %v", tc.cidr, got, tc.want)
		}
	}
}

func TestIsIpBlockClusterInternal_Containment(t *testing.T) {
	withConfig(t, config.Config{PodCIDR: "10.42.0.0/16", SvcCIDR: "10.43.0.0/16"})
	cases := []struct {
		cidr string
		want bool
	}{
		{"10.42.0.0/24", true},  // inside pod CIDR
		{"10.42.5.10/32", true}, // single IP inside pod CIDR
		{"10.43.0.0/24", true},  // inside svc CIDR
		{"10.50.0.0/24", false}, // outside both
		{"0.0.0.0/0", false},
	}
	for _, tc := range cases {
		if got := IsIpBlockClusterInternal(block(tc.cidr)); got != tc.want {
			t.Errorf("IsIpBlockClusterInternal(%q) = %v, want %v", tc.cidr, got, tc.want)
		}
	}
}

func TestIsIpBlockApiServerAccess(t *testing.T) {
	withConfig(t, config.Config{ApiServerCIDRs: []string{"192.168.1.10/32", "10.100.0.0/16"}})
	cases := []struct {
		cidr string
		want bool
	}{
		{"192.168.1.10/32", true},
		{"10.100.5.20/32", true}, // inside the /16
		{"192.168.1.11/32", false},
		{"8.8.8.8/32", false},
	}
	for _, tc := range cases {
		if got := IsIpBlockApiServerAccess(block(tc.cidr)); got != tc.want {
			t.Errorf("IsIpBlockApiServerAccess(%q) = %v, want %v", tc.cidr, got, tc.want)
		}
	}
}

func TestIsIpBlockLanAccess_ExcludesClusterAndApiServer(t *testing.T) {
	withConfig(t, config.Config{
		PodCIDR:        "10.42.0.0/16",
		SvcCIDR:        "10.43.0.0/16",
		ApiServerCIDRs: []string{"192.168.1.10/32"},
	})
	cases := []struct {
		cidr string
		want bool
	}{
		{"192.168.5.0/24", true},  // generic LAN
		{"172.16.0.0/12", true},   // RFC1918
		{"10.42.0.0/24", false},   // pod CIDR
		{"10.43.0.0/24", false},   // svc CIDR
		{"192.168.1.10/32", false}, // api server
		{"0.0.0.0/0", false},      // internet
	}
	for _, tc := range cases {
		if got := IsIpBlockLanAccess(block(tc.cidr)); got != tc.want {
			t.Errorf("IsIpBlockLanAccess(%q) = %v, want %v", tc.cidr, got, tc.want)
		}
	}
}
