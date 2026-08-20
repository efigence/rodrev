package repl

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/efigence/rodrev/plugin/puppet"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testDataDir = "../../t-data"

func testSession(t *testing.T, format string) (*Session, *bytes.Buffer) {
	t.Helper()
	snap, err := puppet.LoadSnapshotFiles(
		testDataDir+"/"+puppet.HarnessFactsFile,
		testDataDir+"/"+puppet.HarnessClassfile,
		testDataDir+"/"+puppet.HarnessLastRunSummaryFile,
	)
	require.NoError(t, err)
	h, err := snap.Harness(nil)
	require.NoError(t, err)
	var out bytes.Buffer
	s, err := New(Config{
		Backend:  NewLocalBackend(h),
		Snapshot: snap,
		Out:      &out,
		Format:   format,
	})
	require.NoError(t, err)
	return s, &out
}

// fakeBackend answers with a scripted set of nodes, so cluster-shaped output can
// be tested without a broker
type fakeBackend struct {
	results []NodeResult
	known   int
	delay   time.Duration
	// err fails the query outright, errAfter fails it after reporting results
	err      error
	errAfter error
	silent   bool
}

func (f *fakeBackend) Describe() string { return "fake cluster" }
func (f *fakeBackend) Eval(ctx context.Context, expr string, sink func(NodeResult)) (Summary, error) {
	if f.err != nil {
		return Summary{}, f.err
	}
	sum := Summary{Expr: expr, Known: f.known, Elapsed: f.delay, Silent: f.silent}
	for _, r := range f.results {
		sum.Responded++
		if r.Matched {
			sum.Matched++
		}
		if len(r.Err) > 0 {
			sum.Errors++
		}
		if sink != nil {
			sink(r)
		}
	}
	return sum, f.errAfter
}
func (f *fakeBackend) Nodes(ctx context.Context, refresh bool) ([]string, error) {
	out := make([]string, 0, len(f.results))
	for _, r := range f.results {
		out = append(out, r.FQDN)
	}
	return out, nil
}
func (f *fakeBackend) Snapshot(ctx context.Context, fqdn string) (*puppet.Snapshot, error) {
	return nil, assert.AnError
}
func (f *fakeBackend) Close() error { return nil }

func TestEvalLineQueries(t *testing.T) {
	tests := []struct {
		name     string
		line     string
		contains []string
		exitCode int
	}{
		{name: "class match", line: `(== (class "nginx") true)`, contains: []string{"=> true"}, exitCode: 0},
		{name: "class miss", line: `(== (class "apache") true)`, contains: []string{"=> false"}, exitCode: 1},
		{name: "nested fact", line: `(== (fact "os" "distro" "codename") "bookworm")`, contains: []string{"=> true"}, exitCode: 0},
		{name: "array index", line: `(== (fact "networking" "interfaces" "eth0" "bindings" 0 "address") "10.100.101.33")`, contains: []string{"=> true"}, exitCode: 0},
		{name: "node meta", line: `(regexp (-> node %fqdn) "^d1-")`, contains: []string{"=> true"}, exitCode: 0},
		{name: "non boolean value", line: `(fact "os" "distro")`, contains: []string{"=>", "bookworm", "not a boolean"}, exitCode: 1},
		{name: "syntax error", line: `(== (class "nginx" true)`, contains: []string{"error:", "parser needs more input"}, exitCode: 2},
		{name: "unknown symbol", line: `(clas "nginx")`, contains: []string{"symbol `clas` not found"}, exitCode: 2},
		{name: "comment", line: `# nothing to see`, contains: []string{}, exitCode: 0},
		{name: "empty", line: "   ", contains: []string{}, exitCode: 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, out := testSession(t, FormatHuman)
			require.NoError(t, s.EvalLine(tt.line))
			for _, want := range tt.contains {
				assert.Contains(t, out.String(), want)
			}
			if len(tt.contains) == 0 {
				assert.Empty(t, out.String())
			}
			assert.Equal(t, tt.exitCode, s.ExitCode())
		})
	}
}

func TestEvalLineMeta(t *testing.T) {
	tests := []struct {
		name       string
		lines      []string
		contains   []string
		notContain []string
	}{
		{name: "help", lines: []string{":help"}, contains: []string{":syntax", ":fact", "anything else is evaluated"}},
		{name: "help alias", lines: []string{":?"}, contains: []string{":syntax"}},
		{name: "syntax", lines: []string{":syntax"}, contains: []string{`(== (class "nginx") true)`, "zygomys"}},
		{name: "fact list", lines: []string{":fact"}, contains: []string{"virtual", "facts."}},
		{name: "fact path", lines: []string{":fact os.distro.codename"}, contains: []string{"os.distro.codename: bookworm"}},
		{name: "fact glob", lines: []string{":fact apt_has*"}, contains: []string{"apt_has_updates: true"}, notContain: []string{"virtual"}},
		{name: "fact nested value", lines: []string{":fact processors"}, contains: []string{"count: 4"}},
		{name: "fact miss", lines: []string{":fact nope_nope"}, contains: []string{"no fact matches"}},
		{name: "class list", lines: []string{":class"}, contains: []string{"nginx", "classes"}},
		{name: "class glob", lines: []string{":class mon::check::p*"}, contains: []string{"mon::check::puppet", "2 classes"}},
		{name: "class miss", lines: []string{":class nope::*"}, contains: []string{"no class matches"}},
		{name: "nodes", lines: []string{":nodes"}, contains: []string{"d1-lg.example.com", "1 nodes"}},
		{name: "snapshot info", lines: []string{":snapshot"}, contains: []string{"d1-lg.example.com", "facts,"}},
		{name: "out", lines: []string{":out json"}, contains: []string{"output format: json"}},
		{name: "out bad", lines: []string{":out yaml"}, contains: []string{"error:", "unknown format"}},
		{name: "timeout", lines: []string{":timeout 10s"}, contains: []string{"timeout: 10s"}},
		{name: "timeout bad", lines: []string{":timeout tenseconds"}, contains: []string{"error:", "can't parse duration"}},
		{name: "timeout negative", lines: []string{":timeout -5s"}, contains: []string{"error:", "must be positive"}},
		{name: "verbose", lines: []string{":verbose"}, contains: []string{"verbose: true"}},
		{name: "abbreviation", lines: []string{":ti 5s"}, contains: []string{"timeout: 5s"}},
		{name: "ambiguous", lines: []string{":c"}, contains: []string{"ambiguous command :c", ":class", ":cluster"}},
		{name: "unknown", lines: []string{":nope"}, contains: []string{"unknown command :nope"}},
		{name: "cluster unavailable", lines: []string{":cluster"}, contains: []string{"no cluster connection"}},
		{name: "local reuses snapshot", lines: []string{":local"}, contains: []string{"evaluating locally"}},
		{name: "local bad file", lines: []string{":local /nonexistent/facts.yaml"}, contains: []string{"error:"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, out := testSession(t, FormatHuman)
			for _, line := range tt.lines {
				require.NoError(t, s.EvalLine(line))
			}
			for _, want := range tt.contains {
				assert.Contains(t, out.String(), want)
			}
			for _, unwanted := range tt.notContain {
				assert.NotContains(t, out.String(), unwanted)
			}
		})
	}
}

func TestQuitAndScript(t *testing.T) {
	s, out := testSession(t, FormatHuman)
	script := strings.Join([]string{
		"# a comment",
		"",
		`(== (class "nginx") true)`,
		":quit",
		`(== (class "never-reached") true)`,
	}, "\n")
	require.NoError(t, s.RunScript(strings.NewReader(script)))
	assert.True(t, s.Quit())
	assert.Contains(t, out.String(), "=> true")
	assert.NotContains(t, out.String(), "=> false")
	assert.Equal(t, 0, s.ExitCode())
}

func TestNoDataHint(t *testing.T) {
	var out bytes.Buffer
	s, err := New(Config{
		Backend: &fakeBackend{},
		Out:     &out,
	})
	require.NoError(t, err)
	require.NoError(t, s.EvalLine(":fact"))
	assert.Contains(t, out.String(), "no fact data loaded - run :snapshot")
	out.Reset()
	// the long hint shows up only once per session
	require.NoError(t, s.EvalLine(":class"))
	assert.Equal(t, "no fact data loaded\n", out.String())
}

// inspecting data in cluster mode says which node it came from, since queries
// still run everywhere
func TestDataSourceNote(t *testing.T) {
	snap, err := puppet.LoadSnapshotFiles(testDataDir+"/"+puppet.HarnessFactsFile, "", "")
	require.NoError(t, err)
	var out bytes.Buffer
	s, err := New(Config{Backend: &fakeBackend{}, Snapshot: snap, Out: &out})
	require.NoError(t, err)
	require.NoError(t, s.EvalLine(":fact virtual"))
	assert.Contains(t, out.String(), "(d1-lg.example.com, from files:")
	assert.Contains(t, out.String(), "queries still run on the cluster")
	assert.Contains(t, out.String(), "virtual: kvm")

	// in local mode there is nothing to disambiguate
	local, out2 := testSession(t, FormatHuman)
	require.NoError(t, local.EvalLine(":fact virtual"))
	assert.Equal(t, "virtual: kvm\n", out2.String())
}

// a failed query must not read as "nothing matched"
func TestQueryErrorReporting(t *testing.T) {
	t.Run("failure with no results", func(t *testing.T) {
		var out bytes.Buffer
		s, err := New(Config{
			Backend: &fakeBackend{err: errors.New("can't subscribe for replies: not currently connected")},
			Out:     &out,
		})
		require.NoError(t, err)
		require.NoError(t, s.EvalLine(`(== (class "nginx") true)`))
		assert.Contains(t, out.String(), "error: can't subscribe for replies")
		assert.Contains(t, out.String(), "reconnects in the background")
		assert.NotContains(t, out.String(), "matched")
		assert.Equal(t, 2, s.ExitCode())
	})
	t.Run("partial results are still shown", func(t *testing.T) {
		var out bytes.Buffer
		s, err := New(Config{
			Backend: &fakeBackend{
				known:    12,
				results:  []NodeResult{{FQDN: "d1.example.com", Matched: true}},
				errAfter: errors.New("connection to tcp://mq lost while waiting for replies"),
				silent:   true,
			},
			Out: &out,
		})
		require.NoError(t, err)
		require.NoError(t, s.EvalLine(`(== (class "nginx") true)`))
		assert.Contains(t, out.String(), "+ d1.example.com")
		assert.Contains(t, out.String(), "error: connection to tcp://mq lost")
		assert.Contains(t, out.String(), "1/12 matched")
		assert.Equal(t, 2, s.ExitCode())
	})
}

// a fleet-wide match must not scroll the summary off the screen
func TestStreamLimit(t *testing.T) {
	results := make([]NodeResult, 0, 50)
	for i := 0; i < 50; i++ {
		results = append(results, NodeResult{FQDN: fmt.Sprintf("d%02d.example.com", i), Matched: true})
	}
	var out bytes.Buffer
	s, err := New(Config{Backend: &fakeBackend{known: 50, results: results, silent: true}, Out: &out})
	require.NoError(t, err)
	require.NoError(t, s.EvalLine(`(== (class "nginx") true)`))
	got := out.String()
	assert.Equal(t, streamLimit, strings.Count(got, "  + d"))
	assert.Contains(t, got, fmt.Sprintf("... %d more not listed", 50-streamLimit))
	assert.Contains(t, got, "50/50 matched")

	// :verbose lists everything
	out.Reset()
	require.NoError(t, s.EvalLine(":verbose"))
	require.NoError(t, s.EvalLine(`(== (class "nginx") true)`))
	assert.Equal(t, 50, strings.Count(out.String(), "  + d"))
}

func TestClusterShapedOutput(t *testing.T) {
	var out bytes.Buffer
	backend := &fakeBackend{
		known: 12,
		results: []NodeResult{
			{FQDN: "d1.example.com", Matched: true},
			{FQDN: "d2.example.com", Matched: true},
			{FQDN: "d3.example.com"},
			{FQDN: "d4.example.com", Err: "error running query [x]: boom"},
		},
	}
	s, err := New(Config{Backend: backend, Out: &out})
	require.NoError(t, err)
	require.NoError(t, s.EvalLine(`(== (class "nginx") true)`))
	got := out.String()
	assert.Contains(t, got, "+ d1.example.com")
	assert.Contains(t, got, "+ d2.example.com")
	assert.NotContains(t, got, "- d3.example.com", "non-matching nodes are quiet unless :verbose")
	assert.Contains(t, got, "! d4.example.com: error running query [x]: boom")
	assert.Contains(t, got, "2/12 matched, 4 of 12 answered, 1 errors")
	assert.Equal(t, 2, s.ExitCode(), "a node error is an error")

	out.Reset()
	require.NoError(t, s.EvalLine(":verbose"))
	require.NoError(t, s.EvalLine(`(== (class "nginx") true)`))
	assert.Contains(t, out.String(), "- d3.example.com")
}

// the summary must say how much of the fleet was actually accounted for
func TestSummaryCoverage(t *testing.T) {
	tests := []struct {
		name     string
		backend  *fakeBackend
		contains []string
		absent   []string
	}{
		{
			name: "every node answered",
			backend: &fakeBackend{known: 3, results: []NodeResult{
				{FQDN: "a", Matched: true}, {FQDN: "b"}, {FQDN: "c"},
			}},
			contains: []string{"1/3 matched, 3 answered"},
			absent:   []string{"did not answer", "stayed silent"},
		},
		{
			name: "some nodes did not answer in time",
			backend: &fakeBackend{known: 5, results: []NodeResult{
				{FQDN: "a", Matched: true}, {FQDN: "b"},
			}},
			contains: []string{"1/5 matched, 2 of 5 answered", "3 nodes did not answer"},
		},
		{
			name: "old daemons stay silent when they do not match",
			backend: &fakeBackend{known: 5, silent: true, results: []NodeResult{
				{FQDN: "a", Matched: true},
			}},
			contains: []string{"1/5 matched, 1 of 5 answered", "only the match count is exact"},
			absent:   []string{"did not answer within"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var out bytes.Buffer
			s, err := New(Config{Backend: tt.backend, Out: &out, Verbose: true})
			require.NoError(t, err)
			require.NoError(t, s.EvalLine(`(== (class "nginx") true)`))
			for _, want := range tt.contains {
				assert.Contains(t, out.String(), want)
			}
			for _, unwanted := range tt.absent {
				assert.NotContains(t, out.String(), unwanted)
			}
		})
	}
}
