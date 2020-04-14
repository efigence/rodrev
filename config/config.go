package config

import (
	"fmt"
	"gopkg.in/yaml.v3"
	"strings"
)

type Config struct {
	MQPrefix   string `yaml:"mq_prefix,omitempty"`
	MQAddress  string `yaml:"mq_address,omitempty"`
	CA         string `yaml:"ca_certs,omitempty"`
	ClientCert string `yaml:"client_cert,omitempty"`
	configPath string
}

func (c *Config) GetDefaultConfig() string {
	defaultCfg := Config{
		MQPrefix:   "rv/",
		MQAddress:  "tls://mq.example.com:8883",
		ClientCert: "/path/to/certandkey.pem",
	}
	outB, err := yaml.Marshal(&defaultCfg)
	out := string(outB)
	if err != nil {
		panic(fmt.Errorf("can't marshal [%T- %+v] into YAML: %s", defaultCfg, defaultCfg, err))
	}
	out = out + "# " +
		strings.Join([]string{
			"by default use system ca, to specify own:",
			"ca_certs: /path/to/ca_bundle.crt",
		}, "\n# ") +
		"\n"
	return out
}

func (c *Config) SetConfigPath(s string) {
	c.configPath = s
}
func (c *Config) GetConfigPath() string {
	return c.configPath
}
