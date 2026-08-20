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

// ClusterBackend evaluates a query on every node by sending it as the filter of
// a status request. Nodes that do not match stay silent, so only the matched
// count is exact - the fleet size comes from heartbeat discovery
type ClusterBackend struct {
	r   *common.Runtime
	log *zap.SugaredLogger
	// session carries every query of this REPL over one reply subscription
	session *client.Session
	l       sync.Mutex
	// nodes is the discovery cache, filled in in the background so the first
	// prompt does not have to wait for it
	nodes []string
	stale []string
	// queryCapable is set when every active node runs a daemon that answers the
	// query command, which is what makes exact counts possible
	queryCapable      bool
	queryCapableCount int
}

func NewClusterBackend(r *common.Runtime, log *zap.SugaredLogger) (*ClusterBackend, error) {
	session, err := client.NewSession(r)
	if err != nil {
		return nil, err
	}
	b := ClusterBackend{r: r, log: log, session: session}
	// discovery takes ~14s (it waits for heartbeats), so it runs in the
	// background and the node list fills in whenever it is done
	go func() {
		// discovery is best effort: whatever goes wrong in there must not take
		// the REPL down with it
		defer func() {
			if err := recover(); err != nil {
				b.log.Debugf("background discovery panicked: %s", err)
			}
		}()
		// retained heartbeats land within milliseconds of subscribing, so the
		// node list does not need the conservative default windows
		if _, err := b.discover(client.DiscoverOpts{
			InitialWait: time.Second * 2,
			IdleWait:    time.Millisecond * 400,
		}); err != nil {
			b.log.Debugf("background discovery failed: %s", err)
		}
	}()
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
	b.l.Lock()
	defer b.l.Unlock()
	return b.queryCapable
}

// Capabilities describes what the fleet supports, for :nodes
func (b *ClusterBackend) Capabilities() string {
	b.l.Lock()
	defer b.l.Unlock()
	if len(b.nodes) == 0 {
		return ""
	}
	if b.queryCapable {
		return fmt.Sprintf("all %d nodes report matches exactly", len(b.nodes))
	}
	return fmt.Sprintf("%d of %d nodes support exact match reporting; "+
		"until the rest are upgraded, nodes that do not match stay silent",
		b.queryCapableCount, len(b.nodes))
}

func (b *ClusterBackend) Nodes(ctx context.Context, refresh bool) ([]string, error) {
	if !refresh {
		// discovery takes seconds, serve whatever the background run found
		return b.Cached(), nil
	}
	return b.discover(client.DefaultDiscoverOpts)
}

// discover runs one discovery pass and caches the result. Heartbeats are
// retained, so a short window is enough to see the whole fleet
func (b *ClusterBackend) discover(o client.DiscoverOpts) ([]string, error) {
	d, err := client.DiscoverOnce(b.r, o)
	if err != nil {
		return nil, err
	}
	nodes := d.ActiveNodes()
	stale := make([]string, 0, len(d.Stale))
	for fqdn := range d.Stale {
		stale = append(stale, fqdn)
	}
	sort.Strings(stale)
	capable := d.WithFeature(puppet.Query)
	b.l.Lock()
	b.nodes = nodes
	b.stale = stale
	b.queryCapableCount = len(capable)
	b.queryCapable = len(nodes) > 0 && len(capable) == len(nodes)
	b.l.Unlock()
	return nodes, nil
}

// Stale returns nodes whose heartbeat has expired
func (b *ClusterBackend) Stale() []string {
	b.l.Lock()
	defer b.l.Unlock()
	out := make([]string, len(b.stale))
	copy(out, b.stale)
	return out
}

// Cached returns nodes discovered so far
func (b *ClusterBackend) Cached() []string {
	b.l.Lock()
	defer b.l.Unlock()
	out := make([]string, len(b.nodes))
	copy(out, b.nodes)
	return out
}

func (b *ClusterBackend) Snapshot(ctx context.Context, fqdn string) (*puppet.Snapshot, error) {
	if len(fqdn) == 0 {
		return nil, fmt.Errorf("which node? :snapshot <fqdn> (:nodes lists them)")
	}
	return b.session.NodeSnapshot(ctx, fqdn)
}

func (b *ClusterBackend) Close() error { return b.session.Close() }
