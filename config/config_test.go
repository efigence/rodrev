package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/XANi/go-yamlcfg"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// the config file is the only way an operator talks to the daemon, so the
// documented keys have to keep working
func TestLoadConfig(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "server.yaml")
	require.NoError(t, os.WriteFile(path, []byte(`---
mq_prefix: rv/
mq_address: tls://dc1-mq.example.com:8883
ca_certs: /etc/rodrev/certs/ca.pem
client_cert: /etc/rodrev/certs/daemon-client.pem
node_meta:
    fqdn: d1-puppet1.example.com
    site: dc1
fence:
    enabled: true
    group: sql
    group_password: hunter2
    node_map:
        rf-client-node1-fence:
            nodes:
                - node1.example.com
            password: asdg
        rf-client-old-style:
            node:
                - node2.example.com
ipset:
    sets:
        blocked-net:
            type: host:net
            timeout: 1h
heartbeat_cleanup:
    max_age: 240h
    interval: 24h
    dry_run: true
icinga_api_url: https://icinga.example.com:5665
`), 0600))
	var cfg Config
	require.NoError(t, yamlcfg.LoadConfig([]string{path}, &cfg))

	assert.Equal(t, "rv/", cfg.MQPrefix)
	assert.Equal(t, "tls://dc1-mq.example.com:8883", cfg.MQAddress)
	assert.Equal(t, "/etc/rodrev/certs/ca.pem", cfg.CA)
	assert.Equal(t, "/etc/rodrev/certs/daemon-client.pem", cfg.ClientCert)
	assert.Equal(t, "d1-puppet1.example.com", cfg.NodeMeta["fqdn"])
	assert.Equal(t, "dc1", cfg.NodeMeta["site"])
	assert.True(t, cfg.Fence.Enabled)
	assert.Equal(t, "sql", cfg.Fence.Group)
	assert.Equal(t, []string{"node1.example.com"},
		cfg.Fence.NodeMap["rf-client-node1-fence"].AllowedNodes())
	// the singular key earlier versions read still works
	assert.Equal(t, []string{"node2.example.com"},
		cfg.Fence.NodeMap["rf-client-old-style"].AllowedNodes())
	assert.Equal(t, "host:net", cfg.IPSet.Sets["blocked-net"].Type)
	assert.Equal(t, "https://icinga.example.com:5665", cfg.IcingaAPIURL)
	// the config path is remembered, commands print it so an operator knows
	// which file was picked up
	assert.Equal(t, path, cfg.GetConfigPath())

	t.Run("heartbeat cleanup", func(t *testing.T) {
		assert.False(t, cfg.HeartbeatCleanup.Disabled)
		assert.Equal(t, "240h0m0s", cfg.HeartbeatCleanup.MaxAge.String())
		assert.Equal(t, "24h0m0s", cfg.HeartbeatCleanup.Interval.String())
		assert.True(t, cfg.HeartbeatCleanup.DryRun)
		// unset means the daemon default applies, not zero
		assert.Zero(t, cfg.HeartbeatCleanup.InitialDelay)
	})
}

func TestLoadConfigErrors(t *testing.T) {
	dir := t.TempDir()
	// there is a sample config to fall back on, so a missing file is not an
	// error: the first candidate path gets created from it
	t.Run("missing config is created from the sample", func(t *testing.T) {
		path := filepath.Join(dir, "created", "server.yaml")
		var cfg Config
		require.NoError(t, yamlcfg.LoadConfig([]string{path}, &cfg))
		assert.FileExists(t, path)
		assert.Equal(t, "rv/", cfg.MQPrefix)
		st, err := os.Stat(path)
		require.NoError(t, err)
		assert.Equal(t, os.FileMode(0600), st.Mode().Perm(),
			"it can hold mq credentials, so it must not be world readable")
	})
	t.Run("not yaml", func(t *testing.T) {
		path := filepath.Join(dir, "broken.yaml")
		require.NoError(t, os.WriteFile(path, []byte("\tnot: [yaml"), 0600))
		var cfg Config
		assert.Error(t, yamlcfg.LoadConfig([]string{path}, &cfg))
	})
	t.Run("wrong type for a known key", func(t *testing.T) {
		path := filepath.Join(dir, "wrongtype.yaml")
		require.NoError(t, os.WriteFile(path, []byte("---\nmq_prefix: [a, b]\n"), 0600))
		var cfg Config
		assert.Error(t, yamlcfg.LoadConfig([]string{path}, &cfg))
	})
}

// the sample config is what an operator starts from, so it has to be valid
func TestGetDefaultConfig(t *testing.T) {
	var cfg Config
	sample := cfg.GetDefaultConfig()
	assert.Contains(t, sample, "mq_prefix: rv/")
	assert.Contains(t, sample, "ca_certs")

	var parsed Config
	require.NoError(t, yaml.Unmarshal([]byte(sample), &parsed))
	assert.Equal(t, "rv/", parsed.MQPrefix)
	assert.Equal(t, "tls://mq.example.com:8883", parsed.MQAddress)
}

func TestFenceNodeAllowedNodes(t *testing.T) {
	assert.Equal(t, []string{"a"}, FenceNode{Nodes: []string{"a"}}.AllowedNodes())
	assert.Equal(t, []string{"a"}, FenceNode{Node: []string{"a"}}.AllowedNodes())
	// a config using both spellings gets both, rather than silently losing one
	assert.ElementsMatch(t, []string{"a", "b"},
		FenceNode{Nodes: []string{"a"}, Node: []string{"b"}}.AllowedNodes())
	assert.Empty(t, FenceNode{}.AllowedNodes())
}

func TestSetConfigPath(t *testing.T) {
	var cfg Config
	assert.Equal(t, "", cfg.GetConfigPath())
	cfg.SetConfigPath("/etc/rodrev/server.conf")
	assert.Equal(t, "/etc/rodrev/server.conf", cfg.GetConfigPath())
}
