package ipset

import (
	"fmt"
	"github.com/efigence/go-ipset"
	"github.com/efigence/rodrev/common"
	"github.com/efigence/rodrev/config"
	"github.com/zerosvc/go-zerosvc"
	"go.uber.org/zap"
	"net"
)

type IPsetCmd struct {
	Net *net.IPNet
}

type IPSetManager struct {
	l *zap.SugaredLogger
}

func New(runtime *common.Runtime, cfg config.IPSetServer) (*IPSetManager, error) {
	ipsm := IPSetManager{}
	return &ipsm, nil
}

func (i *IPSetManager) EventListener(evCh chan zerosvc.Event, setname string) error {
	for ev := range evCh {
		var cmd IPsetCmd
		err := ev.Unmarshal(&cmd)
		set, err := ipset.NewNet("rv_"+setname, "hash:net", "counters", "timeout", "3600")
		if err != nil {
			i.l.Errorf("bad event: %s", err)
			continue
		}
		if cmd.Net != nil {
			err := set.Add(cmd.Net)
			if err != nil {
				i.l.Errorf("error adding %s: %s", cmd.Net.String(), err)
			}
		}
	}
	return fmt.Errorf("channel closed[%s]", setname)
}
