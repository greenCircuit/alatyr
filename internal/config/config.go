package config

import (
	_ "embed"
)

//go:embed defaultConfig.yaml
var defaultConfigYAML []byte

var instance Config

// Install sets the package-level Config from an already-loaded value.
// Callers (main, tests) use Load() to build the value and Install() to
// publish it — the error handling stays in the caller with its logger.
func Install(cfg Config) { instance = cfg }

func Get() Config { return instance }

// Set replaces the in-process config. Intended for tests that need to drive
// CIDR-classification helpers with specific values without loading a file.
func Set(cfg Config) { instance = cfg }

type Config struct {
	PodCIDR        string      `mapstructure:"podCIDR"`
	SvcCIDR        string      `mapstructure:"svcCIDR"`
	ApiServerCIDRs []string          `mapstructure:"apiServerCIDRs"`
	DNSNamespace   string            `mapstructure:"dnsNs"`
	DNSLabels      map[string]string `mapstructure:"dnsLabels"`
	CacheRefreshSec int16			 `mapstructure:"cacheRefreshSec"`
}