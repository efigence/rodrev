package ipset

import (
	"fmt"
	"github.com/efigence/rodrev/common"
)

func Cmd(r *common.Runtime, command Command, group string, ipset string, addr string) error {
	replyPath, replyCh, err := r.GetReplyChan()
	if err != nil {
		return fmt.Errorf("error getting reply channel: %s", err)
	}
	defer close(replyCh)
	cmd := r.Node.NewEvent()
	cmd.Marshal(IPsetCmd{
		Cmd:   command,
		Addr:  addr,
		IPSet: ipset,
	})
	cmd.ReplyTo = replyPath
	err = r.Node.SendEvent("ipset/"+group+"/"+ipset, cmd)
	if err != nil {
		return fmt.Errorf("error sending ipset request: %s", err)
	}
	// wait for reply
	//	tmout := time.After(time.Second * 5)
	//F:
	//	for {
	//		select {
	//		case <-tmout:
	//			break F
	//		case ev := <-replyCh:
	//			r.Log.Infof("got reply from %s", ev.NodeName())
	//		}
	//	}
	return nil
}
