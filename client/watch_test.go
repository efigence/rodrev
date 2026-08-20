package client

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/efigence/rodrev/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zerosvc/go-zerosvc"
)

// heartbeat builds what a daemon publishes about itself
func heartbeat(t *testing.T, name string, ts time.Time, features ...string) zerosvc.Message {
	t.Helper()
	payload, err := json.Marshal(zerosvc.NodeInfo{
		Name: name,
		UUID: "uuid-" + name,
		TS:   ts,
		Services: map[string]zerosvc.Service{
			common.RodrevServiceName: {Ok: true, Data: common.RodrevService{
				FQDN:              name,
				Version:           "1.2.3",
				Features:          features,
				HeartbeatInterval: time.Minute,
			}},
			"puppet": {Ok: true},
		},
	})
	require.NoError(t, err)
	return zerosvc.Message{Topic: node2root + "/discovery/" + name + "/uuid-" + name, Payload: payload}
}

func TestNodeWatcher(t *testing.T) {
	r, tr, _ := testSessionRuntime(t)
	w, err := WatchNodes(r)
	require.NoError(t, err)

	now := time.Now()
	require.NoError(t, tr.deliver(heartbeat(t, "a.example.com", now, "status", "query")))
	require.NoError(t, tr.deliver(heartbeat(t, "b.example.com", now, "status")))
	// quiet for far longer than three heartbeat intervals
	require.NoError(t, tr.deliver(heartbeat(t, "old.example.com", now.Add(-time.Hour*24))))

	assert.Eventually(t, func() bool { return len(w.Discovery().Active) == 2 },
		time.Second*2, time.Millisecond*10)
	d := w.Discovery()
	assert.Equal(t, []string{"a.example.com", "b.example.com"}, d.ActiveNodes())
	assert.Contains(t, d.Stale, "old.example.com")
	assert.NotContains(t, d.Active, "old.example.com")
	// only the node that announced the feature has it
	assert.Equal(t, []string{"a.example.com"}, d.WithFeature("query"))
	assert.Len(t, d.Services["puppet"], 3, "stale nodes still provide their services")
	assert.Equal(t, "1.2.3", d.Active["a.example.com"].DaemonVersion)

	t.Run("a newer heartbeat replaces the old one", func(t *testing.T) {
		require.NoError(t, tr.deliver(heartbeat(t, "old.example.com", time.Now())))
		assert.Eventually(t, func() bool {
			_, stale := w.Discovery().Stale["old.example.com"]
			return !stale
		}, time.Second*2, time.Millisecond*10)
	})

	// an empty payload is a node clearing its presence, which is how it says
	// goodbye. It has to disappear rather than linger as stale
	t.Run("cleared presence removes the node", func(t *testing.T) {
		require.NoError(t, tr.deliver(zerosvc.Message{
			Topic: node2root + "/discovery/b.example.com/uuid-b.example.com",
		}))
		assert.Eventually(t, func() bool {
			_, present := w.Discovery().Active["b.example.com"]
			return !present
		}, time.Second*2, time.Millisecond*10)
	})

	// something else on the discovery tree is not a fleet node
	t.Run("non rodrev heartbeats are ignored", func(t *testing.T) {
		payload, err := json.Marshal(zerosvc.NodeInfo{Name: "rf-client-laptop", TS: time.Now()})
		require.NoError(t, err)
		require.NoError(t, tr.deliver(zerosvc.Message{
			Topic:   node2root + "/discovery/rf-client-laptop/uuid",
			Payload: payload,
		}))
		time.Sleep(time.Millisecond * 100)
		assert.NotContains(t, w.Discovery().Active, "rf-client-laptop")
	})
}

func TestNodeWatcherWaitForNodes(t *testing.T) {
	r, tr, _ := testSessionRuntime(t)
	w, err := WatchNodes(r)
	require.NoError(t, err)

	t.Run("gives up when nothing shows up", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond*100)
		defer cancel()
		start := time.Now()
		w.WaitForNodes(ctx)
		assert.Less(t, time.Since(start), time.Second)
		assert.Empty(t, w.Discovery().Active)
	})
	t.Run("returns as soon as one arrives", func(t *testing.T) {
		go func() {
			time.Sleep(time.Millisecond * 50)
			_ = tr.deliver(heartbeat(t, "a.example.com", time.Now()))
		}()
		ctx, cancel := context.WithTimeout(context.Background(), time.Second*2)
		defer cancel()
		w.WaitForNodes(ctx)
		assert.NotEmpty(t, w.Discovery().Active)
	})
}
