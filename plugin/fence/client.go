package fence

import (
	"fmt"
	"github.com/efigence/rodrev/common"
	"time"
)

const (
	cmdStatus = "status"
	cmdFence  = "fence"
)

func Send(r *common.Runtime, node string) error {
	replyPath, replyCh, err := r.GetReplyChan()
	if err != nil {
		return fmt.Errorf("error getting reply channel: %s", err)
	}
	defer close(replyCh)
	cmd := r.Node.NewEvent()
	if len(r.Cfg.Fence.Group) > 0 {
		cmd.Headers["fence-group"] = r.Cfg.Fence.Group
	}
	cmd.Marshal(FenceCmd{
		Command:  cmdFence,
		Priority: 0,
		Node:     node,
	})
	cmd.ReplyTo = replyPath
	cmd.Prepare()
	err = cmd.Send(r.MQPrefix + "fence/" + node)
	if err != nil {
		return fmt.Errorf("error sending fence request: %s", err)
	}
	select {
	case <-time.After(time.Second * 21): // change server timer too
		return fmt.Errorf("timed out")
		//FIXME add better timeout from fencer
	case ev := <-replyCh:
		// TODO error handling
		r.Log.Infof("got fence answer: %s", string(ev.Body))
		return nil
	}
}

func Status(r *common.Runtime, node string) (ok bool, err error) {
	replyPath, replyCh, err := r.GetReplyChan()
	if err != nil {
		return false, fmt.Errorf("error getting reply channel: %s", err)
	}
	defer close(replyCh)
	cmd := r.Node.NewEvent()
	if len(r.Cfg.Fence.Group) > 0 {
		cmd.Headers["fence-group"] = r.Cfg.Fence.Group
	}
	cmd.Marshal(FenceCmd{
		Command:  cmdStatus,
		Priority: 0,
		Node:     node,
	})
	cmd.ReplyTo = replyPath
	cmd.Prepare()
	errCh := make(chan error, 1)
	go func() {
		err = cmd.Send(r.MQPrefix + "fence/" + node)
		if err != nil {
			errCh <- err
		}
	}()
	select {
	case <-time.After(time.Second * 11):
		return false, fmt.Errorf("timed out")
	case err = <-errCh:
		return false, err
	case ev := <-replyCh:
		_ = ev
		// TODO error handling
		r.Log.Infof("ping %s ok", node)
		return true, nil
	}
}
