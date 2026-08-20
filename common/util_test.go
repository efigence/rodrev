package common

import (
	"encoding/json"
	"net/url"
	"testing"
	"time"

	"github.com/efigence/rodrev/config"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zerosvc/go-zerosvc"
)

func TestRedactURL(t *testing.T) {
	assert := assert.New(t)
	assert.Equal("", RedactURL(""))
	assert.Equal(
		"tcp://mqtt:xxxxx@127.0.0.1:1883",
		RedactURL("tcp://mqtt:mqtt@127.0.0.1:1883"),
	)
	assert.Equal(
		"tls://user:xxxxx@mq.example.com:8883/?ca=%2Fetc%2Fssl%2Fca.crt",
		RedactURL("tls://user:s3cr3t@mq.example.com:8883/?ca=%2Fetc%2Fssl%2Fca.crt"),
	)
	// no credentials to redact
	assert.Equal("tls://mq.example.com:8883", RedactURL("tls://mq.example.com:8883"))
	assert.Equal("tls://user@mq.example.com:8883", RedactURL("tls://user@mq.example.com:8883"))
	assert.Equal("[unparseable url]", RedactURL("tcp://mqtt:p%ss@127.0.0.1:1883"))
}

// testCmd is a command with the flags MergeCliConfig reads
func testCmd(mqttURL string) *cobra.Command {
	cmd := &cobra.Command{Use: "test"}
	cmd.Flags().String("mqtt-url", mqttURL, "")
	return cmd
}

func TestMergeCliConfig(t *testing.T) {
	t.Run("nothing configured falls back to localhost", func(t *testing.T) {
		cfg := config.Config{}
		MergeCliConfig(&cfg, testCmd(""))
		assert.Equal(t, "tcp://mqtt:mqtt@127.0.0.1:1883/", cfg.MQAddress)
	})
	t.Run("the flag wins over the config file", func(t *testing.T) {
		cfg := config.Config{MQAddress: "tcp://fromfile:1883"}
		MergeCliConfig(&cfg, testCmd("tcp://fromflag:1883"))
		assert.Contains(t, cfg.MQAddress, "fromflag")
		assert.NotContains(t, cfg.MQAddress, "fromfile")
	})
	// certificate paths are configured separately but have to end up in the url,
	// because that is where the transport looks for them
	t.Run("ca and cert land in the url", func(t *testing.T) {
		cfg := config.Config{
			MQAddress:  "tls://mq.example.com:8883",
			CA:         "/etc/rodrev/certs/ca.pem",
			ClientCert: "/etc/rodrev/certs/client.pem",
		}
		MergeCliConfig(&cfg, testCmd(""))
		u, err := url.Parse(cfg.MQAddress)
		require.NoError(t, err)
		assert.Equal(t, "/etc/rodrev/certs/ca.pem", u.Query().Get("ca"))
		assert.Equal(t, "/etc/rodrev/certs/client.pem", u.Query().Get("cert"))
		assert.Equal(t, "mq.example.com:8883", u.Host)
	})
	t.Run("paths already in the url are kept", func(t *testing.T) {
		cfg := config.Config{
			MQAddress: "tls://mq.example.com:8883/?ca=%2Ffrom%2Furl%2Fca.pem",
			CA:        "/from/config/ca.pem",
		}
		MergeCliConfig(&cfg, testCmd(""))
		u, err := url.Parse(cfg.MQAddress)
		require.NoError(t, err)
		assert.Equal(t, "/from/url/ca.pem", u.Query().Get("ca"))
	})
	t.Run("a path with spaces survives", func(t *testing.T) {
		cfg := config.Config{
			MQAddress: "tls://mq.example.com:8883",
			CA:        "/etc/my certs/ca.pem",
		}
		MergeCliConfig(&cfg, testCmd(""))
		u, err := url.Parse(cfg.MQAddress)
		require.NoError(t, err)
		assert.Equal(t, "/etc/my certs/ca.pem", u.Query().Get("ca"))
	})
	t.Run("credentials are kept", func(t *testing.T) {
		cfg := config.Config{MQAddress: "tcp://user:pass@mq.example.com:1883", CA: "/ca.pem"}
		MergeCliConfig(&cfg, testCmd(""))
		u, err := url.Parse(cfg.MQAddress)
		require.NoError(t, err)
		pass, set := u.User.Password()
		assert.True(t, set)
		assert.Equal(t, "pass", pass)
		assert.Equal(t, "user", u.User.Username())
	})
}

// the transport only loads certificates for ssl://, while rodrev configs have
// always said tls://
func TestTransportURL(t *testing.T) {
	tests := []struct {
		name    string
		address string
		scheme  string
		wantErr bool
	}{
		{name: "tls becomes ssl", address: "tls://mq.example.com:8883", scheme: "ssl"},
		{name: "ssl stays ssl", address: "ssl://mq.example.com:8883", scheme: "ssl"},
		{name: "tcp is left alone", address: "tcp://mq.example.com:1883", scheme: "tcp"},
		{name: "empty", address: "", wantErr: true},
		{name: "unparseable", address: "tcp://mqtt:p%ss@127.0.0.1:1883", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			u, err := TransportURL(tt.address)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.scheme, u.Scheme)
		})
	}
	t.Run("query options are kept", func(t *testing.T) {
		u, err := TransportURL("tls://mq.example.com:8883/?ca=%2Fca.pem&cert=%2Fclient.pem")
		require.NoError(t, err)
		assert.Equal(t, "/ca.pem", u.Query().Get("ca"))
		assert.Equal(t, "/client.pem", u.Query().Get("cert"))
	})
	// an error message must not leak the password
	t.Run("errors are redacted", func(t *testing.T) {
		_, err := TransportURL("tcp://user:s3cr3t%@mq.example.com:1883")
		require.Error(t, err)
		assert.NotContains(t, err.Error(), "s3cr3t")
	})
}

func TestEventRoot(t *testing.T) {
	assert.Equal(t, "rv", EventRoot("rv/"))
	assert.Equal(t, "rv", EventRoot("rv"))
	assert.Equal(t, "site/rv", EventRoot("site/rv/"))
	assert.Equal(t, "", EventRoot(""))
}

func TestParseRodrevService(t *testing.T) {
	t.Run("what a daemon publishes", func(t *testing.T) {
		// Data goes through json on the wire, so it comes back as a generic map
		raw, err := json.Marshal(zerosvc.Service{Ok: true, Data: RodrevService{
			FQDN:              "d1-lg.example.com",
			Version:           "1.2.3",
			Features:          []string{"status", "query"},
			HeartbeatInterval: time.Minute,
		}})
		require.NoError(t, err)
		var svc zerosvc.Service
		require.NoError(t, json.Unmarshal(raw, &svc))

		got, ok := ParseRodrevService(svc)
		require.True(t, ok)
		assert.Equal(t, "d1-lg.example.com", got.FQDN)
		assert.Equal(t, "1.2.3", got.Version)
		assert.Equal(t, []string{"status", "query"}, got.Features)
		assert.Equal(t, time.Minute, got.HeartbeatInterval)
	})
	t.Run("no data at all", func(t *testing.T) {
		_, ok := ParseRodrevService(zerosvc.Service{Ok: true})
		assert.False(t, ok)
	})
	t.Run("data without an fqdn is not usable", func(t *testing.T) {
		_, ok := ParseRodrevService(zerosvc.Service{Data: map[string]interface{}{"version": "1.2.3"}})
		assert.False(t, ok)
	})
	t.Run("data of the wrong shape", func(t *testing.T) {
		_, ok := ParseRodrevService(zerosvc.Service{Data: "just a string"})
		assert.False(t, ok)
	})
}

func TestRandomToken(t *testing.T) {
	seen := make(map[string]bool, 100)
	for i := 0; i < 100; i++ {
		token := RandomToken(8)
		assert.NotEmpty(t, token)
		// has to be usable in a topic and in an mqtt client id
		assert.NotContains(t, token, "/")
		assert.NotContains(t, token, "+")
		assert.NotContains(t, token, "#")
		assert.NotContains(t, token, "=")
		seen[token] = true
	}
	assert.Len(t, seen, 100, "tokens have to be distinct, they keep clients apart")
}

func TestMapBytesToTopicTitle(t *testing.T) {
	// the characters that mean something in a topic must not survive
	out := MapBytesToTopicTitle([]byte{0xff, 0xfe, 0xfd, 0xfc, 0x00, 0x01})
	assert.NotContains(t, out, "/")
	assert.NotContains(t, out, "+")
	assert.NotContains(t, out, "=")
	assert.Equal(t, "", MapBytesToTopicTitle([]byte{}))
}
