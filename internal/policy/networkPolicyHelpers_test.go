package policy

import (
	"testing"

	"alatyr/internal/config"
	"alatyr/internal/models"

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

func TestCidrType(t *testing.T) {
	withConfig(t, config.Config{PodCIDR: "10.244.0.0/16", SvcCIDR: "10.96.0.0/12"})
	cases := []struct {
		name string
		cidr string
		want models.CidrType
	}{
		{"0.0.0.0/0 is the wan catch-all", "0.0.0.0/0", models.CIDRWan},
		{"exact pod CIDR", "10.244.0.0/16", models.CIDRk8sPod},
		{"exact svc CIDR", "10.96.0.0/12", models.CIDRk8sSvc},
		{"broader-than-pod CIDR still classifies as pod (10.0.0.0/8 ⊃ 10.244.0.0/16)", "10.0.0.0/8", models.CIDRk8sPod},
		{"narrower-than-pod CIDR is not pod-covering (10.244.0.0/24 ⊂ /16)", "10.244.0.0/24", models.CIDRLan},
		{"unrelated RFC1918 range is lan", "172.16.0.0/12", models.CIDRLan},
		{"external CIDR is lan", "203.0.113.0/24", models.CIDRLan},
	}
	for _, testCase := range cases {
		if got := CidrType(testCase.cidr); got != testCase.want {
			t.Errorf("%s: CidrType(%q) = %v, want %v", testCase.name, testCase.cidr, got, testCase.want)
		}
	}
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
		{"192.168.5.0/24", true},   // generic LAN
		{"172.16.0.0/12", true},    // RFC1918
		{"10.42.0.0/24", false},    // pod CIDR
		{"10.43.0.0/24", false},    // svc CIDR
		{"192.168.1.10/32", false}, // api server
		{"0.0.0.0/0", false},       // internet
	}
	for _, tc := range cases {
		if got := IsIpBlockLanAccess(block(tc.cidr)); got != tc.want {
			t.Errorf("IsIpBlockLanAccess(%q) = %v, want %v", tc.cidr, got, tc.want)
		}
	}
}
