package client

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/efigence/rodrev/common"
	"github.com/efigence/rodrev/config"
	"github.com/efigence/rodrev/plugin/puppet"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zerosvc/go-zerosvc"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

// loopback is an in-process zerosvc transport: events sent to it are delivered
// to whatever subscribed to a matching filter, so RPC plumbing can be tested
// without a broker
type loopback struct {
	l    sync.Mutex
	subs map[string]chan zerosvc.Event
	sent []sentEvent
	// onSend plays the daemon side: it is called for every event the client
	// sends, in its own goroutine
	onSend  func(t *loopback, path string, ev zerosvc.Event)
	sendErr error
}

type sentEvent struct {
	Path string
	Ev   zerosvc.Event
}

func newLoopback() *loopback {
	return &loopback{subs: make(map[string]chan zerosvc.Event, 4)}
}

func (t *loopback) Connect() error             { return nil }
func (t *loopback) Shutdown()                  {}
func (t *loopback) AdminCleanup()              {}
func (t *loopback) SetupHeartbeat(path string) {}

func (t *loopback) GetEvents(filter string, ch chan zerosvc.Event) error {
	t.l.Lock()
	defer t.l.Unlock()
	t.subs[filter] = ch
	return nil
}

func (t *loopback) SendEvent(path string, ev zerosvc.Event) error {
	if t.sendErr != nil {
		return t.sendErr
	}
	t.l.Lock()
	t.sent = append(t.sent, sentEvent{Path: path, Ev: ev})
	onSend := t.onSend
	t.l.Unlock()
	if onSend != nil {
		go onSend(t, path, ev)
	}
	return nil
}

func (t *loopback) SendReply(path string, ev zerosvc.Event) error {
	return t.deliver(path, ev)
}

// deliver routes an event to subscribers, handling the trailing # the way MQTT
// does (it matches the parent level too)
func (t *loopback) deliver(topic string, ev zerosvc.Event) error {
	ev.RoutingKey = topic
	t.l.Lock()
	defer t.l.Unlock()
	for filter, ch := range t.subs {
		prefix := strings.TrimSuffix(strings.TrimSuffix(filter, "#"), "/")
		if !strings.HasSuffix(filter, "#") {
			if filter != topic {
				continue
			}
		} else if topic != prefix && !strings.HasPrefix(topic, prefix+"/") {
			continue
		}
		select {
		case ch <- ev:
		case <-time.After(time.Second):
			return nil
		}
	}
	return nil
}

// reply plays a node answering a request, the way plugin/puppet does
func (t *loopback) reply(req zerosvc.Event, fqdn string, replyType string, body interface{}) {
	ev := zerosvc.NewEvent()
	raw, _ := json.Marshal(body)
	ev.Body = raw
	ev.Headers["fqdn"] = fqdn
	ev.Headers["node-name"] = fqdn
	ev.Headers["reply-type"] = replyType
	if id, ok := req.Headers["correlation-id"]; ok {
		ev.Headers["correlation-id"] = id
	}
	_ = t.deliver(req.ReplyTo, ev)
}

func testSessionRuntime(t *testing.T) (*common.Runtime, *loopback, *observer.ObservedLogs) {
	t.Helper()
	tr := newLoopback()
	node := zerosvc.NewNode("rf-client-test", "test-uuid")
	node.SetTransport(tr)
	core, logs := observer.New(zap.DebugLevel)
	r := &common.Runtime{
		Node:     node,
		MQPrefix: "rv/",
		Log:      zap.New(core).Sugar(),
		Cfg:      config.Config{MQAddress: "tcp://test:1883"},
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

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
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
	require.Len(t, tr.sent, 1)
	assert.Equal(t, "rv/puppet", tr.sent[0].Path)
	var cmd puppet.PuppetCmdRecv
	require.NoError(t, tr.sent[0].Ev.Unmarshal(&cmd))
	assert.Equal(t, puppet.Status, cmd.Command)
	assert.True(t, strings.HasPrefix(tr.sent[0].Ev.ReplyTo, s.ReplyPath()+"/"))
	assert.NotEmpty(t, tr.sent[0].Ev.Headers["correlation-id"])
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
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
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
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
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
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
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
	ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond*300)
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
	require.NoError(t, tr.sent[0].Ev.Unmarshal(&cmd))
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
	statusCtx, cancelStatus := context.WithTimeout(context.Background(), time.Millisecond*300)
	defer cancelStatus()
	status, coverage, err := PuppetStatusStream(statusCtx, s, "", nil)
	require.NoError(t, err)
	require.Contains(t, status, "a.example.com")
	assert.Equal(t, 1041, status["a.example.com"].Resources.Total)
	assert.Equal(t, 1, coverage.Matched)
	assert.Equal(t, 1, coverage.Answered())

	factCtx, cancelFact := context.WithTimeout(context.Background(), time.Millisecond*300)
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
	ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond*300)
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
	require.NoError(t, tr.sent[0].Ev.Unmarshal(&cmd))
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
	ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond*300)
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
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	snap, err := s.NodeSnapshot(ctx, "a.example.com")
	require.NoError(t, err)
	assert.Equal(t, "a.example.com", snap.FQDN)
	assert.Equal(t, "node:a.example.com", snap.Source)
	assert.Equal(t, "kvm", snap.Facts["virtual"])
	assert.Equal(t, []string{"nginx", "systemd::common"}, snap.Classes)

	// dumps are addressed at the node, not broadcast
	tr.l.Lock()
	defer tr.l.Unlock()
	for _, sent := range tr.sent {
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
		_ = json.Unmarshal(cmd.Parameters, &opts)
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
	ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond*300)
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
