package client

import (
	"encoding/json"
	"testing"

	"github.com/efigence/rodrev/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zerosvc/go-zerosvc"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

func testRuntime(t *testing.T) (*common.Runtime, *observer.ObservedLogs) {
	t.Helper()
	core, logs := observer.New(zap.DebugLevel)
	return &common.Runtime{Log: zap.New(core).Sugar()}, logs
}

func heartbeatEvent(t *testing.T, topic string, hb *zerosvc.Heartbeat) zerosvc.Event {
	t.Helper()
	ev := zerosvc.Event{RoutingKey: topic}
	if hb != nil {
		body, err := json.Marshal(hb)
		require.NoError(t, err)
		ev.Body = body
	}
	return ev
}

func TestParseHeartbeat(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		r, _ := testRuntime(t)
		ev := heartbeatEvent(t, "rv/heartbeat/d1-lg.example.com", &zerosvc.Heartbeat{
			NodeName: "d1-lg.example.com",
			NodeInfo: map[string]interface{}{"fqdn": "d1-lg.example.com", "version": "1.2.3"},
			Services: map[string]zerosvc.Service{"puppet": {}, "fence": {}},
		})
		node, services, ok := parseHeartbeat(r, &ev)
		require.True(t, ok)
		assert.Equal(t, "d1-lg.example.com", node.FQDN)
		assert.Equal(t, "1.2.3", node.DaemonVersion)
		assert.Equal(t, []string{"fence", "puppet"}, services)
	})
	// a cleared retained heartbeat arrives with an empty payload, and its
	// node-name header is gone too - the topic is all we have to name the sender
	t.Run("empty body names the node", func(t *testing.T) {
		r, logs := testRuntime(t)
		ev := heartbeatEvent(t, "rv/heartbeat/d2-lg.example.com", nil)
		_, _, ok := parseHeartbeat(r, &ev)
		assert.False(t, ok)
		require.Equal(t, 1, logs.Len())
		msg := logs.All()[0].Message
		assert.Contains(t, msg, "d2-lg.example.com")
		assert.Contains(t, msg, "0 bytes")
		assert.Contains(t, msg, "unexpected end of JSON input")
	})
	t.Run("broken json names the node", func(t *testing.T) {
		r, logs := testRuntime(t)
		ev := zerosvc.Event{RoutingKey: "rv/heartbeat/d3-lg.example.com", Body: []byte(`{"node-name":`)}
		_, _, ok := parseHeartbeat(r, &ev)
		assert.False(t, ok)
		require.Equal(t, 1, logs.Len())
		assert.Contains(t, logs.All()[0].Message, "d3-lg.example.com")
	})
	t.Run("no fqdn names the node", func(t *testing.T) {
		r, logs := testRuntime(t)
		ev := heartbeatEvent(t, "rv/heartbeat/d4-lg.example.com", &zerosvc.Heartbeat{NodeName: "d4"})
		_, _, ok := parseHeartbeat(r, &ev)
		assert.False(t, ok)
		require.Equal(t, 1, logs.Len())
		assert.Contains(t, logs.All()[0].Message, "d4-lg.example.com")
	})
	t.Run("falls back to the node-name header", func(t *testing.T) {
		ev := zerosvc.Event{Headers: map[string]interface{}{"node-name": "from-header"}}
		assert.Equal(t, "from-header", heartbeatSource(&ev))
		assert.Equal(t, "unknown", heartbeatSource(&zerosvc.Event{}))
	})
}
