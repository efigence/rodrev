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

// discoveryTopic is where nodes announce themselves, relative to the MQ prefix
const discoveryTopic = "discovery/#"

// staleAfter is how many heartbeat intervals a node can miss before it counts as
// stale. The interval itself comes from the node's own heartbeat
const staleAfter = 3

// minStaleAge is the shortest a heartbeat has to be silent before the node counts
// as stale, for nodes that do not say how often they check in
const minStaleAge = time.Minute * 15

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
// Cleanup matters more than it looks: the transport hands messages over from its
// own reader, so a subscription nobody reads any more blocks it - pings included
// - until the broker drops the connection, which breaks every later
// subscription. So the channel keeps being drained in the background after the
// results are in
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
	ch, err := r.SubscribeRaw(discoveryTopic)
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
		case msg := <-ch:
			count++
			if count == 1 {
				// stream started, from now on wait only for a lull in it
				deadline = time.After(o.IdleWait)
			}
			node, services, ok := parseHeartbeat(r, msg)
			if !ok {
				continue
			}
			for _, service := range services {
				d.Services[service] = append(d.Services[service], node)
			}
			if node.LastUpdate != nil && time.Since(*node.LastUpdate) > node.StaleAge {
				d.Stale[node.FQDN] = node
			} else {
				d.Active[node.FQDN] = node
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

// parseHeartbeat turns a heartbeat message into a node entry and the list of
// services it announced.
//
// Heartbeats are plain JSON node info rather than events, and everything rodrev
// needs beyond the name lives in the data of its own service entry
func parseHeartbeat(r *common.Runtime, msg *zerosvc.Message) (common.Node, []string, bool) {
	node := common.Node{Services: make([]string, 0)}
	source := heartbeatSource(msg.Topic)
	if len(msg.Payload) == 0 {
		// a retained heartbeat being cleared, which is how a node says goodbye
		r.Log.Debugf("heartbeat from [%s] cleared", source)
		return node, nil, false
	}
	var info zerosvc.NodeInfo
	if err := json.Unmarshal(msg.Payload, &info); err != nil {
		r.Log.Errorf("error unmarshalling heartbeat from [%s] (%d bytes: %s): %s",
			source, len(msg.Payload), string(msg.Payload), err)
		return node, nil, false
	}
	svc, isRodrev := info.Services[common.RodrevServiceName]
	if !isRodrev {
		// something else on the same topic tree - a client announcing itself,
		// for one. Not a fleet node
		r.Log.Debugf("heartbeat from [%s] is not a rodrev node, ignoring", source)
		return node, nil, false
	}
	rodrev, ok := common.ParseRodrevService(svc)
	if !ok {
		r.Log.Warnf("heartbeat from [%s] has no usable rodrev service data, ignoring", source)
		return node, nil, false
	}
	node.FQDN = rodrev.FQDN
	node.DaemonVersion = rodrev.Version
	node.Features = rodrev.Features
	ts := info.TS
	node.LastUpdate = &ts
	node.StaleAge = staleAge(rodrev.HeartbeatInterval)
	services := make([]string, 0, len(info.Services))
	for service := range info.Services {
		services = append(services, service)
	}
	sort.Strings(services)
	node.Services = services
	return node, services, true
}

// staleAge is how long a node's heartbeat can be silent before it counts as
// stale. Nodes report how often they check in, so a node with a slow heartbeat
// does not look dead
func staleAge(interval time.Duration) time.Duration {
	age := interval * staleAfter
	if age < minStaleAge {
		return minStaleAge
	}
	return age
}

// heartbeatSource names whoever sent a heartbeat. The topic is
// <root>/discovery/<node name>/<node uuid>, and the name is the only identity
// available when the payload can not be parsed
func heartbeatSource(topic string) string {
	path := strings.Split(topic, "/")
	if len(path) >= 2 {
		return path[len(path)-2]
	}
	if len(topic) > 0 {
		return topic
	}
	return "unknown"
}

// drain keeps reading a channel nobody cares about any more, so the transport
// handler feeding it never blocks
func drain[T any](ch chan T) {
	for range ch {
	}
}
