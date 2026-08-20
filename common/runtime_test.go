package common

import (
	"encoding/binary"
	"sync"
	"testing"
	"time"

	"github.com/efigence/rodrev/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zerosvc/go-zerosvc"
	"go.uber.org/zap"
)

// fakeTransport records what a runtime publishes and lets a test push messages
// back, without a broker
type fakeTransport struct {
	l         sync.Mutex
	published []zerosvc.Message
	subs      map[string]chan *zerosvc.Message
	subErr    error
}

func newFakeTransport() *fakeTransport {
	return &fakeTransport{subs: make(map[string]chan *zerosvc.Message, 2)}
}

func (t *fakeTransport) Connect(h zerosvc.Hooks, willPath string) error { return nil }
func (t *fakeTransport) HeartbeatMessage(m zerosvc.Message) error       { return nil }

func (t *fakeTransport) Publish(m zerosvc.Message) error {
	t.l.Lock()
	defer t.l.Unlock()
	t.published = append(t.published, m)
	return nil
}

func (t *fakeTransport) Subscribe(topic string, data chan *zerosvc.Message) error {
	if t.subErr != nil {
		return t.subErr
	}
	t.l.Lock()
	defer t.l.Unlock()
	t.subs[topic] = data
	return nil
}

func (t *fakeTransport) sent() []zerosvc.Message {
	t.l.Lock()
	defer t.l.Unlock()
	out := make([]zerosvc.Message, len(t.published))
	copy(out, t.published)
	return out
}

func testRuntime(t *testing.T) (*Runtime, *fakeTransport) {
	t.Helper()
	tr := newFakeTransport()
	node, err := zerosvc.NewNode(zerosvc.Config{
		NodeName:          "test.example.com",
		NodeUUID:          "test-uuid",
		Transport:         tr,
		EventRoot:         "rv",
		HeartbeatInterval: time.Hour,
		Logger:            zap.NewNop().Sugar(),
	})
	require.NoError(t, err)
	return &Runtime{
		Node:      node,
		Transport: tr,
		MQPrefix:  "rv/",
		FQDN:      "test.example.com",
		Log:       zap.NewNop().Sugar(),
		Cfg:       config.Config{MQAddress: "tcp://127.0.0.1:1883"},
	}, tr
}

// Reply is what the daemon answers requests with. The correlation id is how a
// client running several requests over one channel tells the answers apart, so
// losing it breaks every reply
func TestRuntimeReply(t *testing.T) {
	r, tr := testRuntime(t)
	t.Run("goes to the reply topic with the correlation id", func(t *testing.T) {
		request := zerosvc.Event{
			ReplyTo: "reply/rf-client-laptop/abc",
			Headers: map[string]any{"correlation-id": "call1"},
		}
		reply := r.Node.PrepareReply(request)
		require.NoError(t, reply.Marshal("answer"))
		require.NoError(t, r.Reply(&request, reply))

		sent := tr.sent()
		require.Len(t, sent, 1)
		assert.Equal(t, "rv/reply/rf-client-laptop/abc", sent[0].Topic)

		decoded, err := (&zerosvc.Event{}).Deserialize(sent[0].Payload, r.Node)
		require.NoError(t, err)
		assert.Equal(t, "call1", decoded.Headers["correlation-id"])
		var body string
		require.NoError(t, decoded.Unmarshal(&body))
		assert.Equal(t, "answer", body)
	})
	t.Run("a request without a correlation id still gets an answer", func(t *testing.T) {
		request := zerosvc.Event{ReplyTo: "reply/somewhere", Headers: map[string]any{}}
		reply := r.Node.PrepareReply(request)
		require.NoError(t, reply.Marshal("answer"))
		require.NoError(t, r.Reply(&request, reply))
	})
	t.Run("nowhere to reply to", func(t *testing.T) {
		request := zerosvc.Event{Headers: map[string]any{}}
		err := r.Reply(&request, r.Node.PrepareReply(request))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "no reply-to")
	})
}

func TestRuntimeSubscribeRaw(t *testing.T) {
	r, tr := testRuntime(t)
	ch, err := r.SubscribeRaw("discovery/#")
	require.NoError(t, err)
	require.NotNil(t, ch)
	// the prefix is added, the caller passes a relative topic like everywhere else
	tr.l.Lock()
	_, subscribed := tr.subs["rv/discovery/#"]
	tr.l.Unlock()
	assert.True(t, subscribed)

	t.Run("buffered, so the transport is never blocked by a slow reader", func(t *testing.T) {
		assert.Greater(t, cap(ch), 1)
	})
	t.Run("without a transport", func(t *testing.T) {
		offline := Runtime{Log: zap.NewNop().Sugar()}
		_, err := offline.SubscribeRaw("discovery/#")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "no transport")
	})
	t.Run("subscribe failure is passed up", func(t *testing.T) {
		r, tr := testRuntime(t)
		tr.subErr = assert.AnError
		_, err := r.SubscribeRaw("discovery/#")
		assert.Error(t, err)
	})
}

func TestRuntimeGetReplyChan(t *testing.T) {
	r, tr := testRuntime(t)
	path, ch, err := r.GetReplyChan()
	require.NoError(t, err)
	require.NotNil(t, ch)
	assert.NotEmpty(t, path)
	// the path goes straight into an event's ReplyTo, so it has to be relative
	assert.NotContains(t, path, "rv/")
	tr.l.Lock()
	defer tr.l.Unlock()
	// everything below the path is subscribed, so one channel serves many calls
	_, subscribed := tr.subs["rv/"+path+"/#"]
	assert.True(t, subscribed)
}

func TestRngBlob(t *testing.T) {
	r, _ := testRuntime(t)
	seen := make(map[uint64]bool, 50)
	for i := 0; i < 50; i++ {
		blob := r.RngBlob(8)
		require.Len(t, blob, 8)
		seen[binary.BigEndian.Uint64(blob)] = true
	}
	assert.Len(t, seen, 50, "reply paths and client ids are built out of this")
	assert.Len(t, r.RngBlob(1), 1)
	assert.Len(t, r.RngBlob(64), 64)
}

func TestSeededPRNG(t *testing.T) {
	r, _ := testRuntime(t)
	// two generators must not walk in step, that is the point of seeding them
	// per node: the fleet has to spread its scheduled work out
	first := r.SeededPRNG().Int63()
	second := r.SeededPRNG().Int63()
	assert.NotEqual(t, first, second)
}

func TestUnlikelyErr(t *testing.T) {
	r, _ := testRuntime(t)
	assert.NotPanics(t, func() {
		r.UnlikelyErr(nil)
		r.UnlikelyErr(assert.AnError)
		r.UnlikelyErr(assert.AnError, "while doing something")
	})
}
