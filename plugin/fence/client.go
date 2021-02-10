package fence

import (
	"fmt"
	"github.com/efigence/rodrev/common"
	"time"
)

const (
	cmdStatus  = "status"
	cmdFence   = "fence"
)


func Send(r *common.Runtime,node string) error{
	replyPath, replyCh, err := r.GetReplyChan()
	if err != nil {
		return fmt.Errorf("error getting reply channel: %s", err)
	}
	defer close(replyCh)
	cmd := r.Node.NewEvent()
	cmd.Marshal(FenceCmd{
		Command: cmdFence,
		Priority: 0,
		Node:     node,
	})
	cmd.ReplyTo = replyPath
	err = cmd.Send(r.MQPrefix + "fence/" + node)
	if err != nil {
		return fmt.Errorf("error sending fence request: %s", err)
	}
	select {
	case <-time.After(time.Second * 11):
		return fmt.Errorf("timed out")
	case ev := <- replyCh:
		// TODO error handling
		r.Log.Infof("got fence answer: %s",string(ev.Body))
		return nil
	}
}

func Status(r *common.Runtime,node string) (ok bool,err error) {
	replyPath, replyCh, err := r.GetReplyChan()
	if err != nil {
		return false,fmt.Errorf("error getting reply channel: %s", err)
	}
	defer close(replyCh)
	cmd := r.Node.NewEvent()
	cmd.Marshal(FenceCmd{
		Command: cmdStatus,
		Priority: 0,
		Node:     node,
	})
	cmd.ReplyTo = replyPath
	err = cmd.Send(r.MQPrefix + "fence/" + node)
	if err != nil {
		return false, fmt.Errorf("error sending status request: %s", err)
	}
	select {
	case <-time.After(time.Second * 11):
		return false,fmt.Errorf("timed out")
	case ev := <- replyCh:
		_ = ev
		// TODO error handling
		r.Log.Infof("ping %s ok",node)
		return true,nil
	}
}