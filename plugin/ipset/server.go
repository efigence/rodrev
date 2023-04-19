package ipset

import (
	"fmt"
	"github.com/efigence/rodrev/common"
	"github.com/efigence/rodrev/config"
	"github.com/zerosvc/go-zerosvc"
	"go.uber.org/zap"
)

type IPsetCmd struct {
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
		if err != nil {
			i.l.Errorf("bad event: %s", err)
			continue
		}
	}
	return fmt.Errorf("channel closed[%s]", setname)
}
