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
	l   sync.Mutex
	// nodes is the discovery cache, filled in in the background so the first
	// prompt does not have to wait for it
	nodes []string
	stale []string
}

func NewClusterBackend(r *common.Runtime, log *zap.SugaredLogger) *ClusterBackend {
	b := ClusterBackend{r: r, log: log}
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
		if _, err := b.discover(); err != nil {
			b.log.Debugf("background discovery failed: %s", err)
		}
	}()
	return &b
}

func (b *ClusterBackend) Describe() string {
	return "cluster " + common.RedactURL(b.r.Cfg.MQAddress)
}

func (b *ClusterBackend) Eval(ctx context.Context, expr string, sink func(NodeResult)) (Summary, error) {
	start := time.Now()
	matched := client.PuppetStatus(b.r, expr)
	sum := Summary{
		Expr:      expr,
		Matched:   len(matched),
		Responded: len(matched),
		Known:     len(b.Cached()),
		Elapsed:   time.Since(start),
		// non-matching nodes send nothing back, so a node that did not answer is
		// indistinguishable from one that is down
		Silent: true,
	}
	fqdns := make([]string, 0, len(matched))
	for fqdn := range matched {
		fqdns = append(fqdns, fqdn)
	}
	sort.Strings(fqdns)
	for _, fqdn := range fqdns {
		if sink != nil {
			sink(NodeResult{FQDN: fqdn, Matched: true})
		}
	}
	return sum, nil
}

func (b *ClusterBackend) Nodes(ctx context.Context, refresh bool) ([]string, error) {
	if cached := b.Cached(); len(cached) > 0 && !refresh {
		return cached, nil
	}
	if !refresh {
		// discovery is slow, let the background one finish instead of blocking
		return b.Cached(), nil
	}
	return b.discover()
}

func (b *ClusterBackend) discover() ([]string, error) {
	_, active, stale, err := client.Discover(b.r)
	if err != nil {
		return nil, err
	}
	nodes := make([]string, 0, len(active))
	for fqdn := range active {
		nodes = append(nodes, fqdn)
	}
	staleNodes := make([]string, 0, len(stale))
	for fqdn := range stale {
		staleNodes = append(staleNodes, fqdn)
	}
	sort.Strings(nodes)
	sort.Strings(staleNodes)
	b.l.Lock()
	b.nodes = nodes
	b.stale = staleNodes
	b.l.Unlock()
	return nodes, nil
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
	return nil, fmt.Errorf("pulling facts from a node needs rvd with fact dump support; " +
		"for now start with --facts/--data-dir to get completion and :fact")
}

func (b *ClusterBackend) Close() error { return nil }
