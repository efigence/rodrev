package repl

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/efigence/rodrev/client"
	"github.com/efigence/rodrev/common"
	"github.com/efigence/rodrev/plugin/puppet"
	"go.uber.org/zap"
)

// waitForFirstHeartbeat is how long to give retained heartbeats to arrive before
// reporting an empty fleet
const waitForFirstHeartbeat = time.Second

// ClusterBackend evaluates a query on every node by sending it as the filter of
// a status request. Nodes that do not match stay silent, so only the matched
// count is exact - the fleet size comes from heartbeat discovery
type ClusterBackend struct {
	r   *common.Runtime
	log *zap.SugaredLogger
	// session carries every query of this REPL over one reply subscription
	session *client.Session
	// watcher follows the heartbeat stream for as long as the session lives
	watcher *client.NodeWatcher
	l       sync.Mutex
}

func NewClusterBackend(r *common.Runtime, log *zap.SugaredLogger) (*ClusterBackend, error) {
	session, err := client.NewSession(r)
	if err != nil {
		return nil, err
	}
	// retained heartbeats arrive the moment the subscription is made and only
	// then, so the fleet view is followed instead of being polled
	watcher, err := client.WatchNodes(r)
	if err != nil {
		_ = session.Close()
		return nil, err
	}
	b := ClusterBackend{r: r, log: log, session: session, watcher: watcher}
	return &b, nil
}

func (b *ClusterBackend) Describe() string {
	return "cluster " + common.RedactURL(b.r.Cfg.MQAddress)
}

func (b *ClusterBackend) Eval(ctx context.Context, expr string, sink func(NodeResult)) (Summary, error) {
	if b.QueryCapable() {
		return b.evalQuery(ctx, expr, sink)
	}
	return b.evalFilter(ctx, expr, sink)
}

// evalQuery uses the query command, where every node answers and the counts are
// therefore exact
func (b *ClusterBackend) evalQuery(ctx context.Context, expr string, sink func(NodeResult)) (Summary, error) {
	start := time.Now()
	out, err := b.session.Query(ctx, expr, func(qr puppet.QueryReply) {
		if sink != nil {
			sink(NodeResult{FQDN: qr.FQDN, Matched: qr.Matched, Err: qr.Error, RTT: time.Since(start)})
		}
	})
	sum := Summary{
		Expr:      expr,
		Matched:   len(out.Matched),
		Responded: len(out.Matched) + len(out.NotMatched) + len(out.Errors),
		Errors:    len(out.Errors),
		Known:     len(b.Cached()),
		Elapsed:   time.Since(start),
	}
	return sum, err
}

// evalFilter is the fallback for daemons that do not know the query command: the
// expression travels as a status request filter. Daemons new enough to
// understand answer_always still report that they did not match, so the count is
// exact for those; older ones only answer when they match
func (b *ClusterBackend) evalFilter(ctx context.Context, expr string, sink func(NodeResult)) (Summary, error) {
	start := time.Now()
	out, err := b.session.FilterMatch(ctx, expr, func(fqdn string) {
		if sink != nil {
			sink(NodeResult{FQDN: fqdn, Matched: true, RTT: time.Since(start)})
		}
	})
	for fqdn, nodeErr := range out.Errors {
		if sink != nil {
			sink(NodeResult{FQDN: fqdn, Err: nodeErr, RTT: time.Since(start)})
		}
	}
	sum := Summary{
		Expr:      expr,
		Matched:   len(out.Matched),
		Responded: out.Answered(),
		Errors:    len(out.Errors),
		Known:     len(b.Cached()),
		Elapsed:   time.Since(start),
		// with no node saying "not me", a node that did not answer is
		// indistinguishable from one that is down
		Silent: len(out.NoMatch) == 0,
	}
	return sum, err
}

// QueryCapable reports whether the whole fleet can answer the query command. A
// mixed fleet stays on the fallback: an old daemon answers a query command with
// "unknown command", which would show up as a per-node error
func (b *ClusterBackend) QueryCapable() bool {
	d := b.watcher.Discovery()
	return len(d.Active) > 0 && len(d.WithFeature(puppet.Query)) == len(d.Active)
}

// Capabilities describes what the fleet supports, for :nodes
func (b *ClusterBackend) Capabilities() string {
	d := b.watcher.Discovery()
	if len(d.Active) == 0 {
		return ""
	}
	capable := len(d.WithFeature(puppet.Query))
	if capable == len(d.Active) {
		return fmt.Sprintf("all %d nodes report matches exactly", len(d.Active))
	}
	return fmt.Sprintf("%d of %d nodes support exact match reporting; "+
		"until the rest are upgraded, nodes that do not match stay silent",
		capable, len(d.Active))
}

// Nodes returns the fleet as the heartbeat stream currently has it. The view is
// live, so a refresh has nothing to do beyond giving the first heartbeats a
// moment to land when the session has only just started
func (b *ClusterBackend) Nodes(ctx context.Context, refresh bool) ([]string, error) {
	if nodes := b.Cached(); len(nodes) > 0 {
		return nodes, nil
	}
	waitCtx, cancel := context.WithTimeout(ctx, waitForFirstHeartbeat)
	defer cancel()
	b.watcher.WaitForNodes(waitCtx)
	return b.Cached(), nil
}

// Stale returns nodes whose heartbeat went quiet
func (b *ClusterBackend) Stale() []string {
	d := b.watcher.Discovery()
	out := make([]string, 0, len(d.Stale))
	for fqdn := range d.Stale {
		out = append(out, fqdn)
	}
	sort.Strings(out)
	return out
}

// Cached returns the nodes seen so far
func (b *ClusterBackend) Cached() []string {
	d := b.watcher.Discovery()
	return d.ActiveNodes()
}

func (b *ClusterBackend) Snapshot(ctx context.Context, fqdn string) (*puppet.Snapshot, error) {
	if len(fqdn) == 0 {
		return nil, fmt.Errorf("which node? :snapshot <fqdn> (:nodes lists them)")
	}
	return b.session.NodeSnapshot(ctx, fqdn)
}

func (b *ClusterBackend) Close() error { return b.session.Close() }
