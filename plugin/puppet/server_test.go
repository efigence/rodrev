package puppet

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/efigence/rodrev/common"
	"github.com/efigence/rodrev/config"
	"github.com/fxamacker/cbor/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zerosvc/go-zerosvc"
	"go.uber.org/zap"
)

// fakeTransport records what the daemon publishes
type fakeTransport struct {
	l         sync.Mutex
	published []zerosvc.Message
}

func (t *fakeTransport) Connect(h zerosvc.Hooks, willPath string) error { return nil }
func (t *fakeTransport) HeartbeatMessage(m zerosvc.Message) error       { return nil }
func (t *fakeTransport) Subscribe(topic string, ch chan *zerosvc.Message) error {
	return nil
}

func (t *fakeTransport) Publish(m zerosvc.Message) error {
	t.l.Lock()
	defer t.l.Unlock()
	t.published = append(t.published, m)
	return nil
}

func (t *fakeTransport) sent() []zerosvc.Message {
	t.l.Lock()
	defer t.l.Unlock()
	out := make([]zerosvc.Message, len(t.published))
	copy(out, t.published)
	return out
}

// testPuppetWithNode is a handler that can actually answer, so the plumbing
// around handleCommand can be tested too
func testPuppetWithNode(t *testing.T) (*Puppet, *fakeTransport) {
	t.Helper()
	p := testPuppet(t)
	tr := &fakeTransport{}
	node, err := zerosvc.NewNode(zerosvc.Config{
		NodeName:          p.fqdn,
		NodeUUID:          "test-uuid",
		Transport:         tr,
		EventRoot:         "rv",
		HeartbeatInterval: time.Hour,
		Logger:            zap.NewNop().Sugar(),
	})
	require.NoError(t, err)
	p.node = node
	p.runtime = &common.Runtime{
		Node:      node,
		Transport: tr,
		FQDN:      p.fqdn,
		MQPrefix:  "rv/",
		Log:       zap.NewNop().Sugar(),
		Cfg:       config.Config{},
	}
	return p, tr
}

// request builds what a client sends
func request(t *testing.T, p *Puppet, topic string, cmd PuppetCmdSend, replyTo string) *zerosvc.Event {
	t.Helper()
	ev := p.node.NewEvent()
	require.NoError(t, ev.Marshal(cmd))
	ev.RoutingKey = topic
	ev.ReplyTo = replyTo
	ev.Headers["correlation-id"] = "call1"
	return &ev
}

func TestHandleEvent(t *testing.T) {
	t.Run("answers a status request", func(t *testing.T) {
		p, tr := testPuppetWithNode(t)
		ev := request(t, p, "rv/puppet", PuppetCmdSend{Command: Status}, "reply/client/abc")
		require.NoError(t, p.HandleEvent(ev))

		sent := tr.sent()
		require.Len(t, sent, 1)
		assert.Equal(t, "rv/reply/client/abc", sent[0].Topic)
		reply, err := (&zerosvc.Event{}).Deserialize(sent[0].Payload, p.node)
		require.NoError(t, err)
		assert.Equal(t, common.PuppetRunSummary, reply.Headers["reply-type"])
		assert.Equal(t, p.fqdn, reply.Headers["fqdn"])
		// the correlation id has to survive, the client matches answers with it
		assert.Equal(t, "call1", reply.Headers["correlation-id"])
		var summary LastRunSummary
		require.NoError(t, reply.Unmarshal(&summary))
		assert.Equal(t, 1041, summary.Resources.Total)
	})
	t.Run("answers a query", func(t *testing.T) {
		p, tr := testPuppetWithNode(t)
		ev := request(t, p, "rv/puppet",
			PuppetCmdSend{Command: Query, Filter: `(== (class "nginx") true)`}, "reply/client/abc")
		require.NoError(t, p.HandleEvent(ev))
		sent := tr.sent()
		require.Len(t, sent, 1)
		reply, err := (&zerosvc.Event{}).Deserialize(sent[0].Payload, p.node)
		require.NoError(t, err)
		assert.Equal(t, common.PuppetQuery, reply.Headers["reply-type"])
		var qr QueryReply
		require.NoError(t, reply.Unmarshal(&qr))
		assert.True(t, qr.Matched)
	})
	t.Run("a filter that does not match answers nothing", func(t *testing.T) {
		p, tr := testPuppetWithNode(t)
		ev := request(t, p, "rv/puppet",
			PuppetCmdSend{Command: Status, Filter: `(== (class "apache") true)`}, "reply/client/abc")
		require.NoError(t, p.HandleEvent(ev))
		assert.Empty(t, tr.sent())
	})
	t.Run("no reply-to is refused", func(t *testing.T) {
		p, tr := testPuppetWithNode(t)
		ev := request(t, p, "rv/puppet", PuppetCmdSend{Command: Status}, "")
		err := p.HandleEvent(ev)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "no reply-to")
		assert.Empty(t, tr.sent())
	})
	t.Run("undecodable request", func(t *testing.T) {
		p, tr := testPuppetWithNode(t)
		ev := p.node.NewEvent()
		ev.Body = []byte{0xff, 0xff}
		ev.RoutingKey = "rv/puppet"
		ev.ReplyTo = "reply/client/abc"
		assert.Error(t, p.HandleEvent(&ev))
		assert.Empty(t, tr.sent())
	})
	t.Run("topic too short to route", func(t *testing.T) {
		p, tr := testPuppetWithNode(t)
		ev := request(t, p, "puppet", PuppetCmdSend{Command: Status}, "reply/client/abc")
		err := p.HandleEvent(ev)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "too short")
		assert.Empty(t, tr.sent())
	})
	// an unknown command still gets an answer, so an older client learns why
	t.Run("unknown command", func(t *testing.T) {
		p, tr := testPuppetWithNode(t)
		ev := request(t, p, "rv/puppet", PuppetCmdSend{Command: "nosuch"}, "reply/client/abc")
		require.NoError(t, p.HandleEvent(ev))
		sent := tr.sent()
		require.Len(t, sent, 1)
		reply, err := (&zerosvc.Event{}).Deserialize(sent[0].Payload, p.node)
		require.NoError(t, err)
		assert.Equal(t, common.Error, reply.Headers["reply-type"])
	})
}

// the daemon refreshes its view of puppet state in the background; the point of
// these is that a bad read keeps the last good data instead of blanking it
func TestUpdateFromFiles(t *testing.T) {
	t.Run("reads what puppet wrote", func(t *testing.T) {
		p := testPuppet(t)
		p.cfg.LastRunSummaryYAML = filepath.Join(testDataDir, HarnessLastRunSummaryFile)
		p.lastRunSummary = LastRunSummary{}
		p.updateLastRunSummary()
		assert.Equal(t, 1041, p.lastRunSummary.Resources.Total)
		p.updateFacts()
		p.updateClasses()
		assert.Equal(t, "kvm", (*p.facts.Map())["virtual"])
		assert.Contains(t, p.classes.List(), "nginx")
	})
	t.Run("a missing file keeps the last good data", func(t *testing.T) {
		p := testPuppet(t)
		p.cfg.LastRunSummaryYAML = filepath.Join(t.TempDir(), "nope.yaml")
		p.lastRunSummary.Resources.Total = 1041
		p.updateLastRunSummary()
		assert.Equal(t, 1041, p.lastRunSummary.Resources.Total, "kept rather than blanked")

		// same for facts and classes: puppet truncates them while writing
		empty := filepath.Join(t.TempDir(), "empty")
		require.NoError(t, os.WriteFile(empty, []byte(""), 0644))
		facts, err := LoadFacts(filepath.Join(testDataDir, HarnessFactsFile))
		require.NoError(t, err)
		p.facts = facts
		p.facts.path = empty
		p.updateFacts()
		assert.Equal(t, "kvm", (*p.facts.Map())["virtual"])

		classes, err := LoadClasses(filepath.Join(testDataDir, HarnessClassfile))
		require.NoError(t, err)
		p.classes = classes
		p.classes.path = empty
		p.updateClasses()
		assert.Contains(t, p.classes.List(), "nginx")
	})
	t.Run("a broken summary keeps the last good one", func(t *testing.T) {
		p := testPuppet(t)
		broken := filepath.Join(t.TempDir(), "broken.yaml")
		require.NoError(t, os.WriteFile(broken, []byte("\tnot: [yaml"), 0644))
		p.cfg.LastRunSummaryYAML = broken
		p.lastRunSummary.Resources.Total = 1041
		p.updateLastRunSummary()
		assert.Equal(t, 1041, p.lastRunSummary.Resources.Total)
	})
}

func TestPuppetErr(t *testing.T) {
	p := testPuppet(t)
	assert.Contains(t, p.puppetErr(assert.AnError).Error(), "puppet error")
	assert.Contains(t, p.puppetErr(assert.AnError, "while running:").Error(), "while running:")
}

func TestRunStatusRPCType(t *testing.T) {
	r := RunStatus{Scheduled: true}
	assert.Equal(t, common.PuppetRunStatus, r.RPCType())
	// the command envelope keeps parameters encoded until the command is known
	raw, err := cbor.Marshal(RunOptions{Noop: true})
	require.NoError(t, err)
	var opts RunOptions
	require.NoError(t, cbor.Unmarshal(raw, &opts))
	assert.True(t, opts.Noop)
}
