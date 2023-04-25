package ipset

import (
	"fmt"
	"github.com/efigence/rodrev/common"
	"time"
)

func Add(r *common.Runtime, group string, ipset string, addr string) error {
	replyPath, replyCh, err := r.GetReplyChan()
	if err != nil {
		return fmt.Errorf("error getting reply channel: %s", err)
	}
	defer close(replyCh)
	cmd := r.Node.NewEvent()
	cmd.Marshal(IPsetCmd{
		Addr:  addr,
		IPSet: ipset,
	})
	cmd.ReplyTo = replyPath
	cmd.Prepare()
	err = cmd.Send(r.MQPrefix + "fence/" + group + "/" + ipset)
	if err != nil {
		return fmt.Errorf("error sending fence request: %s", err)
	}
	tmout := time.After(time.Second * 5)
F:
	for {
		select {
		case <-tmout:
			break F
		case ev := <-replyCh:
			r.Log.Infof("got reply from %s", ev.NodeName())
		}
	}
	return nil
}
