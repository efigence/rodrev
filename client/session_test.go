package client

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/efigence/rodrev/common"
	"github.com/efigence/rodrev/config"
	"github.com/efigence/rodrev/plugin/puppet"
	"github.com/fxamacker/cbor/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zerosvc/go-zerosvc"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

// loopback is an in-process zerosvc transport: events published to it are
// delivered to whatever subscribed to a matching topic, so RPC plumbing can be
// tested without a broker
type loopback struct {
	l         sync.Mutex
	subs      map[string]chan *zerosvc.Message
	published []sentEvent
	// node is needed to serialize and deserialize events, set once it exists
	node *zerosvc.Node
	// onSend plays the daemon side: it is called for every request the client
	// sends, in its own goroutine
	onSend  func(t *loopback, path string, ev zerosvc.Event)
	sendErr error
}

type sentEvent struct {
	Path string
	Ev   zerosvc.Event
}

func newLoopback() *loopback {
	return &loopback{subs: make(map[string]chan *zerosvc.Message, 4)}
}

func (t *loopback) Connect(h zerosvc.Hooks, willPath string) error { return nil }

func (t *loopback) Subscribe(topic string, data chan *zerosvc.Message) error {
	t.l.Lock()
	defer t.l.Unlock()
	t.subs[topic] = data
	return nil
}

func (t *loopback) HeartbeatMessage(m zerosvc.Message) error { return nil }

func (t *loopback) Publish(m zerosvc.Message) error {
	if t.sendErr != nil {
		return t.sendErr
	}
	t.l.Lock()
	node := t.node
	onSend := t.onSend
	t.l.Unlock()
	// a reply is just another publish, only requests get handed to the daemon side
	isReply := strings.Contains(m.Topic, "/reply/")
	var ev zerosvc.Event
	if node != nil && len(m.Payload) > 0 {
		if decoded, err := (&zerosvc.Event{}).Deserialize(m.Payload, node); err == nil {
			ev = *decoded
			ev.RoutingKey = m.Topic
		}
	}
	if isReply {
		return t.deliver(m)
	}
	t.l.Lock()
	t.published = append(t.published, sentEvent{Path: m.Topic, Ev: ev})
	t.l.Unlock()
	if onSend != nil {
		go onSend(t, m.Topic, ev)
	}
	return nil
}

// deliver routes a message to subscribers, handling the trailing # the way MQTT
// does (it matches the parent level too)
func (t *loopback) deliver(m zerosvc.Message) error {
	t.l.Lock()
	defer t.l.Unlock()
	for filter, ch := range t.subs {
		prefix := strings.TrimSuffix(strings.TrimSuffix(filter, "#"), "/")
		if !strings.HasSuffix(filter, "#") {
			if filter != m.Topic {
				continue
			}
		} else if m.Topic != prefix && !strings.HasPrefix(m.Topic, prefix+"/") {
			continue
		}
		msg := m
		select {
		case ch <- &msg:
		case <-time.After(time.Second):
			return nil
		}
	}
	return nil
}

// reply plays a node answering a request, the way plugin/puppet does
func (t *loopback) reply(req zerosvc.Event, fqdn string, replyType string, body interface{}) {
	t.l.Lock()
	node := t.node
	t.l.Unlock()
	reply := node.PrepareReply(req)
	reply.NodeName = fqdn
	reply.Headers["fqdn"] = fqdn
	reply.Headers["reply-type"] = replyType
	if id, ok := req.Headers["correlation-id"]; ok {
		reply.Headers["correlation-id"] = id
	}
	if err := reply.Marshal(body); err != nil {
		return
	}
	payload, err := reply.Serialize()
	if err != nil {
		return
	}
	_ = t.deliver(zerosvc.Message{Topic: node2root + "/" + req.ReplyTo, Payload: payload})
}

func (t *loopback) sent() []sentEvent {
	t.l.Lock()
	defer t.l.Unlock()
	out := make([]sentEvent, len(t.published))
	copy(out, t.published)
	return out
}

// node2root is the event root the test nodes use
const node2root = "rv"

func testSessionRuntime(t *testing.T) (*common.Runtime, *loopback, *observer.ObservedLogs) {
	t.Helper()
	tr := newLoopback()
	core, logs := observer.New(zap.DebugLevel)
	log := zap.New(core).Sugar()
	node, err := zerosvc.NewNode(zerosvc.Config{
		NodeName:          "rf-client-test",
		NodeUUID:          "test-uuid",
		Transport:         tr,
		EventRoot:         node2root,
		HeartbeatInterval: time.Hour,
		Logger:            log,
	})
	require.NoError(t, err)
	tr.l.Lock()
	tr.node = node
	tr.l.Unlock()
	r := &common.Runtime{
		Node:      node,
		Transport: tr,
		MQPrefix:  node2root + "/",
		Log:       log,
		Cfg:       config.Config{MQAddress: "tcp://test:1883"},
	}
	return r, tr, logs
}

func TestSessionCall(t *testing.T) {
	r, tr, _ := testSessionRuntime(t)
	tr.onSend = func(tr *loopback, path string, ev zerosvc.Event) {
		tr.reply(ev, "a.example.com", common.PuppetRunStatus, puppet.LastRunSummary{})
		tr.reply(ev, "b.example.com", common.PuppetRunStatus, puppet.LastRunSummary{})
	}
	s, err := NewSession(r)
	require.NoError(t, err)
	defer s.Close()

	ctx, cancel := context.WithTimeout(context.Background(), testQueryTimeout)
	defer cancel()
	replies, err := s.CallCollect(ctx, Request{Topic: "puppet", Command: puppet.Status})
	require.NoError(t, err)
	require.Len(t, replies, 2)
	assert.Equal(t, "a.example.com", replies[0].FQDN)
	assert.Equal(t, common.PuppetRunStatus, replies[0].ReplyType)
	assert.NoError(t, replies[0].NodeErr)
	assert.NotZero(t, replies[0].RTT)

	// the request goes out as the usual command envelope, with a reply topic
	// under the session's own path
	require.Len(t, tr.sent(), 1)
	assert.Equal(t, "rv/puppet", tr.sent()[0].Path)
	var cmd puppet.PuppetCmdRecv
	sentEv := tr.sent()[0].Ev
	require.NoError(t, sentEv.Unmarshal(&cmd))
	assert.Equal(t, puppet.Status, cmd.Command)
	assert.True(t, strings.HasPrefix(sentEv.ReplyTo, s.ReplyPath()+"/"))
	assert.NotEmpty(t, sentEv.Headers["correlation-id"])
}

// two calls share one subscription, and neither sees the other's replies
func TestSessionCallIsolation(t *testing.T) {
	r, tr, _ := testSessionRuntime(t)
	tr.onSend = func(tr *loopback, path string, ev zerosvc.Event) {
		var cmd puppet.PuppetCmdRecv
		_ = ev.Unmarshal(&cmd)
		// answer with the command name so each call can be told apart
		tr.reply(ev, cmd.Command+".example.com", common.PuppetRunStatus, puppet.LastRunSummary{})
	}
	s, err := NewSession(r)
	require.NoError(t, err)
	defer s.Close()
	ctx, cancel := context.WithTimeout(context.Background(), testQueryTimeout)
	defer cancel()

	var wg sync.WaitGroup
	results := make([][]Reply, 2)
	for i, command := range []string{puppet.Status, puppet.Fact} {
		wg.Add(1)
		go func(i int, command string) {
			defer wg.Done()
			replies, err := s.CallCollect(ctx, Request{Topic: "puppet", Command: command})
			assert.NoError(t, err)
			results[i] = replies
		}(i, command)
	}
	wg.Wait()
	require.Len(t, results[0], 1)
	require.Len(t, results[1], 1)
	assert.Equal(t, puppet.Status+".example.com", results[0][0].FQDN)
	assert.Equal(t, puppet.Fact+".example.com", results[1][0].FQDN)
}

func TestSessionCallStopsEarly(t *testing.T) {
	r, tr, _ := testSessionRuntime(t)
	tr.onSend = func(tr *loopback, path string, ev zerosvc.Event) {
		for _, fqdn := range []string{"a", "b", "c"} {
			tr.reply(ev, fqdn, common.PuppetRunStatus, puppet.LastRunSummary{})
		}
	}
	s, err := NewSession(r)
	require.NoError(t, err)
	defer s.Close()
	count := 0
	// no deadline: the call has to end because onReply said so
	err = s.Call(context.Background(), Request{Topic: "puppet", Command: puppet.Status},
		func(reply Reply) bool {
			count++
			return false
		})
	require.NoError(t, err)
	assert.Equal(t, 1, count)
}

func TestSessionCallDeadline(t *testing.T) {
	r, _, _ := testSessionRuntime(t)
	s, err := NewSession(r)
	require.NoError(t, err)
	defer s.Close()
	ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond*50)
	defer cancel()
	start := time.Now()
	// nobody answers, the deadline is what ends the call
	replies, err := s.CallCollect(ctx, Request{Topic: "puppet", Command: puppet.Status})
	require.NoError(t, err)
	assert.Empty(t, replies)
	assert.Less(t, time.Since(start), time.Second)
}

func TestSessionNodeError(t *testing.T) {
	r, tr, _ := testSessionRuntime(t)
	tr.onSend = func(tr *loopback, path string, ev zerosvc.Event) {
		tr.reply(ev, "a.example.com", common.Error, puppet.Msg{Msg: "unknown command facts"})
	}
	s, err := NewSession(r)
	require.NoError(t, err)
	defer s.Close()
	ctx, cancel := context.WithTimeout(context.Background(), testQueryTimeout)
	defer cancel()
	replies, err := s.CallCollect(ctx, Request{Topic: "puppet", Command: "facts"})
	require.NoError(t, err)
	require.Len(t, replies, 1)
	require.Error(t, replies[0].NodeErr)
	assert.Equal(t, "unknown command facts", replies[0].NodeErr.Error())
}

// a reply for a call that already finished is dropped, and the router carries on
func TestSessionStrayReply(t *testing.T) {
	r, tr, logs := testSessionRuntime(t)
	s, err := NewSession(r)
	require.NoError(t, err)
	defer s.Close()
	tr.reply(zerosvc.Event{
		ReplyTo: s.ReplyPath() + "/gone",
		Headers: map[string]interface{}{"correlation-id": "gone"},
	}, "a.example.com", common.PuppetRunStatus, puppet.LastRunSummary{})

	tr.onSend = func(tr *loopback, path string, ev zerosvc.Event) {
		tr.reply(ev, "b.example.com", common.PuppetRunStatus, puppet.LastRunSummary{})
	}
	ctx, cancel := context.WithTimeout(context.Background(), testQueryTimeout)
	defer cancel()
	replies, err := s.CallCollect(ctx, Request{Topic: "puppet", Command: puppet.Status})
	require.NoError(t, err)
	require.Len(t, replies, 1)
	assert.Equal(t, "b.example.com", replies[0].FQDN)
	assert.NotEmpty(t, logs.FilterMessageSnippet("dropping reply for unknown call").All())
}

func TestSessionClosed(t *testing.T) {
	r, _, _ := testSessionRuntime(t)
	s, err := NewSession(r)
	require.NoError(t, err)
	require.NoError(t, s.Close())
	// closing twice is not an error, using it afterwards is
	require.NoError(t, s.Close())
	err = s.Call(context.Background(), Request{Topic: "puppet", Command: puppet.Status}, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "closed")
}

func TestSessionSendError(t *testing.T) {
	r, tr, _ := testSessionRuntime(t)
	tr.sendErr = assert.AnError
	s, err := NewSession(r)
	require.NoError(t, err)
	defer s.Close()
	err = s.Call(context.Background(), Request{Topic: "puppet", Command: puppet.Status}, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "error sending status command")
}

func TestFilterMatch(t *testing.T) {
	r, tr, _ := testSessionRuntime(t)
	tr.onSend = func(tr *loopback, path string, ev zerosvc.Event) {
		tr.reply(ev, "a.example.com", common.PuppetRunStatus, puppet.LastRunSummary{})
		// a duplicate reply must not be counted twice
		tr.reply(ev, "a.example.com", common.PuppetRunStatus, puppet.LastRunSummary{})
		tr.reply(ev, "b.example.com", common.PuppetRunStatus, puppet.LastRunSummary{})
		tr.reply(ev, "c.example.com", common.Error, puppet.Msg{Msg: "query broke"})
	}
	s, err := NewSession(r)
	require.NoError(t, err)
	defer s.Close()
	ctx, cancel := context.WithTimeout(context.Background(), testQueryTimeout)
	defer cancel()
	streamed := make([]string, 0, 2)
	out, err := s.FilterMatch(ctx, `(== (class "nginx") true)`, func(fqdn string) {
		streamed = append(streamed, fqdn)
	})
	require.NoError(t, err)
	assert.Equal(t, []string{"a.example.com", "b.example.com"}, out.Matched)
	assert.Equal(t, out.Matched, streamed, "results are streamed as they arrive")
	assert.Len(t, out.Errors, 1)
	assert.Equal(t, 3, out.Answered())

	// the expression travels as the filter of a status request
	var cmd puppet.PuppetCmdRecv
	sentEv := tr.sent()[0].Ev
	require.NoError(t, sentEv.Unmarshal(&cmd))
	assert.Equal(t, puppet.Status, cmd.Command)
	assert.Equal(t, `(== (class "nginx") true)`, cmd.Filter)
}

func TestPuppetStatusAndFact(t *testing.T) {
	r, tr, _ := testSessionRuntime(t)
	tr.onSend = func(tr *loopback, path string, ev zerosvc.Event) {
		var cmd puppet.PuppetCmdRecv
		_ = ev.Unmarshal(&cmd)
		switch cmd.Command {
		case puppet.Status:
			summary := puppet.LastRunSummary{}
			summary.Resources.Total = 1041
			tr.reply(ev, "a.example.com", common.PuppetRunStatus, summary)
		case puppet.Fact:
			tr.reply(ev, "a.example.com", common.PuppetFact, map[string]interface{}{"virtual": "kvm"})
		}
	}
	s, err := NewSession(r)
	require.NoError(t, err)
	defer s.Close()
	// a broadcast never knows how many nodes will answer, so every call runs
	// until its own deadline - they can not share one
	statusCtx, cancelStatus := context.WithTimeout(context.Background(), testQueryTimeout)
	defer cancelStatus()
	status, coverage, err := PuppetStatusStream(statusCtx, s, "", nil)
	require.NoError(t, err)
	require.Contains(t, status, "a.example.com")
	assert.Equal(t, 1041, status["a.example.com"].Resources.Total)
	assert.Equal(t, 1, coverage.Matched)
	assert.Equal(t, 1, coverage.Answered())

	factCtx, cancelFact := context.WithTimeout(context.Background(), testQueryTimeout)
	defer cancelFact()
	facts, factCoverage, err := PuppetFactStream(factCtx, s, "virtual", "", nil)
	require.NoError(t, err)
	assert.Equal(t, "kvm", facts["a.example.com"])
	assert.Equal(t, 1, factCoverage.Matched)
}

func TestSessionQuery(t *testing.T) {
	r, tr, _ := testSessionRuntime(t)
	tr.onSend = func(tr *loopback, path string, ev zerosvc.Event) {
		tr.reply(ev, "a.example.com", common.PuppetQuery,
			puppet.QueryReply{FQDN: "a.example.com", Matched: true, Type: "bool"})
		tr.reply(ev, "b.example.com", common.PuppetQuery,
			puppet.QueryReply{FQDN: "b.example.com", Matched: false, Type: "bool"})
		tr.reply(ev, "c.example.com", common.PuppetQuery,
			puppet.QueryReply{FQDN: "c.example.com", Error: "symbol `clas` not found"})
		tr.reply(ev, "d.example.com", common.PuppetQuery,
			puppet.QueryReply{FQDN: "d.example.com", Error: "query returned hash, not a boolean",
				Type: "hash", Value: `(hash a:1)`})
		// an old daemon does not know the command at all
		tr.reply(ev, "e.example.com", common.Error, puppet.Msg{Msg: "unknown command query"})
	}
	s, err := NewSession(r)
	require.NoError(t, err)
	defer s.Close()
	ctx, cancel := context.WithTimeout(context.Background(), testQueryTimeout)
	defer cancel()
	streamed := 0
	out, err := s.Query(ctx, `(== (class "nginx") true)`, func(qr puppet.QueryReply) { streamed++ })
	require.NoError(t, err)
	assert.Equal(t, []string{"a.example.com"}, out.Matched)
	assert.Equal(t, []string{"b.example.com"}, out.NotMatched)
	assert.Len(t, out.Errors, 3)
	assert.Contains(t, out.Errors["c.example.com"], "symbol `clas`")
	assert.Contains(t, out.Errors["d.example.com"], "not a boolean")
	assert.Contains(t, out.Errors["e.example.com"], "unknown command query")
	assert.Equal(t, `(hash a:1)`, out.Values["d.example.com"])
	assert.Equal(t, 5, streamed)

	// the expression travels in the filter field, as the query command
	var cmd puppet.PuppetCmdRecv
	sentEv := tr.sent()[0].Ev
	require.NoError(t, sentEv.Unmarshal(&cmd))
	assert.Equal(t, puppet.Query, cmd.Command)
	assert.Equal(t, `(== (class "nginx") true)`, cmd.Filter)
}

// with answer_always the reply count is the number of nodes that ran the query,
// no heartbeat inventory needed
func TestFilterMatchCounting(t *testing.T) {
	r, tr, _ := testSessionRuntime(t)
	tr.onSend = func(tr *loopback, path string, ev zerosvc.Event) {
		var cmd puppet.PuppetCmdRecv
		_ = ev.Unmarshal(&cmd)
		assert.True(t, cmd.AnswerAlways, "the client asks for no-match answers")
		tr.reply(ev, "a.example.com", common.PuppetRunStatus, puppet.LastRunSummary{})
		// three nodes ran the query and said it did not match them
		for _, fqdn := range []string{"b.example.com", "c.example.com", "d.example.com"} {
			tr.reply(ev, fqdn, common.PuppetNoMatch, puppet.QueryReply{FQDN: fqdn})
		}
		// and one is running a daemon that does not know the flag, so it stays
		// quiet - it shows up nowhere, which is the honest answer
	}
	s, err := NewSession(r)
	require.NoError(t, err)
	defer s.Close()
	ctx, cancel := context.WithTimeout(context.Background(), testQueryTimeout)
	defer cancel()
	out, err := s.FilterMatch(ctx, `(== (class "nginx") true)`, nil)
	require.NoError(t, err)
	assert.Equal(t, []string{"a.example.com"}, out.Matched)
	assert.Len(t, out.NoMatch, 3)
	assert.Equal(t, 4, out.Answered())
}

func TestNodeSnapshot(t *testing.T) {
	r, tr, _ := testSessionRuntime(t)
	tr.onSend = func(tr *loopback, path string, ev zerosvc.Event) {
		var cmd puppet.PuppetCmdRecv
		_ = ev.Unmarshal(&cmd)
		switch cmd.Command {
		case puppet.FactList:
			tr.reply(ev, "a.example.com", common.PuppetFacts, puppet.FactsReply{
				FQDN:  "a.example.com",
				TS:    time.Now(),
				Count: 1,
				Facts: map[string]interface{}{"virtual": "kvm"},
			})
		case puppet.ClassList:
			tr.reply(ev, "a.example.com", common.PuppetClasses, puppet.ClassesReply{
				FQDN:    "a.example.com",
				Classes: []string{"nginx", "systemd::common"},
			})
		}
	}
	s, err := NewSession(r)
	require.NoError(t, err)
	defer s.Close()
	ctx, cancel := context.WithTimeout(context.Background(), testQueryTimeout)
	defer cancel()
	snap, err := s.NodeSnapshot(ctx, "a.example.com")
	require.NoError(t, err)
	assert.Equal(t, "a.example.com", snap.FQDN)
	assert.Equal(t, "node:a.example.com", snap.Source)
	assert.Equal(t, "kvm", snap.Facts["virtual"])
	assert.Equal(t, []string{"nginx", "systemd::common"}, snap.Classes)

	// dumps are addressed at the node, not broadcast
	for _, sent := range tr.sent() {
		assert.Equal(t, "rv/puppet/a.example.com", sent.Path)
	}

	// and the snapshot is usable as a local data set right away
	h, err := snap.Harness(nil)
	require.NoError(t, err)
	matched, err := h.Query(`(and (== (fact "virtual") "kvm") (== (class "nginx") true))`)
	require.NoError(t, err)
	assert.True(t, matched)
}

func TestNodeFactsSubsetAndOldDaemon(t *testing.T) {
	r, tr, _ := testSessionRuntime(t)
	tr.onSend = func(tr *loopback, path string, ev zerosvc.Event) {
		var cmd puppet.PuppetCmdRecv
		_ = ev.Unmarshal(&cmd)
		var opts puppet.FactListOptions
		_ = cbor.Unmarshal(cmd.Parameters, &opts)
		if len(opts.Keys) > 0 {
			tr.reply(ev, "a.example.com", common.PuppetFacts, puppet.FactsReply{
				FQDN:  "a.example.com",
				Facts: map[string]interface{}{opts.Keys[0]: "kvm"},
			})
			return
		}
		// no fact support on this daemon
		tr.reply(ev, "a.example.com", common.Error, puppet.Msg{Msg: "unknown command facts"})
	}
	s, err := NewSession(r)
	require.NoError(t, err)
	defer s.Close()
	ctx, cancel := context.WithTimeout(context.Background(), testQueryTimeout)
	defer cancel()

	facts, err := s.NodeFacts(ctx, "a.example.com", "virtual")
	require.NoError(t, err)
	assert.Equal(t, "kvm", facts.Facts["virtual"])

	_, err = s.NodeFacts(ctx, "a.example.com")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "without fact dump support")
}

func TestDiscoveryFeatures(t *testing.T) {
	d := Discovery{Active: map[string]common.Node{
		"new.example.com": {FQDN: "new.example.com", Features: []string{"status", "query", "facts"}},
		"old.example.com": {FQDN: "old.example.com"},
	}}
	assert.Equal(t, []string{"new.example.com", "old.example.com"}, d.ActiveNodes())
	assert.Equal(t, []string{"new.example.com"}, d.WithFeature(puppet.Query))
	assert.Empty(t, d.WithFeature("nosuchfeature"))
}

// the features the daemon announces must be the commands it actually handles
func TestAnnouncedFeatures(t *testing.T) {
	assert.Contains(t, puppet.Features, puppet.Query)
	assert.Contains(t, puppet.Features, puppet.FactList)
	assert.Contains(t, puppet.Features, puppet.ClassList)
	assert.Contains(t, puppet.Features, puppet.Status)
}

// PuppetFilterMatch is the sessionless form, for callers that run one query
func TestPuppetFilterMatch(t *testing.T) {
	r, tr, _ := testSessionRuntime(t)
	tr.onSend = func(tr *loopback, path string, ev zerosvc.Event) {
		tr.reply(ev, "a.example.com", common.PuppetRunStatus, puppet.LastRunSummary{})
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond*200)
	defer cancel()
	out, err := PuppetFilterMatch(ctx, r, `(== (class "nginx") true)`, nil)
	require.NoError(t, err)
	assert.Equal(t, []string{"a.example.com"}, out.Matched)
}

// PuppetStatus and PuppetFact are what the cli calls; they wrap a session and
// report how much of the fleet took part
func TestPuppetStatusAndFactWrappers(t *testing.T) {
	r, tr, logs := testSessionRuntime(t)
	tr.onSend = func(tr *loopback, path string, ev zerosvc.Event) {
		var cmd puppet.PuppetCmdRecv
		_ = ev.Unmarshal(&cmd)
		switch cmd.Command {
		case puppet.Status:
			summary := puppet.LastRunSummary{}
			summary.Resources.Total = 7
			tr.reply(ev, "a.example.com", common.PuppetRunStatus, summary)
			// a node the filter did not match answers too, so the count is real
			tr.reply(ev, "b.example.com", common.PuppetNoMatch,
				puppet.QueryReply{FQDN: "b.example.com"})
		}
	}
	status := PuppetStatus(r, `(== (class "nginx") true)`)
	require.Contains(t, status, "a.example.com")
	assert.Equal(t, 7, status["a.example.com"].Resources.Total)
	assert.NotContains(t, status, "b.example.com", "a no-match is counted, not returned")
	assert.NotEmpty(t, logs.FilterMessageSnippet("1 matched, 2 answered").All())

	t.Run("a filter is passed through", func(t *testing.T) {
		var cmd puppet.PuppetCmdRecv
		require.NoError(t, tr.sent()[0].Ev.Unmarshal(&cmd))
		assert.Equal(t, `(== (class "nginx") true)`, cmd.Filter)
		assert.True(t, cmd.AnswerAlways)
	})
	t.Run("more than one filter is a programming error", func(t *testing.T) {
		assert.Panics(t, func() { PuppetStatus(r, "a", "b") })
	})
}

func TestCoverage(t *testing.T) {
	c := Coverage{Matched: 3, NoMatch: 360, Errors: 3}
	assert.Equal(t, 366, c.Answered())
	assert.Equal(t, "3 matched, 366 answered", c.String())
	empty := Coverage{}
	assert.Equal(t, 0, empty.Answered())
}

func TestFilterOutcomeAnswered(t *testing.T) {
	out := FilterOutcome{
		Matched: []string{"a"},
		NoMatch: []string{"b", "c"},
		Errors:  map[string]string{"d": "boom"},
	}
	assert.Equal(t, 4, out.Answered())
	assert.Equal(t, 0, (&FilterOutcome{}).Answered())
}
