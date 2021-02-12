package fence

import (
	"fmt"
	"github.com/efigence/rodrev/common"
	"github.com/efigence/rodrev/config"
	"github.com/efigence/rodrev/util"
	"github.com/zerosvc/go-zerosvc"
	"go.uber.org/zap"
	"time"
)

const (
	FenceLocalSysrq = "local_sysrq"
	FenceRemoteLibvirt = "remote_libvirt"
)



var DefaultConfig = config.FenceConfig {
   Type: FenceLocalSysrq,
}

type FenceModule interface {
	// fences self after a delay
	// initError is "the fence method doesn't appear to work"
	// it should be returned after any pre-flight checks are done
	// runError is "I tried to fence and failed"
	// run error should return `nil` after delay or error if the fencing failed
	Self(delay time.Duration) (initError error, runError chan error)
	// same as Self but targets different node
	Node(nodeName string, delay time.Duration)  (initError error, runError chan error)
}

type Fence struct {
	cfg *config.FenceConfig
	fenceModule FenceModule
	l *zap.SugaredLogger
	node *zerosvc.Node
}


type FenceCmd struct {
	Command string
	Priority int
	Node string
}
type FenceResponse struct {
	Priority int
	Success bool
}


func New(runtime *common.Runtime,cfg config.FenceConfig) (*Fence, error) {
	var f Fence
	f.cfg = &cfg
	f.l  = cfg.Logger
	f.node = runtime.Node
	return &f, nil
}


func (p *Fence) EventListener(evCh chan zerosvc.Event) error {
	for ev := range evCh {
		err := p.HandleEvent(&ev)
		if err != nil {
			p.l.Errorf("Error handling fence event[%s]: %s", ev.NodeName(), err)
		}
	}
	return fmt.Errorf("channel for puppet server disconnected")
}

func (f *Fence) CheckPermissions(ev *zerosvc.Event, cmd *FenceCmd) (allowed bool, err error) {
	if (f.cfg.Group == "" && len(f.cfg.NodeMap) == 0) {
		return true, nil
	}
	if f.cfg.Group != "" {
		if v, ok := ev.Headers["fence-group"]; ok {
			if v == f.cfg.Group {
				return true, nil
			}
		}
	}
	if len(f.cfg.NodeMap) > 0 {
		allowedTargets := f.cfg.NodeMap[cmd.Node].Nodes
		for _,n :=  range allowedTargets {
			if n == util.GetFQDN() {
				return true, nil
			}
		}
	}
	return false,nil
}

func (f *Fence) HandleEvent(ev *zerosvc.Event) error {
	var cmd FenceCmd
	err := ev.Unmarshal(&cmd)
	if err != nil {
		return fmt.Errorf("error unmarshalling event from %s: %s", ev.NodeName(), err)
	}
	allowed, err := f.CheckPermissions(ev, &cmd)
	if !allowed {
		return fmt.Errorf("node %s is not permitted to fence us [group:%s]", ev.NodeName(),ev.Headers["fence-group"])
	}
	switch cmd.Command {
	case cmdFence:
		f.l.Debugf("got fence request from %s",ev.NodeName())

		initErr, runErr := (&fenceSelf{}).Self(time.Second * 11)
		if initErr != nil {
			f.l.Errorf("error initializing fencing [%+v]: %s", cmd, err)
		}
		err = <-runErr
		re := f.node.NewEvent()
		re.Headers["fqdn"] = util.GetFQDN()
		resp := FenceResponse{}

		if err != nil {
			f.l.Errorf("error running fencing [%+v]: %s", cmd, err)
		} else {
			resp.Success = true
		}
		re.Marshal(resp)
		ev.Reply(re)
	case cmdStatus:
		f.l.Infof("status request from %s[%s]",ev.NodeName(),ev.Headers["fqdn"])
		// TODO check fence status
		resp := FenceResponse{}
		resp.Success = true
		re := f.node.NewEvent()
		re.Headers["fqdn"] = util.GetFQDN()
		re.Marshal(resp)
		ev.Reply(re)
	default:
		f.l.Warnf("got unknown command [%s] from %s",cmd.Command, ev.NodeName())

	}

	return err
}