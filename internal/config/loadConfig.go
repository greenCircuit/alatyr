package config

import (
	"bytes"

	"github.com/spf13/viper"
)

func Load(overridePath string) (Config, error) {
	v := viper.New()
	v.SetConfigType("yaml")

	if err := v.ReadConfig(bytes.NewReader(defaultConfigYAML)); err != nil {
		return Config{}, err
	}

	if overridePath != "" {
		v.SetConfigFile(overridePath)
		if err := v.MergeInConfig(); err != nil {
			return Config{}, err
		}
	}

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return Config{}, err
	}
	return cfg, nil
}