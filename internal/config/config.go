package config

import (
	_ "embed"
	"log"
)

//go:embed defaultConfig.yaml
var defaultConfigYAML []byte

var instance Config

func MustLoad(path string) {
	cfg, err := Load(path)
	if err != nil {
		log.Fatalf("config: %v", err)
	}
	instance = cfg
}

func Get() Config { return instance }

// Set replaces the in-process config. Intended for tests that need to drive
// CIDR-classification helpers with specific values without loading a file.
func Set(cfg Config) { instance = cfg }

type Config struct {
	PodCIDR        string      `mapstructure:"podCIDR"`
	SvcCIDR        string      `mapstructure:"svcCIDR"`
	ApiServerCIDRs []string    `mapstructure:"apiServerCIDRs"`
	CustomRules    CustomRules `mapstructure:"customRules"`
}

type CustomRules struct {
	Ingress []StatusRule `mapstructure:"ingress"`
	Egress  []StatusRule `mapstructure:"egress"`
}

type StatusRule struct {
	Name     string         `mapstructure:"name"`
	Status   string         `mapstructure:"status"` // "present" or "missing"
	Port     int            `mapstructure:"port"`
	Protocol string         `mapstructure:"protocol"`
	Policies []PolicyPeer   `mapstructure:"policies"`
}

type PolicyPeer struct {
	NamespaceSelector *LabelSelector `mapstructure:"namespaceSelector"`
	PodSelector       *LabelSelector `mapstructure:"podSelector"`
}

type LabelSelector struct {
	MatchLabels map[string]string `mapstructure:"matchLabels"`
}

