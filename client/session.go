package client

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/efigence/rodrev/common"
	"github.com/efigence/rodrev/plugin/puppet"
	"github.com/zerosvc/go-zerosvc"
)

// replyBuffer is how many replies a single call can get ahead of its consumer.
// The router must never block - a blocked subscription handler stalls the whole
// MQTT client (see DiscoverOnce) - so replies over that are dropped
const replyBuffer = 512

// closeGrace is how long a session keeps consuming replies after Close, so
// stragglers do not block the transport
const closeGrace = time.Second * 30

// Request is one RPC to send. Topic is appended to the MQ prefix: "puppet"
// reaches every node, "puppet/<fqdn>" a single one
type Request struct {
	Topic   string
	Command string
	Filter  string
	Params  interface{}
	// AnswerAlways asks nodes the filter did not match to answer anyway, so the
	// replies themselves say how many nodes took part. Daemons without the
	// answer-always feature stay quiet as before
	AnswerAlways bool
}

// Reply is one node's answer
type Reply struct {
	FQDN      string
	ReplyType string
	Body      []byte
	RTT       time.Duration
	// NodeErr is set when the node answered with an error instead of a result
	NodeErr error
	// NoMatch is set when the node answered only to say the filter did not
	// match it
	NoMatch bool
	ev      zerosvc.Event
}

// Unmarshal decodes the reply body
func (r *Reply) Unmarshal(v interface{}) error {
	return r.ev.Unmarshal(v)
}

// Session multiplexes any number of RPC calls over a single reply subscription.
// zerosvc can not unsubscribe, so a subscription per call would pile up on the
// broker for as long as the client runs; with a session there is exactly one
type Session struct {
	r         *common.Runtime
	replyPath string
	ch        chan zerosvc.Event
	l         sync.Mutex
	calls     map[string]chan Reply
	closed    bool
}

// NewSession subscribes for replies. Close it when done
func NewSession(r *common.Runtime) (*Session, error) {
	replyPath, ch, err := r.GetReplyChan()
	if err != nil {
		return nil, fmt.Errorf("can't subscribe for replies: %s", err)
	}
	s := Session{
		r:         r,
		replyPath: replyPath,
		ch:        ch,
		calls:     make(map[string]chan Reply, 4),
	}
	go s.route()
	return &s, nil
}

// route hands replies to whoever is waiting for them. It never blocks and never
// runs caller code
func (s *Session) route() {
	for ev := range s.ch {
		id := callID(&ev)
		s.l.Lock()
		sink, ok := s.calls[id]
		s.l.Unlock()
		if !ok {
			// a reply to a call that already gave up, or to nothing at all
			s.r.Log.Debugf("dropping reply for unknown call [%s]: %s", id, ev.RoutingKey)
			continue
		}
		reply := newReply(&ev)
		select {
		case sink <- reply:
		default:
			s.r.Log.Warnf("reply buffer full for call [%s], dropping reply from %s", id, reply.FQDN)
		}
	}
}

// Call sends req and passes replies to onReply as they arrive, until onReply
// returns false or the context is done. Errors are transport errors; a node
// reporting a problem shows up as a Reply with NodeErr set
func (s *Session) Call(ctx context.Context, req Request, onReply func(Reply) bool) error {
	if _, ok := ctx.Deadline(); !ok {
		var cancel func()
		ctx, cancel = context.WithTimeout(ctx, DefaultQueryTimeout)
		defer cancel()
	}
	id := common.MapBytesToTopicTitle(s.r.RngBlob(8))
	sink := make(chan Reply, replyBuffer)
	s.l.Lock()
	if s.closed {
		s.l.Unlock()
		return fmt.Errorf("session is closed")
	}
	s.calls[id] = sink
	s.l.Unlock()
	defer func() {
		s.l.Lock()
		delete(s.calls, id)
		s.l.Unlock()
	}()

	ev := s.r.Node.NewEvent()
	err := ev.Marshal(&puppet.PuppetCmdSend{
		Command:      req.Command,
		Filter:       req.Filter,
		AnswerAlways: req.AnswerAlways,
		Parameters:   req.Params,
	})
	if err != nil {
		return fmt.Errorf("error marshalling %s command: %s", req.Command, err)
	}
	ev.ReplyTo = s.replyPath + "/" + id
	ev.Headers["correlation-id"] = id
	start := time.Now()
	if err := s.r.Node.SendEvent(req.Topic, ev); err != nil {
		return fmt.Errorf("error sending %s command: %s", req.Command, err)
	}
	for {
		select {
		case reply := <-sink:
			reply.RTT = time.Since(start)
			if onReply != nil && !onReply(reply) {
				return nil
			}
		case <-ctx.Done():
			return nil
		}
	}
}

// CallCollect gathers every reply that arrives before the deadline
func (s *Session) CallCollect(ctx context.Context, req Request) ([]Reply, error) {
	replies := make([]Reply, 0, 16)
	err := s.Call(ctx, req, func(r Reply) bool {
		replies = append(replies, r)
		return true
	})
	return replies, err
}

// Close stops routing replies. The subscription can not be cancelled, so it
// keeps being drained for a while and is then torn down the only way zerosvc
// allows: closing the channel makes its handler unsubscribe on the next message
func (s *Session) Close() error {
	s.l.Lock()
	if s.closed {
		s.l.Unlock()
		return nil
	}
	s.closed = true
	s.calls = make(map[string]chan Reply, 0)
	ch := s.ch
	s.l.Unlock()
	// the channel can not be closed: the node writes into it from a goroutine of
	// its own, so closing it would panic there. Draining it is the only way to
	// keep the transport unblocked
	go drain(ch)
	return nil
}

// ReplyPath is the topic replies to this session are sent to
func (s *Session) ReplyPath() string { return s.replyPath }

// callID finds which call a reply belongs to. Event.Reply copies the
// correlation-id header, and the reply topic ends with the same id, so this
// works with daemons that know nothing about sessions
func callID(ev *zerosvc.Event) string {
	if id, ok := ev.Headers["correlation-id"].(string); ok && len(id) > 0 {
		return id
	}
	if path := strings.Split(ev.RoutingKey, "/"); len(path) > 0 {
		return path[len(path)-1]
	}
	return ""
}

func newReply(ev *zerosvc.Event) Reply {
	r := Reply{Body: ev.Body, ev: *ev}
	if fqdn, ok := ev.Headers["fqdn"].(string); ok {
		r.FQDN = fqdn
	} else {
		// not every reply type sets the header
		r.FQDN = ev.NodeName
	}
	if replyType, ok := ev.Headers["reply-type"].(string); ok {
		r.ReplyType = replyType
	}
	if r.ReplyType == common.PuppetNoMatch {
		r.NoMatch = true
	}
	if r.ReplyType == common.Error {
		var msg puppet.Msg
		if err := ev.Unmarshal(&msg); err == nil && len(msg.Msg) > 0 {
			r.NodeErr = errors.New(msg.Msg)
		} else {
			r.NodeErr = fmt.Errorf("node reported an error: %s", string(ev.Body))
		}
	}
	return r
}
