package client

import (
	"github.com/efigence/rodrev/common"
	"github.com/efigence/rodrev/plugin/puppet"
	"github.com/zerosvc/go-zerosvc"
	"time"
)

func PuppetStatus(r *common.Runtime, filter ...string) map[string]puppet.LastRunSummary {
	statusMap := make(map[string]puppet.LastRunSummary, 0)
	replyPath, replyCh, err := r.GetReplyChan()
	if err != nil {
		r.Log.Errorf("error getting reply channel: %s", err)
	}
	defer close(replyCh)
	query := r.Node.NewEvent()
	f := ""
	if len(filter) == 1 {
		f = filter[0]
	}
	if len(filter) > 1 {
		panic("filter accepts 0 or 1 arguments")
	}
	err = query.Marshal(&puppet.PuppetCmdSend{
		Command:    puppet.Status,
		Filter:     f,
		Parameters: nil,
	})
	if err != nil {
		r.Log.Panicf("error marshalling command: %s", err)
	}
	query.ReplyTo = replyPath
	r.Log.Info("sending command")
	err = query.Send(r.MQPrefix + "puppet")
	if err != nil {
		r.Log.Errorf("err sending: %s", err)
	}
	r.Log.Info("waiting 4s for response")
	go func() {
		for ev := range replyCh {
			var summary puppet.LastRunSummary
			var fqdn string
			if v, ok := ev.Headers["fqdn"].(string); !ok {
				r.Log.Warnf("skipping message, no fqdn header: %s", ev)
				continue
			} else {
				fqdn = v
			}
			err := ev.Unmarshal(&summary)
			if err != nil {
				r.Log.Errorf("error decoding message: %s", err)
				continue
			}
			statusMap[fqdn] = summary
		}
	}()
	time.Sleep(time.Second * 4)
	return statusMap
}

func PuppetRun(r *common.Runtime, node string, filter string, delay time.Duration) chan zerosvc.Event {
	replyPath, replyCh, err := r.GetReplyChan()
	if err != nil {
		r.Log.Errorf("error getting reply channel: %s", err)
	}
	defer close(replyCh)
	query := r.Node.NewEvent()
	r.UnlikelyErr(query.Marshal(puppet.PuppetCmdSend{
		Command:    puppet.Run,
		Filter:     filter,
		Parameters: puppet.RunOptions{Delay: delay, RandomizeDelay: true},
	}))

	query.ReplyTo = replyPath
	if node == "all" {
		err = query.Send(r.MQPrefix + "puppet")
	} else {
		err = query.Send(r.MQPrefix + "puppet" + "/" + node)
	}
	if err != nil {
		r.Log.Errorf("err sending: %s", err)
	}
	return replyCh

}
