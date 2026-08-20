package puppet

import (
	"fmt"
	"github.com/efigence/rodrev/common"
	"github.com/efigence/rodrev/util"
	"github.com/fxamacker/cbor/v2"
	"github.com/zerosvc/go-zerosvc"
	"os"
	"strings"
)
import "time"

func (p *Puppet) StartServer() {
	go p.backgroundWorker()
}

func (p *Puppet) EventListener(evCh chan zerosvc.Event) error {
	for ev := range evCh {
		err := p.HandleEvent(&ev)
		if err != nil {
			p.l.Errorf("Error handling puppet event[%s]: %s:", ev.NodeName, err)
		}
	}
	return fmt.Errorf("channel for puppet server disconnected")
}

// handlerResult is what a command decided to answer with
type handlerResult struct {
	Body      interface{}
	ReplyType string
	// Send is false when the node should stay quiet: the filter did not match,
	// or the request was addressed to somebody else
	Send bool
}

func (p *Puppet) HandleEvent(ev *zerosvc.Event) error {
	var cmd PuppetCmdRecv
	err := ev.Unmarshal(&cmd)
	if err != nil {
		return p.puppetErr(err)
	}
	if len(ev.ReplyTo) == 0 {
		return fmt.Errorf("no reply-to in incoming event, aborting: %+v", ev)
	}
	if p.runtime.Debug {
		p.l.Debugf("incoming event: %s", util.PPEvent(ev))
	}
	reqPath := strings.Split(ev.RoutingKey, "/")
	if len(reqPath) < 2 {
		return fmt.Errorf("too short path, ignoring: [%s]%s", reqPath, ev.RoutingKey)
	}
	res, handlerErr := p.handleCommand(cmd, reqPath)
	if !res.Send {
		return handlerErr
	}
	re := p.node.PrepareReply(*ev)
	// the identity settled at startup, not whatever dns says right now - it has
	// to match the fqdn the command bodies carry
	re.Headers["fqdn"] = p.fqdn
	if err := re.Marshal(res.Body); err != nil {
		return fmt.Errorf("error marshalling %s reply: %s", cmd.Command, err)
	}
	re.Headers["reply-type"] = res.ReplyType
	if err := p.runtime.Reply(ev, re); err != nil {
		return err
	}
	return handlerErr
}

// handleCommand runs a command and returns what to answer with. It touches no
// MQ, so it can be tested on its own
func (p *Puppet) handleCommand(cmd PuppetCmdRecv, reqPath []string) (handlerResult, error) {
	// Query is the filter itself, so it runs before the filter gate
	if cmd.Command == Query {
		return p.handleQuery(cmd)
	}
	if len(cmd.Filter) > 0 {
		ok, err := p.query.ParseBool(cmd.Filter)
		if err != nil {
			// tell the client its filter is broken instead of looking like a
			// node that just did not match
			return handlerResult{
				Body:      &Msg{Msg: fmt.Sprintf("filter error: %s", err)},
				ReplyType: common.Error,
				Send:      true,
			}, fmt.Errorf("remote query error: %s", err)
		}
		if !ok {
			p.l.Debugf("node skipped by query filter %s", cmd.Filter)
			if cmd.AnswerAlways {
				// the client wants to know how many nodes took part, so say
				// "not me" instead of staying quiet
				return handlerResult{
					Body:      &QueryReply{FQDN: p.fqdn, Matched: false},
					ReplyType: common.PuppetNoMatch,
					Send:      true,
				}, nil
			}
			return handlerResult{}, nil
		}
	}
	switch cmd.Command {
	case Status:
		p.lock.RLock()
		summary := p.lastRunSummary
		p.lock.RUnlock()
		return handlerResult{Body: summary, ReplyType: summary.RPCType(), Send: true}, nil
	case Run:
		var opts RunOptions
		if err := cbor.Unmarshal(cmd.Parameters, &opts); err != nil {
			return handlerResult{}, fmt.Errorf("error unmarshalling puppet command: %s|[%s]", err, string(cmd.Parameters))
		}
		if !p.addressedToMe(reqPath) {
			p.l.Debugf("got request for path %s, ignoring as it does not match %s", reqPath, p.fqdn)
			return handlerResult{}, nil
		}
		r := p.Run(opts)
		return handlerResult{Body: &r, ReplyType: r.RPCType(), Send: true}, nil
	case Fact:
		var opts FactOptions
		if err := cbor.Unmarshal(cmd.Parameters, &opts); err != nil {
			return handlerResult{}, fmt.Errorf("error unmarshalling [%s]: %s", string(cmd.Parameters), err)
		}
		facts := *(p.facts.Map())
		return handlerResult{
			Body:      map[string]interface{}{opts.Name: facts[opts.Name]},
			ReplyType: common.PuppetFact,
			Send:      true,
		}, nil
	case FactList:
		return p.handleFactList(cmd, reqPath)
	case ClassList:
		switch p.dumpDecision(cmd, reqPath) {
		case dumpIgnore:
			return handlerResult{}, nil
		case dumpRefuse:
			return dumpRefused(ClassList), nil
		}
		reply := ClassesReply{
			FQDN:    p.fqdn,
			TS:      time.Now(),
			Classes: p.classes.List(),
		}
		return handlerResult{Body: &reply, ReplyType: reply.RPCType(), Send: true}, nil
	default:
		p.l.Warnf("unknown command %s [%+v]", cmd.Command, reqPath)
		return handlerResult{
			Body:      &Msg{Msg: "unknown command " + cmd.Command},
			ReplyType: common.Error,
			Send:      true,
		}, nil
	}
}

// handleQuery evaluates an expression and always answers, so the client can
// count nodes that matched, did not match, and could not run it at all
func (p *Puppet) handleQuery(cmd PuppetCmdRecv) (handlerResult, error) {
	reply := QueryReply{FQDN: p.fqdn}
	if len(cmd.Filter) == 0 {
		reply.Error = "empty query"
		return handlerResult{Body: &reply, ReplyType: reply.RPCType(), Send: true}, nil
	}
	res, err := p.query.Parse(cmd.Filter)
	switch {
	case err != nil:
		reply.Error = err.Error()
	case !res.Boolish:
		reply.Type = res.Type
		reply.Value = res.Display
		reply.Error = fmt.Sprintf("query returned %s, not a boolean", res.Type)
	default:
		reply.Matched = res.Bool
		reply.Type = res.Type
	}
	return handlerResult{Body: &reply, ReplyType: reply.RPCType(), Send: true}, nil
}

func (p *Puppet) handleFactList(cmd PuppetCmdRecv, reqPath []string) (handlerResult, error) {
	switch p.dumpDecision(cmd, reqPath) {
	case dumpIgnore:
		return handlerResult{}, nil
	case dumpRefuse:
		return dumpRefused(FactList), nil
	}
	var opts FactListOptions
	if len(cmd.Parameters) > 0 {
		if err := cbor.Unmarshal(cmd.Parameters, &opts); err != nil {
			return handlerResult{}, fmt.Errorf("error unmarshalling [%s]: %s", string(cmd.Parameters), err)
		}
	}
	facts := factSubset(*(p.facts.Map()), opts.Keys)
	reply := FactsReply{
		FQDN:  p.fqdn,
		TS:    time.Now(),
		Count: len(facts),
		Facts: facts,
	}
	return handlerResult{Body: &reply, ReplyType: reply.RPCType(), Send: true}, nil
}

// dumpDecision is what to do with a fact/class dump request
type dumpDecision int

const (
	// dumpAnswer sends the dump
	dumpAnswer dumpDecision = iota
	// dumpIgnore stays quiet: the request named a different node
	dumpIgnore
	// dumpRefuse explains why the dump is not being sent
	dumpRefuse
)

// dumpDecision keeps a fact/class dump from being answered by the whole fleet:
// a full fact set is tens of kilobytes per node. It has to be addressed at this
// node or narrowed down by a filter.
//
// Note that a request sent to puppet/<fqdn> reaches every daemon (they all
// subscribe to puppet/#), which is why the "somebody else was asked" case has to
// be silent - answering it would turn one unicast dump into a fleet-wide reply
// storm
func (p *Puppet) dumpDecision(cmd PuppetCmdRecv, reqPath []string) dumpDecision {
	switch {
	case p.unicastToMe(reqPath):
		return dumpAnswer
	case unicastToOther(reqPath):
		return dumpIgnore
	case len(cmd.Filter) > 0:
		// the filter gate already ran, so getting here means it matched
		return dumpAnswer
	default:
		return dumpRefuse
	}
}

func dumpRefused(command string) handlerResult {
	return handlerResult{
		Body: &Msg{Msg: command + " needs to be addressed at a node (" +
			"send it to puppet/<fqdn>) or narrowed down with a filter"},
		ReplyType: common.Error,
		Send:      true,
	}
}

// unicastToMe reports whether the request named this node explicitly
func (p *Puppet) unicastToMe(reqPath []string) bool {
	if len(reqPath) < 2 {
		return false
	}
	return reqPath[len(reqPath)-1] == p.fqdn && reqPath[len(reqPath)-2] == "puppet"
}

// unicastToOther reports whether the request named some other node
func unicastToOther(reqPath []string) bool {
	if len(reqPath) < 2 {
		return false
	}
	last := reqPath[len(reqPath)-1]
	return reqPath[len(reqPath)-2] == "puppet" && last != "puppet"
}

// addressedToMe reports whether a request was meant for this node. Every daemon
// subscribes to the whole puppet/# tree, so a request sent to puppet/<fqdn> is
// delivered to the entire fleet and has to be filtered here
func (p *Puppet) addressedToMe(reqPath []string) bool {
	if len(reqPath) < 2 {
		return false
	}
	last := reqPath[len(reqPath)-1]
	previous := reqPath[len(reqPath)-2]
	switch {
	case last == p.fqdn && previous == "puppet": // unicast
		return true
	case last == "puppet" && len(reqPath) == 2: // broadcast
		return true
	case last == "puppet" && previous != "puppet": // broadcast under a prefix
		return true
	}
	return false
}

// factSubset returns only the requested keys, or everything when none are given
func factSubset(facts map[string]interface{}, keys []string) map[string]interface{} {
	if len(keys) == 0 {
		return facts
	}
	out := make(map[string]interface{}, len(keys))
	for _, key := range keys {
		if v, ok := facts[key]; ok {
			out[key] = v
		}
	}
	return out
}

func (p *Puppet) backgroundWorker() {
	for {
		p.updateLastRunSummary()
		p.updateFacts()
		p.updateClasses()
		time.Sleep(p.cfg.RefreshInterval)

	}

}

func (p *Puppet) updateLastRunSummary() {
	fd, err := os.Open(p.cfg.LastRunSummaryYAML)
	if err != nil {
		p.l.Errorf("could not open puppet run summary [%s]: %s", p.cfg.LastRunSummaryYAML, err)
		return
	}
	summary, err := ParseLastRunSummary(fd)
	if err != nil {
		p.l.Warnf("error parsing last run summary: %s")
		return
	}
	p.lock.Lock()
	defer p.lock.Unlock()
	p.lastRunSummary = summary
}
func (p *Puppet) updateFacts() {
	err := p.facts.UpdateFacts()
	if err != nil {
		p.l.Warnf("error updating facts: %s", err)
	}

}

func (p *Puppet) updateClasses() {
	err := p.classes.UpdateClasses()
	if err != nil {
		p.l.Warnf("error updating classes: %s", err)
	}

}
