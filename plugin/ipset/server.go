package ipset

import (
	"fmt"
	"github.com/XANi/goneric"
	"github.com/efigence/go-ipset"
	"github.com/efigence/rodrev/common"
	"github.com/efigence/rodrev/config"
	"github.com/zerosvc/go-zerosvc"
	"go.uber.org/zap"
	"strconv"
)

type IPSetManager struct {
	l       *zap.SugaredLogger
	sets    map[string]ipset.IPSet
	runtime *common.Runtime
}

func New(runtime *common.Runtime, cfg config.IPSetServer) (*IPSetManager, error) {
	errList := []error{}
	ipsm := IPSetManager{
		l:       runtime.Log,
		runtime: runtime,
		sets: goneric.MapMap(func(k string, v config.IPSet) (string, ipset.IPSet) {
			extraParams := []string{}
			if v.Timeout.Seconds() > 1 {
				extraParams = append(extraParams, "timeout", strconv.Itoa(int(v.Timeout.Seconds())))
			}
			switch v.Type {
			case "hash:ip", "bitmap:ip":
				ipset, ipsetErr := ipset.NewIP(v.Name, v.Type, extraParams...)
				if ipsetErr != nil {
					errList = append(errList, ipsetErr)
					return "", nil
				}
				return v.Name, ipset
			case "hash:net":
				ipset, ipsetErr := ipset.NewNet(v.Name, v.Type, extraParams...)
				if ipsetErr != nil {
					errList = append(errList, ipsetErr)
					return "", nil
				}
				return v.Name, ipset
			default:
				errList = append(errList, fmt.Errorf("ipset type [%s] not supported", v.Type))
			}
			return "", nil
		}, cfg.Sets),
	}
	delete(ipsm.sets, "")
	if len(errList) > 0 {
		return nil, fmt.Errorf("errors: %+v", errList)
	}
	return &ipsm, nil
}

func (i *IPSetManager) EventListener(evCh chan zerosvc.Event, setname string) error {
	for ev := range evCh {
		var cmd IPsetCmd
		err := ev.Unmarshal(&cmd)
		if err != nil {
			i.l.Errorf("error decoding command: %s", err)
		}
		if set, ok := i.sets[cmd.IPSet]; ok {
			err := set.Add(cmd.Addr)
			if err != nil {
				i.l.Errorf("error adding to set[%s]: %s", cmd.IPSet, err)
			}
		} else {
			i.l.Errorf("got command sent to nonexisting set: %s/%s", cmd.IPSet, cmd.Addr)
		}
	}
	return fmt.Errorf("channel closed[%s]", setname)
}
