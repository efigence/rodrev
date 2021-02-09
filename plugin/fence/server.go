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
			p.l.Errorf("Error handling puppet event[%s]: %s:", ev.NodeName(), err)
		}
	}
	return fmt.Errorf("channel for puppet server disconnected")
}


func (f *Fence) HandleEvent(ev *zerosvc.Event) error {
	var cmd FenceCmd
	err := ev.Unmarshal(&cmd)
	f.l.Debugf("got fence request from %s",ev.NodeName())
	if err != nil {
		return fmt.Errorf("error unmarshalling event from %s: %s", ev.NodeName(), err)
	}
	f.l.Infof("fencing %s", cmd.Node)
	initErr, runErr := (&fenceSelf{}).Self(time.Second)
	if initErr != nil {
		f.l.Errorf("error initializing fencing [%+v]: %s", cmd, err)
	}
	err = <- runErr
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

	return err
}