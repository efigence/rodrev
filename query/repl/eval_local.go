package repl

import (
	"context"
	"fmt"
	"time"

	"github.com/efigence/rodrev/plugin/puppet"
)

// LocalBackend evaluates queries against a single node's data loaded from files
// or from a snapshot, with no MQ involved
type LocalBackend struct {
	h *puppet.QueryHarness
}

func NewLocalBackend(h *puppet.QueryHarness) *LocalBackend {
	return &LocalBackend{h: h}
}

func (b *LocalBackend) Describe() string {
	source := b.h.Source
	if len(source) == 0 {
		source = b.h.FQDN()
	}
	return "local " + source
}

func (b *LocalBackend) Eval(ctx context.Context, expr string, sink func(NodeResult)) (Summary, error) {
	start := time.Now()
	sum := Summary{Expr: expr, Responded: 1, Known: 1}
	res, err := b.h.Engine.Parse(expr)
	sum.Elapsed = time.Since(start)
	r := NodeResult{FQDN: b.h.FQDN(), RTT: sum.Elapsed}
	if err != nil {
		r.Err = err.Error()
		sum.Errors = 1
	} else {
		sum.Type = res.Type
		sum.Value = res.Display
		sum.Boolish = res.Boolish
		r.Matched = res.Bool
		if res.Bool {
			sum.Matched = 1
		}
	}
	if sink != nil {
		sink(r)
	}
	return sum, nil
}

func (b *LocalBackend) Nodes(ctx context.Context, refresh bool) ([]string, error) {
	return []string{b.h.FQDN()}, nil
}

func (b *LocalBackend) Snapshot(ctx context.Context, fqdn string) (*puppet.Snapshot, error) {
	if len(fqdn) > 0 && fqdn != b.h.FQDN() {
		return nil, fmt.Errorf("local mode only has data for %s", b.h.FQDN())
	}
	summary := b.h.LastRunSummary
	return &puppet.Snapshot{
		FQDN:           b.h.FQDN(),
		TS:             time.Now(),
		Source:         b.h.Source,
		Facts:          *b.h.Facts.Map(),
		Classes:        b.h.Classes.List(),
		LastRunSummary: &summary,
	}, nil
}

func (b *LocalBackend) Close() error { return nil }

// Harness exposes the underlying data set, for :local reuse and completion
func (b *LocalBackend) Harness() *puppet.QueryHarness { return b.h }
