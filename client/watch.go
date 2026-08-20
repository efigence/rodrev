package client

import (
	"context"
	"sort"
	"sync"
	"time"

	"github.com/efigence/rodrev/common"
	"github.com/zerosvc/go-zerosvc"
)

// NodeWatcher keeps a live view of the fleet from the heartbeat stream.
//
// It exists because retained heartbeats are only delivered when a subscription
// is made, and a second subscription to the same topic filter does not get them
// again - so a long running process can not just re-run discovery. One
// subscription that is never dropped sees every heartbeat, including the empty
// one a node sends on its way out
type NodeWatcher struct {
	r     *common.Runtime
	l     sync.Mutex
	nodes map[string]common.Node
}

// WatchNodes subscribes to the heartbeat stream and keeps following it
func WatchNodes(r *common.Runtime) (*NodeWatcher, error) {
	ch, err := r.SubscribeRaw(discoveryTopic)
	if err != nil {
		return nil, err
	}
	w := NodeWatcher{r: r, nodes: make(map[string]common.Node, 16)}
	go w.run(ch)
	return &w, nil
}

func (w *NodeWatcher) run(ch chan *zerosvc.Message) {
	for msg := range ch {
		if len(msg.Payload) == 0 {
			// retained presence cleared: the node is gone
			w.l.Lock()
			delete(w.nodes, msg.Topic)
			w.l.Unlock()
			continue
		}
		node, _, ok := parseHeartbeat(w.r, msg)
		if !ok {
			continue
		}
		w.l.Lock()
		w.nodes[msg.Topic] = node
		w.l.Unlock()
	}
}

// WaitForNodes blocks until at least one node shows up or the context is done.
// Retained heartbeats are delivered right after subscribing, so on a healthy
// broker this is a matter of milliseconds
func (w *NodeWatcher) WaitForNodes(ctx context.Context) {
	for {
		w.l.Lock()
		count := len(w.nodes)
		w.l.Unlock()
		if count > 0 {
			return
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(time.Millisecond * 20):
		}
	}
}

// Discovery returns the current view, split into nodes that are checking in and
// nodes whose heartbeat went quiet
func (w *NodeWatcher) Discovery() Discovery {
	d := Discovery{
		Services: make(map[string][]common.Node),
		Active:   make(map[string]common.Node),
		Stale:    make(map[string]common.Node),
	}
	w.l.Lock()
	defer w.l.Unlock()
	for _, node := range w.nodes {
		if node.LastUpdate != nil && time.Since(*node.LastUpdate) > node.StaleAge {
			d.Stale[node.FQDN] = node
		} else {
			d.Active[node.FQDN] = node
		}
		for _, service := range node.Services {
			d.Services[service] = append(d.Services[service], node)
		}
	}
	for _, nodes := range d.Services {
		sort.Slice(nodes, func(i, j int) bool { return nodes[i].FQDN < nodes[j].FQDN })
	}
	return d
}
