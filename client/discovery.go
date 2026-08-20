package client

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/efigence/rodrev/common"
	"github.com/zerosvc/go-zerosvc"
)

// DiscoverOpts tunes how long a discovery run listens for heartbeats
type DiscoverOpts struct {
	// InitialWait is how long to wait for the heartbeat stream to start
	InitialWait time.Duration
	// IdleWait is how long to keep listening after the last heartbeat arrived
	IdleWait time.Duration
}

var DefaultDiscoverOpts = DiscoverOpts{
	InitialWait: time.Second * 10,
	IdleWait:    time.Second * 4,
}

// Discovery is the result of one discovery run
type Discovery struct {
	// Services maps a service name to the nodes providing it
	Services map[string][]common.Node
	Active   map[string]common.Node
	Stale    map[string]common.Node
}

// ActiveNodes returns sorted fqdns of nodes with a live heartbeat
func (d *Discovery) ActiveNodes() []string {
	out := make([]string, 0, len(d.Active))
	for fqdn := range d.Active {
		out = append(out, fqdn)
	}
	sort.Strings(out)
	return out
}

// WithFeature returns the active nodes announcing support for a command
func (d *Discovery) WithFeature(feature string) []string {
	out := make([]string, 0, len(d.Active))
	for fqdn, node := range d.Active {
		for _, f := range node.Features {
			if f == feature {
				out = append(out, fqdn)
				break
			}
		}
	}
	sort.Strings(out)
	return out
}

// DiscoverOnce lists nodes that announced themselves via heartbeats and stops
// listening once it is done.
//
// Cleanup matters more than it looks: zerosvc has no way to unsubscribe, and its
// subscription handler hands events over on an unbuffered channel from inside the
// MQTT message router. A subscription nobody reads any more therefore blocks the
// router - pings included - until the broker drops the connection, which breaks
// every later subscription with "not currently connected". So the channel keeps
// being drained in the background after the results are in
func DiscoverOnce(r *common.Runtime, o DiscoverOpts) (Discovery, error) {
	d := Discovery{
		Services: make(map[string][]common.Node),
		Active:   make(map[string]common.Node),
		Stale:    make(map[string]common.Node),
	}
	if o.InitialWait <= 0 {
		o.InitialWait = DefaultDiscoverOpts.InitialWait
	}
	if o.IdleWait <= 0 {
		o.IdleWait = DefaultDiscoverOpts.IdleWait
	}
	ch, err := r.Node.GetEventsCh(r.MQPrefix + "heartbeat/#")
	if err != nil {
		return d, fmt.Errorf("can't subscribe to heartbeats at %s: %s",
			common.RedactURL(r.Cfg.MQAddress), err)
	}
	// keep the subscription drained: heartbeats keep coming after we stop caring
	defer func() { go drain(ch) }()
	deadline := time.After(o.InitialWait)
	count := 0
	for {
		select {
		case ev := <-ch:
			count++
			if count == 1 {
				// stream started, from now on wait only for a lull in it
				deadline = time.After(o.IdleWait)
			}
			node, services, ok := parseHeartbeat(r, &ev)
			if !ok {
				continue
			}
			for _, service := range services {
				d.Services[service] = append(d.Services[service], node)
			}
			if ev.RetainTill.After(time.Now()) {
				d.Active[node.FQDN] = node
			} else {
				d.Stale[node.FQDN] = node
			}
		case <-deadline:
			return d, nil
		}
	}
}

// Discover lists nodes that announced themselves via heartbeats, split into
// active and stale ones, plus a map of which nodes provide which service
func Discover(r *common.Runtime) (
	serviceMap map[string][]common.Node,
	nodesActive map[string]common.Node,
	nodesStale map[string]common.Node,
	err error,
) {
	d, err := DiscoverOnce(r, DefaultDiscoverOpts)
	return d.Services, d.Active, d.Stale, err
}

// parseHeartbeat turns a heartbeat event into a node entry and the list of
// services it announced
func parseHeartbeat(r *common.Runtime, ev *zerosvc.Event) (common.Node, []string, bool) {
	node := common.Node{Services: make([]string, 0)}
	source := heartbeatSource(ev)
	if len(strings.Split(ev.RoutingKey, "/")) < 2 {
		r.Log.Errorf("heartbeat from [%s]: path too short: %s", source, ev.RoutingKey)
	}
	var hb zerosvc.Heartbeat
	if err := json.Unmarshal(ev.Body, &hb); err != nil {
		// an empty body is a retained heartbeat being cleared, which is why the
		// size is worth printing along with the sender
		r.Log.Errorf("error unmarshalling heartbeat from [%s] (%d bytes: %s): %s",
			source, len(ev.Body), string(ev.Body), err)
		return node, nil, false
	}
	fqdn, ok := hb.NodeInfo["fqdn"].(string)
	if !ok {
		r.Log.Warnf("heartbeat from [%s] has no fqdn in node info, ignoring", source)
		return node, nil, false
	}
	node.FQDN = fqdn
	if version, ok := hb.NodeInfo["version"].(string); ok {
		node.DaemonVersion = version
	}
	// older daemons announce no features at all
	if features, ok := hb.NodeInfo["features"].(string); ok && len(features) > 0 {
		node.Features = strings.Split(features, ",")
	}
	ts := ev.TS()
	node.LastUpdate = &ts
	services := make([]string, 0, len(hb.Services))
	for service := range hb.Services {
		services = append(services, service)
	}
	sort.Strings(services)
	node.Services = services
	return node, services, true
}

// heartbeatSource names whoever sent a heartbeat. The topic ends with the node
// name, which is the only identity available when the payload can not be parsed
// - the node-name header is empty on a cleared retained message
func heartbeatSource(ev *zerosvc.Event) string {
	if path := strings.Split(ev.RoutingKey, "/"); len(path) > 0 && len(path[len(path)-1]) > 0 {
		return path[len(path)-1]
	}
	if name := ev.NodeName(); len(name) > 0 {
		return name
	}
	return "unknown"
}

// drain keeps reading a channel nobody cares about any more, so the zerosvc
// message handler feeding it never blocks
func drain(ch chan zerosvc.Event) {
	for range ch {
	}
}
