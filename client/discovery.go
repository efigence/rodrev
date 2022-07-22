package client

import (
	"encoding/json"
	"github.com/efigence/rodrev/common"
	"github.com/zerosvc/go-zerosvc"
	"log"
	"strings"
	"time"
)

// Discover rea
func Discover(r *common.Runtime) (
	serviceMap map[string][]common.Node,
	nodesActive map[string]common.Node,
	nodesStale map[string]common.Node,
	err error,
) {
	serviceMap = make(map[string][]common.Node)
	nodesActive = make(map[string]common.Node)
	nodesStale = make(map[string]common.Node)
	ch, err := r.Node.GetEventsCh(r.MQPrefix + "heartbeat/#")
	if err != nil {
		log.Panicf("can't connect to %s: %s", r.Cfg.MQAddress, err)
	}
	// time to wait for event stream to start
	discoveryTime := time.After(time.Second * 10)
	exit := false
	ctr := 0
	for {
		if exit {
			break
		}
		select {
		case ev := <-ch:
			ctr++
			if ctr == 1 {
				// once event stream started, shorten the idle timeout
				discoveryTime = time.After(time.Second * 4)
			}
			path := strings.Split(ev.RoutingKey, "/")
			if len(path) < 2 {
				r.Log.Errorf("path too short: %s", ev.RoutingKey)
			}
			fqdn := path[len(path)-1]
			_ = fqdn

			var hb zerosvc.Heartbeat
			hbNode := common.Node{
				Services: make([]string, 0),
			}
			err := json.Unmarshal(ev.Body, &hb)
			if err != nil {
				r.Log.Errorf("error unmarshalling %s: %s", string(ev.Body), err)
				continue
			}
			has := func(key string) (string, bool) { str, ok := hb.NodeInfo[key].(string); return str, ok }
			if val, ok := has("fqdn"); ok {
				hbNode.FQDN = val
			} else {
				r.Log.Warnf("node without info data: %s", ev.NodeName())
				continue
			}
			if val, ok := has("version"); ok {
				hbNode.DaemonVersion = val
			}

			ts := ev.TS()
			hbNode.LastUpdate = &ts
			for k, _ := range hb.Services {
				if _, ok := serviceMap[k]; !ok {
					serviceMap[k] = make([]common.Node, 0)
				}
				serviceMap[k] = append(serviceMap[k], hbNode)
				hbNode.Services = append(hbNode.Services, k)
			}
			if ev.RetainTill.After(time.Now()) {
				nodesActive[hbNode.FQDN] = hbNode
			} else {
				nodesStale[hbNode.FQDN] = hbNode
			}
		case <-discoveryTime:
			exit = true
		}
	}
	return serviceMap, nodesActive, nodesStale, nil

}
