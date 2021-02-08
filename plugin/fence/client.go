package fence

import (
	"fmt"
	"github.com/efigence/rodrev/common"
	"time"
)

func SendFence(r *common.Runtime,node string) error {
	replyPath, replyCh, err := r.GetReplyChan()
	if err != nil {
		return fmt.Errorf("error getting reply channel: %s", err)
	}
	defer close(replyCh)
	cmd := r.Node.NewEvent()
	cmd.Marshal(FenceCmd{
		Priority: 0,
		Node:     node,
	})
	cmd.ReplyTo = replyPath
	err = cmd.Send(r.MQPrefix + "fence/" + node)
	select {
	case <-time.After(time.Second * 11):
		return fmt.Errorf("timed out")
	case ev := <- replyCh:
		// TODO error handling
		r.Log.Infof("got fence answer: %s",string(ev.Body))
		return nil
	}
}