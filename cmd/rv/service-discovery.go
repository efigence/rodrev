package main

import (
	"encoding/json"
	"github.com/zerosvc/go-zerosvc"
	"strings"
	"time"
)

func ServiceDiscovery(node *zerosvc.Node) {
	ch, err := node.GetEventsCh("rf/heartbeat/#")
		if err != nil {
			log.Panicf("can't connect: %s",err)
		}
		services := make(map[string]map[string]bool,0)
		totalDiscoveryTime := time.After(time.Second * 30)
		exit := false
		log.Info("running service discovery")
		for {
			if exit {break}
			select {
			case ev := <-ch:
				path := strings.Split(ev.RoutingKey, "/")
				if len(path) < 2 {
					log.Errorf("path too short: %s", ev.RoutingKey)
				}
				fqdn := path[len(path)-1]

				var hb zerosvc.Heartbeat
				err := json.Unmarshal(ev.Body, &hb)
				if err != nil {
					log.Errorf("error unmarshalling %s: %s", string(ev.Body), err)
					continue
				}
				for k, _ := range hb.Services {
					if _, ok := services[k]; !ok {
						services[k] = make(map[string]bool)
						services[k][fqdn] = true
					}
				}
			case <-time.After(4 * time.Second):
					exit = true
			case <- totalDiscoveryTime:
				exit = true
			}
		}
		for svc, hosts := range services {
			log.Infof("%s:",svc)
			for host, _ := range hosts {
				log.Infof("    %s",host)
			}
		}
}