package fence

import (
	"fmt"
	"github.com/zerosvc/go-zerosvc"
	"go.uber.org/zap"
	"time"
)

const (
	FenceLocalSysrq = "local_sysrq"
	FenceRemoteLibvirt = "remote_libvirt"
)


type Config struct {
	Whitelist map[string]string
	Type string
}

var DefaultConfig = Config {
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
	cfg *Config
	fenceModule FenceModule
	l *zap.SugaredLogger
}


type FenceCmd struct {
	Priority int
	Node string
}


func New(cfg Config) (*Fence, error) {
	var f Fence
	f.cfg = &cfg
	return &f, nil
}

func (f *Fence) HandleEvent(ev *zerosvc.Event) error {
	var cmd FenceCmd
	err := ev.Unmarshal(&cmd)
	f.l.Debugf("got fence request from %s",ev.NodeName())
	if err != nil {
		return fmt.Errorf("error unmarshalling event from %s: %s", ev.NodeName(), err)
	}
	f.l.Infof("fencing %s", cmd.Node)
	initErr, runErr := fenceSelf{}.Self(time.Second)
	if initErr != nil {
		f.l.Errorf("error initializing fencing [%+v]: %s", cmd, err)
	}
	err = <- runErr
	if runErr != nil {
		f.l.Errorf("error running fencing [%+v]: %s", cmd, err)
	}

	return nil
}