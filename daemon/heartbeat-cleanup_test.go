package daemon

import (
	"encoding/json"
	"math/rand"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zerosvc/go-zerosvc"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

func presencePayload(t *testing.T, ts time.Time) []byte {
	t.Helper()
	raw, err := json.Marshal(zerosvc.NodeInfo{
		Name:     "d1-lg.example.com",
		UUID:     "uuid1",
		TS:       ts,
		Services: map[string]zerosvc.Service{"puppet": {Ok: true}},
	})
	require.NoError(t, err)
	return raw
}

func TestStaleHeartbeat(t *testing.T) {
	now := time.Now()
	maxAge := time.Hour * 24 * 30
	tests := []struct {
		name    string
		topic   string
		payload []byte
		stale   bool
		reason  string
	}{
		{
			name:    "checking in",
			topic:   "rv/discovery/d1-lg.example.com/uuid1",
			payload: presencePayload(t, now.Add(-time.Minute)),
		},
		{
			name:    "quiet but within the grace",
			topic:   "rv/discovery/d1-lg.example.com/uuid1",
			payload: presencePayload(t, now.Add(-time.Hour*24*29)),
		},
		{
			name:    "long gone",
			topic:   "rv/discovery/d1-lg.example.com/uuid1",
			payload: presencePayload(t, now.Add(-time.Hour*24*400)),
			stale:   true, reason: "last seen",
		},
		{
			name:    "no timestamp",
			topic:   "rv/discovery/d1-lg.example.com/uuid1",
			payload: []byte(`{"name":"d1-lg.example.com"}`),
			stale:   true, reason: "no timestamp",
		},
		{
			name:    "unparseable",
			topic:   "rv/discovery/d1-lg.example.com/uuid1",
			payload: []byte(`{"name":`),
			stale:   true, reason: "can not be parsed",
		},
		// nothing publishes to the old topic any more, so whatever is retained
		// there is left over no matter how it looks
		{
			name:    "old heartbeat topic",
			topic:   "rv/heartbeat/d1-lg.example.com",
			payload: presencePayload(t, now),
			stale:   true, reason: "older daemons used",
		},
		{
			name:    "already removed",
			topic:   "rv/discovery/d1-lg.example.com/uuid1",
			payload: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reason, stale := staleHeartbeat(tt.topic, tt.payload, now, maxAge)
			assert.Equal(t, tt.stale, stale)
			if len(tt.reason) > 0 {
				assert.Contains(t, reason, tt.reason)
			} else {
				assert.Empty(t, reason)
			}
		})
	}
}

// fakePublisher records what would be removed
type fakePublisher struct {
	cleared []string
	err     error
}

func (f *fakePublisher) SendCleanup(path string) error {
	if f.err != nil {
		return f.err
	}
	f.cleared = append(f.cleared, path)
	return nil
}

func testCleanupCache(t *testing.T) *heartbeatCache {
	t.Helper()
	now := time.Now()
	cache := newHeartbeatCache()
	cache.entries = map[string][]byte{
		"rv/discovery/alive.example.com/uuid1":  presencePayload(t, now),
		"rv/discovery/dead.example.com/uuid2":   presencePayload(t, now.Add(-time.Hour*24*365)),
		"rv/discovery/broken.example.com/uuid3": []byte(`{"name":`),
		"rv/heartbeat/ancient.example.com":      presencePayload(t, now),
		"rv/discovery/self.example.com/uuid4":   presencePayload(t, now),
	}
	return cache
}

func TestRunHeartbeatCleanup(t *testing.T) {
	core, logs := observer.New(zap.DebugLevel)
	l := zap.New(core).Sugar()
	cache := testCleanupCache(t)
	pub := &fakePublisher{}
	opts := cleanupOpts{
		MaxAge:    time.Hour * 24 * 30,
		EventRoot: "rv",
		Keep:      "rv/discovery/self.example.com/uuid4",
	}
	cleared, seen := runHeartbeatCleanup(cache, pub, opts, l)
	assert.Equal(t, 5, seen)
	assert.Equal(t, 3, cleared)
	// the paths are relative, the node puts the event root back on
	assert.ElementsMatch(t, []string{
		"discovery/dead.example.com/uuid2",
		"discovery/broken.example.com/uuid3",
		"heartbeat/ancient.example.com",
	}, pub.cleared)
	// a node that is checking in, and our own presence, are left alone
	remaining := cache.snapshot()
	assert.Contains(t, remaining, "rv/discovery/alive.example.com/uuid1")
	assert.Contains(t, remaining, "rv/discovery/self.example.com/uuid4")
	assert.NotContains(t, remaining, "rv/discovery/dead.example.com/uuid2")
	assert.NotEmpty(t, logs.FilterMessageSnippet("removed retained presence").All())

	// running again has nothing left to do
	cleared, _ = runHeartbeatCleanup(cache, pub, opts, l)
	assert.Equal(t, 0, cleared)
}

func TestRunHeartbeatCleanupDryRun(t *testing.T) {
	core, logs := observer.New(zap.DebugLevel)
	cache := testCleanupCache(t)
	pub := &fakePublisher{}
	cleared, _ := runHeartbeatCleanup(cache, pub, cleanupOpts{
		MaxAge:    time.Hour * 24 * 30,
		EventRoot: "rv",
		DryRun:    true,
	}, zap.New(core).Sugar())
	// the same three as above: fresh presence is never touched, Keep or not
	assert.Equal(t, 3, cleared)
	assert.Empty(t, pub.cleared, "a dry run removes nothing")
	assert.Len(t, cache.snapshot(), 5, "and forgets nothing")
	assert.NotEmpty(t, logs.FilterMessageSnippet("would remove").All())
}

func TestRunHeartbeatCleanupPublishError(t *testing.T) {
	core, logs := observer.New(zap.DebugLevel)
	cache := testCleanupCache(t)
	pub := &fakePublisher{err: assert.AnError}
	cleared, _ := runHeartbeatCleanup(cache, pub, cleanupOpts{
		MaxAge: time.Hour * 24 * 30, EventRoot: "rv",
	}, zap.New(core).Sugar())
	assert.Equal(t, 0, cleared)
	// a failed removal keeps the entry, so the next pass tries again
	assert.Len(t, cache.snapshot(), 5)
	assert.NotEmpty(t, logs.FilterMessageSnippet("could not remove").All())
}

// the cache follows the broker: a removed retained message drops out of it
func TestHeartbeatCacheFollow(t *testing.T) {
	cache := newHeartbeatCache()
	ch := make(chan *zerosvc.Message, 4)
	done := make(chan bool)
	go func() {
		cache.follow(ch)
		done <- true
	}()
	payload := presencePayload(t, time.Now())
	ch <- &zerosvc.Message{Topic: "rv/discovery/a/uuid", Payload: payload}
	ch <- &zerosvc.Message{Topic: "rv/discovery/b/uuid", Payload: payload}
	assert.Eventually(t, func() bool { return len(cache.snapshot()) == 2 }, time.Second, time.Millisecond*10)
	// an empty payload is a removal
	ch <- &zerosvc.Message{Topic: "rv/discovery/a/uuid"}
	assert.Eventually(t, func() bool {
		_, present := cache.snapshot()["rv/discovery/a/uuid"]
		return !present
	}, time.Second, time.Millisecond*10)
	close(ch)
	<-done
}

func TestRelativeTopic(t *testing.T) {
	assert.Equal(t, "discovery/a/uuid", relativeTopic("rv/discovery/a/uuid", "rv"))
	assert.Equal(t, "heartbeat/a", relativeTopic("rv/heartbeat/a", "rv"))
	assert.Equal(t, "rv/discovery/a", relativeTopic("rv/discovery/a", ""))
	// a prefix with more than one level works the same way
	assert.Equal(t, "discovery/a", relativeTopic("site/rv/discovery/a", "site/rv"))
}

// the schedule has to be spread out, or a fleet does this in a spike
func TestJittered(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	week := time.Hour * 24 * 7
	seen := make(map[time.Duration]bool, 100)
	for i := 0; i < 100; i++ {
		got := jittered(rng, week, intervalSpread)
		assert.Greater(t, got, time.Duration(float64(week)*(1-intervalSpread))-time.Second)
		assert.Less(t, got, time.Duration(float64(week)*(1+intervalSpread))+time.Second)
		seen[got] = true
	}
	assert.Greater(t, len(seen), 90, "the delay has to actually vary")
	assert.Equal(t, week, jittered(rng, week, 0), "no spread means no change")
	assert.Equal(t, time.Duration(0), jittered(rng, 0, intervalSpread))
}
