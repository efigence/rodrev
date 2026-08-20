package client

import (
	"context"
	"time"

	"github.com/efigence/rodrev/common"
	"github.com/efigence/rodrev/plugin/puppet"
	"github.com/zerosvc/go-zerosvc"
)

type Opts struct {
	Noop bool
}

// PuppetStatus returns the last puppet run summary of every node matching the
// optional filter
func PuppetStatus(r *common.Runtime, filter ...string) map[string]puppet.LastRunSummary {
	statusMap := make(map[string]puppet.LastRunSummary, 0)
	s, err := NewSession(r)
	if err != nil {
		r.Log.Errorf("%s", err)
		return statusMap
	}
	defer s.Close()
	ctx, cancel := context.WithTimeout(context.Background(), DefaultQueryTimeout)
	defer cancel()
	r.Log.Debugf("sending status request, waiting %s for replies", DefaultQueryTimeout)
	statusMap, coverage, err := PuppetStatusStream(ctx, s, oneFilter(filter), nil)
	if err != nil {
		r.Log.Errorf("%s", err)
	}
	r.Log.Infof("%s", coverage.String())
	return statusMap
}

// PuppetFact returns the value of one fact from every node matching the
// optional filter
func PuppetFact(r *common.Runtime, factName string, filter ...string) map[string]interface{} {
	facts := make(map[string]interface{}, 0)
	s, err := NewSession(r)
	if err != nil {
		r.Log.Errorf("%s", err)
		return facts
	}
	defer s.Close()
	ctx, cancel := context.WithTimeout(context.Background(), DefaultQueryTimeout)
	defer cancel()
	r.Log.Debugf("sending fact request, waiting %s for replies", DefaultQueryTimeout)
	facts, coverage, err := PuppetFactStream(ctx, s, factName, oneFilter(filter), nil)
	if err != nil {
		r.Log.Errorf("%s", err)
	}
	r.Log.Infof("%s", coverage.String())
	return facts
}

// PuppetRun triggers a puppet run on one node, or on every node matching the
// filter when node is "all".
//
// The returned channel is closed before it can be read from, so replies are not
// available through it; use a Session directly if you need them
func PuppetRun(r *common.Runtime, node string, filter string, delay time.Duration, opts Opts) chan zerosvc.Event {
	replyPath, replyCh, err := r.GetReplyChan()
	if err != nil {
		r.Log.Errorf("error getting reply channel: %s", err)
		return nil
	}
	query := r.Node.NewEvent()
	r.UnlikelyErr(query.Marshal(puppet.PuppetCmdSend{
		Command:    puppet.Run,
		Filter:     filter,
		Parameters: puppet.RunOptions{Delay: delay, RandomizeDelay: true, Noop: opts.Noop},
	}))

	query.ReplyTo = replyPath
	if node == "all" {
		err = r.Node.SendEvent("puppet", query)
	} else {
		err = r.Node.SendEvent("puppet/"+node, query)
	}
	if err != nil {
		r.Log.Errorf("err sending: %s", err)
	}
	return replyCh

}
