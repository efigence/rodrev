package client

import (
	"context"
	"fmt"
	"time"

	"github.com/efigence/rodrev/common"
	"github.com/efigence/rodrev/plugin/puppet"
	"github.com/zerosvc/go-zerosvc"
)

// DefaultQueryTimeout is used when a context has no deadline of its own. It
// matches how long PuppetStatus has always waited for replies
const DefaultQueryTimeout = time.Second * 4

// FilterOutcome is who answered a filtered request
type FilterOutcome struct {
	Matched []string
	// NoMatch are nodes that said the filter did not match them. Only daemons
	// with the answer-always feature report this
	NoMatch []string
	Errors  map[string]string
}

// Answered is how many nodes took part in the request
func (o *FilterOutcome) Answered() int {
	return len(o.Matched) + len(o.NoMatch) + len(o.Errors)
}

// FilterMatch sends expr to every node as a status request filter and reports
// who matched, calling onNode for each match as it arrives.
//
// It asks for no-match answers, so on a fleet of new daemons Answered() is the
// real number of nodes that ran the query. Older daemons only answer when the
// filter matches, so they show up in neither NoMatch nor Answered
func (s *Session) FilterMatch(ctx context.Context, expr string, onNode func(fqdn string)) (FilterOutcome, error) {
	out := FilterOutcome{
		Matched: make([]string, 0, 16),
		NoMatch: make([]string, 0, 16),
		Errors:  make(map[string]string, 0),
	}
	seen := make(map[string]bool, 16)
	err := s.Call(ctx, Request{
		Topic:        "puppet",
		Command:      puppet.Status,
		Filter:       expr,
		AnswerAlways: true,
	}, func(reply Reply) bool {
		if len(reply.FQDN) == 0 || seen[reply.FQDN] {
			return true
		}
		seen[reply.FQDN] = true
		switch {
		case reply.NodeErr != nil:
			out.Errors[reply.FQDN] = reply.NodeErr.Error()
		case reply.NoMatch:
			out.NoMatch = append(out.NoMatch, reply.FQDN)
		default:
			out.Matched = append(out.Matched, reply.FQDN)
			if onNode != nil {
				onNode(reply.FQDN)
			}
		}
		return true
	})
	return out, err
}

// PuppetFilterMatch is FilterMatch for callers that do not keep a session around
func PuppetFilterMatch(ctx context.Context, r *common.Runtime, expr string, onNode func(fqdn string)) (FilterOutcome, error) {
	s, err := NewSession(r)
	if err != nil {
		return FilterOutcome{}, err
	}
	defer s.Close()
	return s.FilterMatch(ctx, expr, onNode)
}

// Coverage counts who took part in a request. NoMatch is only filled in by
// daemons that support answer-always
type Coverage struct {
	Matched int
	NoMatch int
	Errors  int
}

// Answered is how many nodes replied at all
func (c *Coverage) Answered() int { return c.Matched + c.NoMatch + c.Errors }

// String describes the coverage of one request
func (c *Coverage) String() string {
	return fmt.Sprintf("%d matched, %d answered", c.Matched, c.Answered())
}

// PuppetStatusStream collects last run summaries, reporting each as it arrives
func PuppetStatusStream(ctx context.Context, s *Session, filter string,
	onNode func(fqdn string, summary puppet.LastRunSummary)) (map[string]puppet.LastRunSummary, Coverage, error) {
	statusMap := make(map[string]puppet.LastRunSummary, 16)
	var coverage Coverage
	err := s.Call(ctx, Request{
		Topic:        "puppet",
		Command:      puppet.Status,
		Filter:       filter,
		AnswerAlways: true,
	},
		func(reply Reply) bool {
			if reply.NoMatch {
				coverage.NoMatch++
				return true
			}
			if reply.NodeErr != nil {
				coverage.Errors++
				s.r.Log.Errorf("error from %s: %s", reply.FQDN, reply.NodeErr)
				return true
			}
			var summary puppet.LastRunSummary
			if err := reply.Unmarshal(&summary); err != nil {
				s.r.Log.Errorf("error decoding reply from %s: %s", reply.FQDN, err)
				return true
			}
			statusMap[reply.FQDN] = summary
			coverage.Matched++
			if onNode != nil {
				onNode(reply.FQDN, summary)
			}
			return true
		})
	return statusMap, coverage, err
}

// PuppetFactStream collects one fact from every node, reporting each as it arrives
func PuppetFactStream(ctx context.Context, s *Session, factName, filter string,
	onNode func(fqdn string, value interface{})) (map[string]interface{}, Coverage, error) {
	facts := make(map[string]interface{}, 16)
	var coverage Coverage
	err := s.Call(ctx, Request{
		Topic:        "puppet",
		Command:      puppet.Fact,
		Filter:       filter,
		AnswerAlways: true,
		Params:       puppet.FactOptions{Name: factName},
	}, func(reply Reply) bool {
		if reply.NoMatch {
			coverage.NoMatch++
			return true
		}
		if reply.NodeErr != nil {
			coverage.Errors++
			s.r.Log.Errorf("error from %s: %s", reply.FQDN, reply.NodeErr)
			return true
		}
		var fact map[string]interface{}
		if err := reply.Unmarshal(&fact); err != nil {
			s.r.Log.Errorf("error decoding reply from %s: %s", reply.FQDN, err)
			return true
		}
		facts[reply.FQDN] = fact[factName]
		coverage.Matched++
		if onNode != nil {
			onNode(reply.FQDN, fact[factName])
		}
		return true
	})
	return facts, coverage, err
}

// drainFor keeps a channel drained for a while, then gives up on it. Anything
// arriving after that panics inside the zerosvc handler, which recovers and
// unsubscribes - the only cleanup the library offers
func drainFor(ch chan zerosvc.Event, d time.Duration) {
	deadline := time.After(d)
	for {
		select {
		case _, ok := <-ch:
			if !ok {
				return
			}
		case <-deadline:
			close(ch)
			return
		}
	}
}

// oneFilter keeps the variadic filter argument the old API uses
func oneFilter(filter []string) string {
	if len(filter) > 1 {
		panic("filter accepts 0 or 1 arguments")
	}
	if len(filter) == 1 {
		return filter[0]
	}
	return ""
}

// QueryOutcome is the result of a Query command run across the fleet. Unlike a
// filtered status request every node answers, so a node that did not match can
// be told apart from one that is not running
type QueryOutcome struct {
	Expr    string
	Matched []string
	// NotMatched are nodes that answered but did not match
	NotMatched []string
	// Errors are nodes where the query itself failed, by fqdn
	Errors map[string]string
	// Values are non-boolean results, by fqdn
	Values map[string]string
}

// Query evaluates expr on every node and reports what each of them said.
// Requires a daemon supporting the query command; older ones answer with an
// "unknown command" error, which shows up in Errors
func (s *Session) Query(ctx context.Context, expr string, onNode func(puppet.QueryReply)) (QueryOutcome, error) {
	out := QueryOutcome{
		Expr:       expr,
		Matched:    make([]string, 0, 16),
		NotMatched: make([]string, 0, 16),
		Errors:     make(map[string]string, 0),
		Values:     make(map[string]string, 0),
	}
	seen := make(map[string]bool, 16)
	err := s.Call(ctx, Request{Topic: "puppet", Command: puppet.Query, Filter: expr},
		func(reply Reply) bool {
			var qr puppet.QueryReply
			if reply.NodeErr != nil {
				// an old daemon rejecting the command, or a node-side failure
				qr = puppet.QueryReply{FQDN: reply.FQDN, Error: reply.NodeErr.Error()}
			} else if err := reply.Unmarshal(&qr); err != nil {
				qr = puppet.QueryReply{FQDN: reply.FQDN, Error: fmt.Sprintf("undecodable reply: %s", err)}
			}
			if len(qr.FQDN) == 0 {
				qr.FQDN = reply.FQDN
			}
			if len(qr.FQDN) == 0 || seen[qr.FQDN] {
				return true
			}
			seen[qr.FQDN] = true
			switch {
			case len(qr.Error) > 0:
				out.Errors[qr.FQDN] = qr.Error
			case qr.Matched:
				out.Matched = append(out.Matched, qr.FQDN)
			default:
				out.NotMatched = append(out.NotMatched, qr.FQDN)
			}
			if len(qr.Value) > 0 {
				out.Values[qr.FQDN] = qr.Value
			}
			if onNode != nil {
				onNode(qr)
			}
			return true
		})
	return out, err
}

// NodeFacts pulls the full fact set from one node
func (s *Session) NodeFacts(ctx context.Context, fqdn string, keys ...string) (*puppet.FactsReply, error) {
	var out *puppet.FactsReply
	err := s.Call(ctx, Request{
		Topic:   "puppet/" + fqdn,
		Command: puppet.FactList,
		Params:  puppet.FactListOptions{Keys: keys},
	}, func(reply Reply) bool {
		if reply.NodeErr != nil {
			return true
		}
		var facts puppet.FactsReply
		if err := reply.Unmarshal(&facts); err != nil {
			s.r.Log.Errorf("error decoding fact dump from %s: %s", reply.FQDN, err)
			return true
		}
		out = &facts
		// only the node we asked answers a unicast dump
		return false
	})
	if err != nil {
		return nil, err
	}
	if out == nil {
		return nil, fmt.Errorf("no fact dump from %s: it may be running a daemon without fact dump support", fqdn)
	}
	return out, nil
}

// NodeClasses pulls the class list from one node
func (s *Session) NodeClasses(ctx context.Context, fqdn string) (*puppet.ClassesReply, error) {
	var out *puppet.ClassesReply
	err := s.Call(ctx, Request{Topic: "puppet/" + fqdn, Command: puppet.ClassList},
		func(reply Reply) bool {
			if reply.NodeErr != nil {
				return true
			}
			var classes puppet.ClassesReply
			if err := reply.Unmarshal(&classes); err != nil {
				s.r.Log.Errorf("error decoding class list from %s: %s", reply.FQDN, err)
				return true
			}
			out = &classes
			return false
		})
	if err != nil {
		return nil, err
	}
	if out == nil {
		return nil, fmt.Errorf("no class list from %s: it may be running a daemon without class dump support", fqdn)
	}
	return out, nil
}

// NodeSnapshot pulls facts and classes from one node, both at once
func (s *Session) NodeSnapshot(ctx context.Context, fqdn string) (*puppet.Snapshot, error) {
	type result struct {
		facts   *puppet.FactsReply
		classes *puppet.ClassesReply
		err     error
	}
	factCh := make(chan result, 1)
	classCh := make(chan result, 1)
	go func() {
		facts, err := s.NodeFacts(ctx, fqdn)
		factCh <- result{facts: facts, err: err}
	}()
	go func() {
		classes, err := s.NodeClasses(ctx, fqdn)
		classCh <- result{classes: classes, err: err}
	}()
	factRes, classRes := <-factCh, <-classCh
	if factRes.err != nil {
		return nil, factRes.err
	}
	if classRes.err != nil {
		// facts alone are still worth having, queries on classes just miss
		s.r.Log.Warnf("%s", classRes.err)
	}
	return puppet.SnapshotFromReplies(factRes.facts, classRes.classes), nil
}
