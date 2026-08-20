package client

import (
	"context"
	"fmt"
	"time"

	"github.com/efigence/rodrev/common"
	"github.com/efigence/rodrev/plugin/puppet"
	"github.com/zerosvc/go-zerosvc"
)

// DefaultQueryTimeout is used when the context has no deadline of its own
const DefaultQueryTimeout = time.Second * 4

// PuppetFilterMatch sends expr to every node as a status request filter and
// returns the fqdns of the nodes that matched, calling onNode for each as it
// arrives. Nodes that do not match send nothing back - that is how a filter
// works on the node side - so this can not tell "did not match" apart from
// "is not running".
//
// Unlike PuppetStatus it stops at the context deadline instead of always
// sleeping 4s, and it reports subscribe/send failures instead of looking like
// nothing matched
func PuppetFilterMatch(ctx context.Context, r *common.Runtime, expr string, onNode func(fqdn string)) ([]string, error) {
	matched := make([]string, 0, 16)
	if _, ok := ctx.Deadline(); !ok {
		var cancel func()
		ctx, cancel = context.WithTimeout(ctx, DefaultQueryTimeout)
		defer cancel()
	}
	replyPath, replyCh, err := r.GetReplyChan()
	if err != nil {
		return matched, fmt.Errorf("can't subscribe for replies: %s", err)
	}
	// see DiscoverOnce: a subscription nobody reads any more blocks the whole
	// MQTT client, so late replies have to keep being consumed
	defer func() { go drainFor(replyCh, time.Second*30) }()
	ev := r.Node.NewEvent()
	err = ev.Marshal(&puppet.PuppetCmdSend{
		Command: puppet.Status,
		Filter:  expr,
	})
	if err != nil {
		return matched, fmt.Errorf("error marshalling query: %s", err)
	}
	ev.ReplyTo = replyPath
	if err := ev.Send(r.MQPrefix + "puppet"); err != nil {
		return matched, fmt.Errorf("error sending query: %s", err)
	}
	seen := make(map[string]bool, 16)
	for {
		select {
		case reply, ok := <-replyCh:
			if !ok {
				// the transport closed the channel, which is how it reports a
				// lost connection
				return matched, fmt.Errorf("connection to %s lost while waiting for replies",
					common.RedactURL(r.Cfg.MQAddress))
			}
			fqdn, ok := reply.Headers["fqdn"].(string)
			if !ok {
				r.Log.Debugf("skipping reply without fqdn header: %s", reply.RoutingKey)
				continue
			}
			if seen[fqdn] {
				continue
			}
			seen[fqdn] = true
			matched = append(matched, fqdn)
			if onNode != nil {
				onNode(fqdn)
			}
		case <-ctx.Done():
			return matched, nil
		}
	}
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
