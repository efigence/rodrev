package repl

import (
	"context"
	"time"

	"github.com/efigence/rodrev/plugin/puppet"
)

// NodeResult is a single node's answer to a query
type NodeResult struct {
	FQDN    string
	Matched bool
	// Err is the error the node hit while evaluating, empty when there was none
	Err string
	RTT time.Duration
}

// Summary is the outcome of one query
type Summary struct {
	Expr string
	// Matched/Responded/Errors count nodes. Known is how many nodes we believe
	// exist; 0 means unknown, in which case only Responded can be trusted
	Matched   int
	Responded int
	Known     int
	Errors    int
	Elapsed   time.Duration
	// Silent is set by backends where a node that did not match does not answer,
	// so "did not match" and "is down" can not be told apart
	Silent bool
	// Type/Value/Boolish describe the returned value. Only backends that evaluate
	// locally can fill these in, the cluster only reports booleans
	Type    string
	Value   string
	Boolish bool
}

// Backend evaluates queries, either against the cluster or against local data
type Backend interface {
	// Describe returns a short label for the prompt and the banner
	Describe() string
	// Eval evaluates expr, calling sink for every node result as it arrives.
	// A node that failed to evaluate is a NodeResult with Err set, not an error;
	// the error return is for transport failures
	Eval(ctx context.Context, expr string, sink func(NodeResult)) (Summary, error)
	// Nodes returns known node names. refresh forces re-discovery
	Nodes(ctx context.Context, refresh bool) ([]string, error)
	// Snapshot returns fact/class data for completion and :fact/:class.
	// fqdn is ignored by backends that only have one node's data
	Snapshot(ctx context.Context, fqdn string) (*puppet.Snapshot, error)
	Close() error
}
