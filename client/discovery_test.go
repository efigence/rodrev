package client

import (
	"encoding/json"
	"testing"
	"time"

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

// heartbeatMsg builds what a node actually publishes: plain JSON node info,
// with rodrev's own data in its service entry
func heartbeatMsg(t *testing.T, topic string, info *zerosvc.NodeInfo) *zerosvc.Message {
	t.Helper()
	msg := zerosvc.Message{Topic: topic}
	if info != nil {
		payload, err := json.Marshal(info)
		require.NoError(t, err)
		msg.Payload = payload
	}
	return &msg
}

// rodrevInfo is a heartbeat of a rodrev daemon
func rodrevInfo(name string, data common.RodrevService) *zerosvc.NodeInfo {
	return &zerosvc.NodeInfo{
		Name: name,
		UUID: "uuid-" + name,
		TS:   time.Now(),
		Services: map[string]zerosvc.Service{
			common.RodrevServiceName: {Ok: true, Data: data},
			"puppet":                 {Ok: true},
		},
	}
}

func TestParseHeartbeat(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		r, _ := testRuntime(t)
		msg := heartbeatMsg(t, "rv/discovery/d1-lg.example.com/uuid1",
			rodrevInfo("d1-lg.example.com", common.RodrevService{
				FQDN:              "d1-lg.example.com",
				Version:           "1.2.3",
				Features:          []string{"status", "query"},
				HeartbeatInterval: time.Minute,
			}))
		node, services, ok := parseHeartbeat(r, msg)
		require.True(t, ok)
		assert.Equal(t, "d1-lg.example.com", node.FQDN)
		assert.Equal(t, "1.2.3", node.DaemonVersion)
		assert.Equal(t, []string{"status", "query"}, node.Features)
		assert.Equal(t, []string{"puppet", common.RodrevServiceName}, services)
		assert.Equal(t, minStaleAge, node.StaleAge, "a fast heartbeat still gets the minimum grace")
	})
	// a node whose heartbeat was cleared publishes an empty payload. That is a
	// goodbye, not a problem worth warning about
	t.Run("cleared heartbeat is not an error", func(t *testing.T) {
		r, logs := testRuntime(t)
		msg := heartbeatMsg(t, "rv/discovery/d2-lg.example.com/uuid2", nil)
		_, _, ok := parseHeartbeat(r, msg)
		assert.False(t, ok)
		require.Equal(t, 1, logs.Len())
		assert.Equal(t, zap.DebugLevel, logs.All()[0].Level)
		assert.Contains(t, logs.All()[0].Message, "d2-lg.example.com")
	})
	t.Run("broken json names the node", func(t *testing.T) {
		r, logs := testRuntime(t)
		msg := &zerosvc.Message{Topic: "rv/discovery/d3-lg.example.com/uuid3", Payload: []byte(`{"name":`)}
		_, _, ok := parseHeartbeat(r, msg)
		assert.False(t, ok)
		require.Equal(t, 1, logs.Len())
		assert.Contains(t, logs.All()[0].Message, "d3-lg.example.com")
	})
	// this is what a heartbeat of something that is not a rodrev daemon looks
	// like - the cli announces itself on the same topic tree
	t.Run("not a rodrev node is ignored quietly", func(t *testing.T) {
		r, logs := testRuntime(t)
		msg := heartbeatMsg(t, "rv/discovery/rf-client-laptop/uuid4", &zerosvc.NodeInfo{
			Name: "rf-client-laptop",
			TS:   time.Now(),
		})
		_, _, ok := parseHeartbeat(r, msg)
		assert.False(t, ok)
		require.Equal(t, 1, logs.Len())
		assert.Equal(t, zap.DebugLevel, logs.All()[0].Level)
	})
	t.Run("rodrev service without usable data warns", func(t *testing.T) {
		r, logs := testRuntime(t)
		msg := heartbeatMsg(t, "rv/discovery/d5-lg.example.com/uuid5", &zerosvc.NodeInfo{
			Name:     "d5-lg.example.com",
			TS:       time.Now(),
			Services: map[string]zerosvc.Service{common.RodrevServiceName: {Ok: true}},
		})
		_, _, ok := parseHeartbeat(r, msg)
		assert.False(t, ok)
		require.Equal(t, 1, logs.Len())
		assert.Equal(t, zap.WarnLevel, logs.All()[0].Level)
		assert.Contains(t, logs.All()[0].Message, "d5-lg.example.com")
	})
	t.Run("names the sender from the topic", func(t *testing.T) {
		assert.Equal(t, "d1.example.com", heartbeatSource("rv/discovery/d1.example.com/uuid"))
		assert.Equal(t, "unknown", heartbeatSource(""))
	})
	t.Run("a slow heartbeat gets a longer grace", func(t *testing.T) {
		r, _ := testRuntime(t)
		msg := heartbeatMsg(t, "rv/discovery/slow.example.com/uuid6",
			rodrevInfo("slow.example.com", common.RodrevService{
				FQDN:              "slow.example.com",
				HeartbeatInterval: time.Hour,
			}))
		node, _, ok := parseHeartbeat(r, msg)
		require.True(t, ok)
		assert.Equal(t, time.Hour*staleAfter, node.StaleAge)
	})
}

// DiscoverOnce is what a short lived command uses: subscribe, collect what is
// retained, stop
func TestDiscoverOnce(t *testing.T) {
	r, tr, _ := testSessionRuntime(t)
	now := time.Now()
	go func() {
		// retained heartbeats arrive right after the subscription is made
		time.Sleep(time.Millisecond * 50)
		_ = tr.deliver(heartbeat(t, "a.example.com", now, "status", "query"))
		_ = tr.deliver(heartbeat(t, "b.example.com", now))
		_ = tr.deliver(heartbeat(t, "gone.example.com", now.Add(-time.Hour*48)))
	}()
	d, err := DiscoverOnce(r, DiscoverOpts{
		InitialWait: testDiscoverInitialWait,
		IdleWait:    testDiscoverIdleWait,
	})
	require.NoError(t, err)
	assert.Equal(t, []string{"a.example.com", "b.example.com"}, d.ActiveNodes())
	assert.Contains(t, d.Stale, "gone.example.com")
	assert.Equal(t, []string{"a.example.com"}, d.WithFeature("query"))
	assert.Len(t, d.Services[common.RodrevServiceName], 3)
}

// with nothing to hear it gives up after the initial wait rather than hanging
func TestDiscoverOnceNothingThere(t *testing.T) {
	r, _, _ := testSessionRuntime(t)
	start := time.Now()
	d, err := DiscoverOnce(r, DiscoverOpts{
		InitialWait: time.Millisecond * 100,
		IdleWait:    time.Millisecond * 50,
	})
	require.NoError(t, err)
	assert.Empty(t, d.Active)
	assert.Less(t, time.Since(start), time.Second)
}

func TestDiscoverOnceDefaults(t *testing.T) {
	r, tr, _ := testSessionRuntime(t)
	go func() {
		time.Sleep(time.Millisecond * 20)
		_ = tr.deliver(heartbeat(t, "a.example.com", time.Now()))
	}()
	// zero options mean the conservative defaults, and the idle wait ends the
	// run long before the initial one would
	start := time.Now()
	_, active, _, err := Discover(r)
	require.NoError(t, err)
	assert.Contains(t, active, "a.example.com")
	assert.Less(t, time.Since(start), DefaultDiscoverOpts.InitialWait)
}

// a subscription failure has to be reported, not silently treated as an empty fleet
func TestDiscoverOnceNoTransport(t *testing.T) {
	r, _, _ := testSessionRuntime(t)
	r.Transport = nil
	_, err := DiscoverOnce(r, DefaultDiscoverOpts)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "subscribe")
}

func TestStaleAge(t *testing.T) {
	// a node that checks in often still gets a generous grace period
	assert.Equal(t, minStaleAge, staleAge(time.Minute))
	assert.Equal(t, minStaleAge, staleAge(0))
	// a node with a slow heartbeat gets proportionally more
	assert.Equal(t, time.Hour*staleAfter, staleAge(time.Hour))
}
